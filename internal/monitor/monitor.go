package monitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"dnsmonitor/internal/model"
	"dnsmonitor/internal/store"
)

const maxQueued = 256

type cachedProbe struct{ result model.ProbeResult }
type completion struct {
	refreshID int64
	error     string
	serverID  int64
	nextDue   int64
	failures  int
}

type Monitor struct {
	batch                map[int64]bool
	batchSequence        int64
	cacheGeneration      uint64
	st                   *store.Store
	doggoPath            string
	wake                 chan struct{}
	done                 chan completion
	mu                   sync.Mutex
	config               model.Config
	status               model.RuntimeStatus
	queued               map[int64]bool
	inFlight             map[int64]bool
	busy                 map[int64]bool
	cache                map[string]cachedProbe
	referenceCollections map[string]*referenceCollection
	changed              chan struct{}
	probe                func(context.Context, string, model.Server, model.Domain, time.Duration) model.ProbeResult
}

func New(st *store.Store, doggoPath string) *Monitor {
	return &Monitor{st: st, doggoPath: doggoPath, wake: make(chan struct{}, 1), done: make(chan completion, 50), config: model.DefaultConfig(), queued: make(map[int64]bool), inFlight: make(map[int64]bool), busy: make(map[int64]bool), cache: make(map[string]cachedProbe), referenceCollections: make(map[string]*referenceCollection), changed: make(chan struct{}), probe: runDoggo}
}

func (m *Monitor) Wake() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Monitor) Queue(serverID int64) error {
	cfg, err := m.st.GetConfig()
	if err != nil {
		return err
	}
	if cfg.Concurrency == 0 {
		return errors.New("探测已暂停（并发为 0）")
	}
	s, err := m.st.GetServer(serverID)
	if err != nil {
		return err
	}
	if !s.Enabled {
		return errors.New("服务器已禁用")
	}
	m.mu.Lock()
	if m.queued[serverID] || m.inFlight[serverID] {
		m.mu.Unlock()
		return errors.New("该服务器已在队列中或正在探测")
	}
	if len(m.queued) >= maxQueued {
		m.mu.Unlock()
		return errors.New("手动探测队列已满，请稍后再试")
	}
	m.queued[serverID] = true
	m.mu.Unlock()
	m.Wake()
	return nil
}

func (m *Monitor) Status() model.RuntimeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.status
	if status.Refresh != nil {
		refresh := *status.Refresh
		status.Refresh = &refresh
	}
	return status
}
func (m *Monitor) setError(err error) {
	if err != nil {
		m.mu.Lock()
		m.status.LastError = err.Error()
		m.mu.Unlock()
	}
}
func (m *Monitor) signalLocked() { close(m.changed); m.changed = make(chan struct{}) }

