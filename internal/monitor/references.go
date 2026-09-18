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
const referenceRecheckInterval = 5 * time.Minute

// One collection per domain is shared by every target. Expired TTLs never become
// fresh because collection is rate-limited: the persistent pool decides whether
// an address is fresh, historical or expired independently of this budget.
type referenceCollection struct {
	done        chan struct{}
	fingerprint string
	nextAt      int64
	lastForced  int64
	complete    bool
	err         error
}

func (m *Monitor) references(ctx context.Context, domain model.Domain, cfg model.Config, force bool) ([]model.Reference, bool, error) {
	key := domain.Name + "\x00" + domain.Type
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		trusted, fingerprint, err := m.trustedSourceSnapshot(cfg)
		if err != nil {
			return nil, false, err
		}
		now := time.Now().UnixMilli()
		m.mu.Lock()
		if m.config.Concurrency == 0 {
			m.mu.Unlock()
			return nil, false, errors.New("可信参考探测已暂停")
		}
		state := m.referenceCollections[key]
		if state == nil {
			state = &referenceCollection{}
			m.referenceCollections[key] = state
		}
		if state.done != nil {
			done := state.done
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}
		canReuse := state.fingerprint == fingerprint && now < state.nextAt
		if force {
			canReuse = state.fingerprint == fingerprint && state.lastForced > 0 && now-state.lastForced < referenceRecheckInterval.Milliseconds()
		}
		if canReuse {
			complete, previousErr := state.complete, state.err
			m.mu.Unlock()
			refs, err := m.st.TrustedReferences(domain, now, cfg.ReferenceHistoryHours)
			if err != nil {
				return nil, false, err
			}
			_, currentFingerprint, snapshotErr := m.trustedSourceSnapshot(cfg)
			if snapshotErr != nil || currentFingerprint != fingerprint {
				if snapshotErr == nil {
					snapshotErr = errors.New("可信来源配置在参考读取期间发生变化，等待重新采集")
				}
				m.mu.Lock()
				if state.done == nil && state.fingerprint == fingerprint {
					state.nextAt = 0
					state.lastForced = 0
					state.complete = false
					state.err = snapshotErr
				}
				m.mu.Unlock()
				return refs, false, snapshotErr
			}
			return refs, complete && referenceSourcesPresent(refs, trusted), previousErr
		}
		done := make(chan struct{})
		state.done = done
		state.fingerprint = fingerprint
		if force {
			state.lastForced = now
		}
		generation := m.cacheGeneration
		m.mu.Unlock()

		complete := true
		var collectionErr error
		next := now + int64(cfg.ReferenceTTLSeconds)*1000
		if cfg.ReferenceTTLSeconds == 0 {
			next = now
		}
		for _, source := range trusted {
			current, err := m.st.GetServer(source.ID)
			if err != nil || !current.Enabled || !current.Trusted || current.Address != source.Address || current.TrustEpoch != source.TrustEpoch {
				complete = false
				continue
			}
			result, err := m.lookupMode(ctx, source, domain, cfg, true, force)
			if err != nil {
				complete = false
				collectionErr = err
				break
			}
			if !result.Success || result.Rcode != "NOERROR" || len(result.Answers) == 0 {
				complete = false
			}
			cacheDeadline := result.Timestamp + int64(cfg.ReferenceTTLSeconds)*1000
			if cacheDeadline < next {
				next = cacheDeadline
			}
			// Actual record TTL controls normal reuse; the independent minimum budget
			// prevents many targets from chasing very short or missing TTLs.
			if len(result.Records) == 0 {
				next = now
			}
			for _, record := range result.Records {
				if record.ExpiresAt < next {
					next = record.ExpiresAt
				}
			}
		}
		if err := ctx.Err(); err != nil {
			complete = false
			collectionErr = err
		}
		finished := time.Now().UnixMilli()
		refs, readErr := m.st.TrustedReferences(domain, finished, cfg.ReferenceHistoryHours)
		if readErr != nil {
			complete = false
			collectionErr = readErr
		}
		_, currentFingerprint, snapshotErr := m.trustedSourceSnapshot(cfg)
		if snapshotErr != nil || currentFingerprint != fingerprint {
			complete = false
			if snapshotErr != nil {
				collectionErr = snapshotErr
			} else {
				collectionErr = errors.New("可信来源配置在采集期间发生变化，等待重新采集")
			}
		}
		m.mu.Lock()
		if m.config.Concurrency == 0 && collectionErr == nil {
			complete = false
			collectionErr = errors.New("可信参考探测已暂停")
		}
		if generation != m.cacheGeneration {
			complete = false
			if collectionErr == nil {
				collectionErr = errors.New("手动刷新使在途可信采集失效，等待新一轮参考")
			}
		}
		state.complete = complete
		state.err = collectionErr
		state.nextAt = next
		if state.nextAt < finished+referenceMinInterval.Milliseconds() {
			state.nextAt = finished + referenceMinInterval.Milliseconds()
		}
		// Interrupted collections must be retried after resume, and a manual refresh
		// invalidates collection reuse without deleting trusted observation history.
		if collectionErr != nil || generation != m.cacheGeneration {
			state.nextAt = 0
			state.lastForced = 0
		}
		state.done = nil
		close(done)
		m.mu.Unlock()
		return refs, complete && referenceSourcesPresent(refs, trusted), collectionErr
	}
}

func (m *Monitor) classify(ctx context.Context, result *model.ProbeResult, domain model.Domain, cfg model.Config) error {
	// No A result means no reference traffic and no automatic E/F judgment.
	if !result.Success || domain.Type != "A" || result.Rcode != "NOERROR" || len(result.Answers) == 0 {
		compareAt(result, nil, time.Now().UnixMilli(), cfg.ReferenceHistoryHours)
		return nil
	}
	refs, complete, err := m.references(ctx, domain, cfg, false)
	compareAt(result, refs, time.Now().UnixMilli(), cfg.ReferenceHistoryHours)
	if err == nil && (result.Pollution == "suspicious" || result.Pollution == "polluted") {
		refs, complete, err = m.references(ctx, domain, cfg, true)
	}
	compareCollectionAt(result, refs, complete, time.Now().UnixMilli(), cfg.ReferenceHistoryHours)
	if err != nil {
		result.Pollution = "unknown"
		result.Reason = "可信参考采集未完整完成，暂不依据不完整集合评 E/F"
	}
	return err
}

// TTL=0 with disabled history, or expiry during a slow collection, must not let
// a remaining subset of trusted sources become sufficient negative evidence.
func referenceSourcesPresent(refs []model.Reference, sources []model.Server) bool {
	present := make(map[int64]bool, len(refs))
	for _, ref := range refs {
		if len(ref.Records) > 0 {
			present[ref.ServerID] = true
		}
	}
	for _, source := range sources {
		if !present[source.ID] {
			return false
		}
	}
	return true
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
