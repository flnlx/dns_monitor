package monitor

import (
	"context"
	"crypto/sha256"
	"dnsmonitor/internal/model"
	"errors"
	"fmt"
	"sort"
	"time"
)

const referenceMinInterval = 30 * time.Second

// Cold-start collection is shared per domain and rate limited when no eligible
// observations exist. A usable persistent pool always takes precedence over
// collection state: ordinary trusted probes keep that pool up to date.
type referenceCollection struct {
	done        chan struct{}
	fingerprint string
	nextAt      int64
	err         error
}

func (m *Monitor) references(ctx context.Context, domain model.Domain, cfg model.Config) ([]model.Reference, error) {
	key := domain.Name + "\x00" + domain.Type
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		now := time.Now().UnixMilli()
		refs, err := m.st.TrustedReferences(domain, now, cfg.ReferenceHistoryHours)
		if err != nil || hasUsableReference(refs, now, cfg.ReferenceHistoryHours) {
			return refs, err
		}
		trusted, fingerprint, err := m.trustedSourceSnapshot(cfg)
		if err != nil {
			return nil, err
		}
		m.mu.Lock()
		if m.config.Concurrency == 0 {
			m.mu.Unlock()
			return nil, errors.New("可信参考探测已暂停")
		}
		state := m.referenceCollections[key]
		if state == nil {
			state = &referenceCollection{}
			m.referenceCollections[key] = state
		}
		if state.done != nil {
			done, changed := state.done, m.changed
			m.mu.Unlock()
			// Subscribe before re-reading the pool so a normal trusted probe
			// cannot fill it between our read and notification registration.
			now = time.Now().UnixMilli()
			refs, err = m.st.TrustedReferences(domain, now, cfg.ReferenceHistoryHours)
			if err != nil || hasUsableReference(refs, now, cfg.ReferenceHistoryHours) {
				return refs, err
			}
			select {
			case <-done:
				continue
			case <-changed:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if state.fingerprint == fingerprint && now < state.nextAt {
			previousErr := state.err
			m.mu.Unlock()
			return refs, previousErr
		}
		done := make(chan struct{})
		state.done = done
		state.fingerprint = fingerprint
		generation := m.cacheGeneration
		m.mu.Unlock()

		var collectionErr error
		for _, source := range trusted {
			// A normal scheduled probe may have filled the pool while this
			// caller was waiting. Never send another query when it is usable.
			now = time.Now().UnixMilli()
			refs, err = m.st.TrustedReferences(domain, now, cfg.ReferenceHistoryHours)
			if err != nil || hasUsableReference(refs, now, cfg.ReferenceHistoryHours) {
				collectionErr = err
				break
			}
			current, err := m.st.GetServer(source.ID)
			if err != nil || !current.Enabled || !current.Trusted || current.Address != source.Address || current.TrustEpoch != source.TrustEpoch {
				continue
			}
			_, lookupErr := m.lookup(ctx, source, domain, cfg, true)
			// Read back the current, enabled source pool; a source changed in
			// flight cannot contribute its revoked observation.
			now = time.Now().UnixMilli()
			refs, err = m.st.TrustedReferences(domain, now, cfg.ReferenceHistoryHours)
			if err != nil {
				collectionErr = err
				break
			}
			if hasUsableReference(refs, now, cfg.ReferenceHistoryHours) {
				collectionErr = nil
				break
			}
			if lookupErr != nil {
				collectionErr = lookupErr
			}
			if ctx.Err() != nil {
				break
			}
			m.mu.Lock()
			interrupted := m.config.Concurrency == 0 || generation != m.cacheGeneration
			m.mu.Unlock()
			if interrupted {
				break
			}
		}
		finished := time.Now().UnixMilli()
		m.mu.Lock()
		if err := ctx.Err(); err != nil {
			collectionErr = err
		} else if m.config.Concurrency == 0 {
			collectionErr = errors.New("可信参考探测已暂停")
		} else if generation != m.cacheGeneration {
			collectionErr = errors.New("手动刷新使在途可信采集失效，等待新一轮参考")
		}
		state.err = collectionErr
		state.nextAt = finished + referenceMinInterval.Milliseconds()
		// Resume/manual refresh may retry immediately. Failed or empty DNS
		// replies otherwise share the same budget, even across many targets.
		if ctx.Err() != nil || m.config.Concurrency == 0 || generation != m.cacheGeneration {
			state.nextAt = 0
		}
		state.done = nil
		close(done)
		m.mu.Unlock()
		return refs, collectionErr
	}
}

func (m *Monitor) classify(ctx context.Context, result *model.ProbeResult, domain model.Domain, cfg model.Config) error {
	// No A result means no reference traffic and no automatic E/F judgment.
	if !result.Success || domain.Type != "A" || result.Rcode != "NOERROR" || len(result.Answers) == 0 {
		compareAt(result, nil, time.Now().UnixMilli(), cfg.ReferenceHistoryHours)
		return nil
	}
	refs, err := m.references(ctx, domain, cfg)
	compareAt(result, refs, time.Now().UnixMilli(), cfg.ReferenceHistoryHours)
	if err != nil {
		result.Pollution = "unknown"
		result.Reason = "可信参考暂不可用: " + err.Error()
	}
	return err
}

func hasUsableReference(refs []model.Reference, now int64, historyHours float64) bool {
	for _, ref := range refs {
		if !ref.Success || ref.Rcode != "NOERROR" {
			continue
		}
		for _, record := range ref.Records {
			if _, _, usable := referenceEligibility(record, now, historyHours); usable {
				return true
			}
		}
	}
	return false
}

// Hash source identity rather than retaining long endpoint strings once per
// domain. Epoch is persisted and changes on every trust/address/enabled edit.
func (m *Monitor) trustedSourceSnapshot(cfg model.Config) ([]model.Server, string, error) {
	servers, err := m.st.ListServers()
	if err != nil {
		return nil, "", err
	}
	trusted := make([]model.Server, 0)
	for _, s := range servers {
		if s.Enabled && s.Trusted {
			trusted = append(trusted, s)
		}
	}
	sort.Slice(trusted, func(i, j int) bool { return trusted[i].ID < trusted[j].ID })
	digest := sha256.New()
	for _, s := range trusted {
		fmt.Fprintf(digest, "%d/%d/%s\x00", s.ID, s.TrustEpoch, s.Address)
	}
	return trusted, fmt.Sprintf("%d/%x", cfg.ReferenceTTLSeconds, digest.Sum(nil)), nil
}