// Run owns the schedule. Workers are bounded by the configured round concurrency;
// each worker probes domains and references serially. A second global gate bounds
// actual subprocesses, including trusted references, after live config changes.
func (m *Monitor) Run(ctx context.Context) {
	m.mu.Lock()
	m.status.StartedAt = time.Now().UnixMilli()
	m.mu.Unlock()
	nextDue := make(map[int64]int64)
	failures := make(map[int64]int)
	if summaries, err := m.st.Summary(time.Now().Add(-model.Retention).UnixMilli(), time.Now().UnixMilli()); err == nil {
		for _, s := range summaries {
			nextDue[s.ID] = s.NextDue
			failures[s.ID] = s.Failures
		}
	} else {
		m.setError(err)
	}
	var servers []model.Server
	var serverByID map[int64]model.Server
	var cfg model.Config
	var pruneAt int64
	var workers sync.WaitGroup
	refresh := func() {
		c, err := m.st.GetConfig()
		if err != nil {
			m.setError(err)
			return
		}
		if err = ValidateConfig(&c); err != nil {
			m.setError(err)
			return
		}
		ss, err := m.st.ListServers()
		if err != nil {
			m.setError(err)
			return
		}
		now := time.Now().UnixMilli()
		valid := make(map[int64]bool, len(ss))
		for i, s := range ss {
			valid[s.ID] = s.Enabled
			if nextDue[s.ID] == 0 {
				nextDue[s.ID] = now + int64(i%240)*250
			}
			if cfg.IntervalSeconds > 0 && (cfg.IntervalSeconds != c.IntervalSeconds || cfg.SmartBackoff != c.SmartBackoff || cfg.MaxBackoffHours != c.MaxBackoffHours) {
				deadline := now + nextDelay(c, failures[s.ID], failures[s.ID]).Milliseconds()
				if nextDue[s.ID] > deadline {
					nextDue[s.ID] = deadline
				}
			}
		}
		for id := range nextDue {
			if !valid[id] {
				delete(nextDue, id)
				delete(failures, id)
			}
		}
		m.mu.Lock()
		for id := range m.queued {
			if !valid[id] {
				delete(m.queued, id)
			}
		}
		// Bound reference cache by the current server/domain set, pruning old and
		// deleted configuration keys on every refresh (not just by wall time).
		allowedServers := make(map[string]bool, len(ss))
		for _, s := range ss {
			if s.Enabled && s.Trusted {
				allowedServers[fmt.Sprintf("%d\x00%d\x00%s", s.ID, s.TrustEpoch, s.Address)] = true
			}
		}
		allowedDomains := make(map[string]bool, len(c.Domains))
		for _, d := range c.Domains {
			allowedDomains[d.Name+"\x00"+d.Type] = true
		}
		for k, v := range m.cache {
			parts := strings.Split(k, "\x00")
			validKey := len(parts) == 5 && allowedServers[parts[0]+"\x00"+parts[1]+"\x00"+parts[2]] && allowedDomains[parts[3]+"\x00"+parts[4]]
			if !validKey || now-v.result.Timestamp > int64(c.ReferenceTTLSeconds)*1000 {
				delete(m.cache, k)
			}
		}
		for key, state := range m.referenceCollections {
			if !allowedDomains[key] && state.done == nil {
				delete(m.referenceCollections, key)
			}
		}
		m.updateBatchLocked(valid, c.Concurrency == 0)
		m.config = c
		m.status.Paused = c.Concurrency == 0
		m.signalLocked()
		m.mu.Unlock()
		if now >= pruneAt || cfg.ReferenceHistoryHours != c.ReferenceHistoryHours {
			m.setError(m.st.PruneTrustedReferences(now, c.ReferenceHistoryHours))
			pruneAt = now + time.Minute.Milliseconds()
		}
		cfg = c
		servers = ss
		serverByID = make(map[int64]model.Server, len(ss))
		for i := range ss {
			serverByID[ss[i].ID] = ss[i]
		}
	}
	refresh()
	dispatch := func() {
		if cfg.Concurrency == 0 {
			return
		}
		now := time.Now().UnixMilli()
		// Manual requests first, then the oldest deadline. This avoids starving
		// later servers when the configured interval is shorter than all work.
		candidates := append([]model.Server(nil), servers...)
		m.mu.Lock()
		sort.SliceStable(candidates, func(i, j int) bool {
			a, b := candidates[i].ID, candidates[j].ID
			manualA := m.queued[a] || m.batchWaitingLocked(a)
			manualB := m.queued[b] || m.batchWaitingLocked(b)
			if manualA != manualB {
				return manualA
			}
			return nextDue[a] < nextDue[b]
		})
		for _, s := range candidates {
			if len(m.inFlight) >= cfg.Concurrency {
				break
			}
			if !s.Enabled || m.inFlight[s.ID] || (!m.queued[s.ID] && !m.batchWaitingLocked(s.ID) && nextDue[s.ID] > now) {
				continue
			}
			refreshID := int64(0)
			if m.batchWaitingLocked(s.ID) {
				m.batch[s.ID] = true
				refreshID = m.status.Refresh.ID
			}
			delete(m.queued, s.ID)
			m.inFlight[s.ID] = true
			previous := failures[s.ID]
			refs := append([]model.Server(nil), servers...)
			workers.Add(1)
			go func(s model.Server, c model.Config, previous int, refs []model.Server, refreshID int64) {
				defer workers.Done()
				result := m.runRound(ctx, s, c, refs, previous)
				result.refreshID = refreshID
				select {
				case m.done <- result:
				case <-ctx.Done():
				}
			}(s, cfg, previous, refs, refreshID)
		}
		m.mu.Unlock()
	}
	dispatch()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	refreshAt := time.Now().Add(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.cancelBatchLocked("程序停止，本轮刷新未完成")
			m.mu.Unlock()
			workers.Wait()
			return
		case completed := <-m.done:
			m.mu.Lock()
			delete(m.inFlight, completed.serverID)
			m.completeBatchLocked(completed)
			m.mu.Unlock()
			previous := failures[completed.serverID]
			if complete, ok := serverByID[completed.serverID]; ok {
				if previous == 0 && completed.failures > 0 {
					log.Printf("服务器探测失败，进入退避: id=%d name=%s address=%s failures=%d", complete.ID, complete.Name, complete.Address, completed.failures)
				} else if previous > 0 && completed.failures == 0 {
					log.Printf("服务器恢复可用: id=%d name=%s address=%s", complete.ID, complete.Name, complete.Address)
				}
			}
			nextDue[completed.serverID] = completed.nextDue
			failures[completed.serverID] = completed.failures
			dispatch()
		case <-m.wake:
			refresh()
			refreshAt = time.Now().Add(5 * time.Second)
			dispatch()
		case <-tick.C:
			if !time.Now().Before(refreshAt) {
				refresh()
				refreshAt = time.Now().Add(5 * time.Second)
			}
			dispatch()
		}
	}
}

func (m *Monitor) runRound(ctx context.Context, server model.Server, cfg model.Config, servers []model.Server, previousFailures int) completion {
	round := model.Round{ServerID: server.ID, StartedAt: time.Now().UnixMilli()}
	for _, domain := range cfg.Domains {
		if ctx.Err() != nil {
			break
		}
		// Server removal/disable takes effect before each next domain.
		current, err := m.st.GetServer(server.ID)
		if err != nil || !current.Enabled || current.Address != server.Address {
			break
		}
		server = current
		result, err := m.lookup(ctx, server, domain, cfg, false)
		if err != nil {
			if result.ServerID != 0 {
				result.Pollution = "unknown"
				result.Reason = err.Error()
				round.Results = append(round.Results, result)
			}
			break
		}
		if result.Trusted {
			round.Results = append(round.Results, result)
			continue
		}
		referenceErr := m.classify(ctx, &result, domain, cfg)
		round.Results = append(round.Results, result)
		if referenceErr != nil {
			break
		}

	}
	round.FinishedAt = time.Now().UnixMilli()
	offline := len(round.Results) > 0
	for _, r := range round.Results {
		if r.Received {
			offline = false
			break
		}
	}
	failures := 0
	if offline {
		failures = previousFailures + 1
	}
	if ctx.Err() != nil {
		failures = previousFailures
	}
	delay := nextDelay(cfg, failures, previousFailures)
	round.NextDue = round.FinishedAt + delay.Milliseconds()
	completed := completion{serverID: server.ID, nextDue: round.NextDue, failures: failures}
	if len(round.Results) > 0 {
		if err := m.st.SaveRound(round); err != nil {
			m.setError(err)
			completed.error = "保存探测结果失败: " + err.Error()
		}
	}
	return completed
}

func nextDelay(cfg model.Config, failures, previousFailures int) time.Duration {
	base := time.Duration(cfg.IntervalSeconds) * time.Second
	delay := base
	ceiling := time.Duration(cfg.MaxBackoffHours * float64(time.Hour))
	if ceiling < base {
		ceiling = base
	}
	if cfg.SmartBackoff && cfg.MaxBackoffHours > 0 && failures >= 3 {
		for i := 2; i < failures && delay < ceiling; i++ {
			delay *= 2
			if delay > ceiling {
				delay = ceiling
			}
		}
	}
	if failures == 0 && previousFailures >= 3 && delay > 30*time.Second {
		delay = 30 * time.Second
	}
	// Jitter spreads both retries and regular queries; never exceed the selected
	// maximum backoff and never schedule tighter than the minimum 5s interval.
	delay = time.Duration(float64(delay) * (0.90 + rand.Float64()*0.10))
	if delay < 5*time.Second {
		delay = 5 * time.Second
	}
	return delay
}

func cacheKey(s model.Server, d model.Domain) string {
	return fmt.Sprintf("%d\x00%d\x00%s\x00%s\x00%s", s.ID, s.TrustEpoch, s.Address, d.Name, d.Type)
}

func (m *Monitor) acquire(ctx context.Context, serverID int64) error {
	for {
		m.mu.Lock()
		if m.config.Concurrency == 0 {
			m.mu.Unlock()
			return errors.New("探测已暂停")
		}
		if m.status.Active < m.config.Concurrency && !m.busy[serverID] {
			m.status.Active++
			m.busy[serverID] = true
			m.mu.Unlock()
			return nil
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
func (m *Monitor) release(serverID int64) {
	m.mu.Lock()
	m.status.Active--
	delete(m.busy, serverID)
	m.signalLocked()
	m.mu.Unlock()
}

func (m *Monitor) lookup(ctx context.Context, server model.Server, domain model.Domain, cfg model.Config, reference bool) (model.ProbeResult, error) {
	key := cacheKey(server, domain)
	cacheGet := func() (model.ProbeResult, bool) {
		m.mu.Lock()
		defer m.mu.Unlock()
		v, ok := m.cache[key]
		now := time.Now().UnixMilli()
		valid := ok && v.result.Success && len(v.result.Records) > 0 && cfg.ReferenceTTLSeconds > 0 && now-v.result.Timestamp < int64(cfg.ReferenceTTLSeconds)*1000
		for _, r := range v.result.Records {
			if r.TTLSeconds <= 0 || r.ExpiresAt <= now {
				valid = false
				break
			}
		}
		return v.result, valid
	}
	if reference {
		if result, ok := cacheGet(); ok {
			return result, nil
		}
	}
	if err := m.acquire(ctx, server.ID); err != nil {
		return model.ProbeResult{}, err
	}
	defer m.release(server.ID)
	// A concurrent target/reference lookup may have populated the cache while
	// waiting for the per-server gate. Recheck before spawning another process.
	if reference {
		if result, ok := cacheGet(); ok {
			return result, nil
		}
	}
	if ctx.Err() != nil {
		return model.ProbeResult{}, ctx.Err()
	}
	m.mu.Lock()
	generation := m.cacheGeneration
	m.mu.Unlock()
	if server.Trusted {
		current, err := m.st.GetServer(server.ID)
		if err != nil || !current.Enabled || !current.Trusted || current.Address != server.Address || current.TrustEpoch != server.TrustEpoch {
			return model.ProbeResult{}, errors.New("可信来源配置已变化，需重新采集")
		}
	}
	result := m.probe(ctx, m.doggoPath, server, domain, time.Duration(cfg.TimeoutSeconds)*time.Second)
	result.PolicyVersion = 2
	result.Timestamp = time.Now().UnixMilli()
	if len(result.Records) == 0 {
		for _, value := range result.Answers {
			if normalized := normalizeAnswer(domain.Type, value); normalized != "" {
				result.Records = append(result.Records, model.AnswerRecord{Value: normalized})
			}
		}
	}
	for i := range result.Records {
		record := &result.Records[i]
		if record.TTLSeconds < 0 {
			record.TTLSeconds = 0
		}
		if record.TTLSeconds > 86400 {
			record.TTLSeconds = 86400
		}
		record.ObservedAt = result.Timestamp
		record.ExpiresAt = result.Timestamp + record.TTLSeconds*1000
	}
	var sourceErr error
	if server.Trusted {
		current, err := m.st.GetServer(server.ID)
		if err != nil || !current.Enabled || !current.Trusted || current.Address != server.Address || current.TrustEpoch != server.TrustEpoch {
			sourceErr = errors.New("可信来源配置已变化，丢弃在途参考")
		}
	}
	if server.Trusted && sourceErr == nil {
		result.Trusted = true
		result.Pollution = "clean"
		result.Reason = "用户可信 DNS，不参与污染定罪"
	}
	if server.Trusted && sourceErr == nil {
		m.mu.Lock()
		if generation == m.cacheGeneration {
			// Save only actual observations, never a cache hit. Store rechecks trust epoch
			// within its transaction to reject revoke/re-enable and address-change races.
			if result.Success {
				if err := m.st.SaveTrustedObservation(server, result); err != nil {
					m.status.LastError = err.Error()
					sourceErr = fmt.Errorf("保存可信参考失败: %w", err)
				}
			}
			if sourceErr == nil {
				m.cache[key] = cachedProbe{result: result}
			}
		}
		// Cache bounds also apply to large TXT answers and large server lists.
		for {
			total := 0
			var oldestKey string
			var oldest int64
			for k, v := range m.cache {
				total += len(v.result.Raw) + len(v.result.Error) + len(v.result.Records)*128 + 256
				for _, a := range v.result.Answers {
					total += len(a)
				}
				if oldestKey == "" || v.result.Timestamp < oldest {
					oldestKey = k
					oldest = v.result.Timestamp
				}
			}
			if len(m.cache) <= 512 && total <= 8*1024*1024 {
				break
			}
			delete(m.cache, oldestKey)
		}
		m.mu.Unlock()
	}
	if reference {
		finished := time.Now().UnixMilli()
		m.setError(m.st.SaveRound(model.Round{Auxiliary: true, ServerID: server.ID, StartedAt: result.Timestamp, FinishedAt: finished, NextDue: finished + int64(cfg.IntervalSeconds)*1000, Results: []model.ProbeResult{result}}))
	}
	return result, sourceErr
}
