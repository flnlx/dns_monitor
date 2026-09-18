package monitor

import (
	"context"
	"errors"
	"fmt"
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
	serverID int64
	nextDue  int64
	failures int
}

type Monitor struct {
	st        *store.Store
	doggoPath string
	wake      chan struct{}
	done      chan completion
	mu        sync.Mutex
	config    model.Config
	status    model.RuntimeStatus
	queued    map[int64]bool
	inFlight  map[int64]bool
	busy      map[int64]bool
	cache     map[string]cachedProbe
	changed   chan struct{}
	probe     func(context.Context, string, model.Server, model.Domain, time.Duration) model.ProbeResult
}

func New(st *store.Store, doggoPath string) *Monitor {
	return &Monitor{st: st, doggoPath: doggoPath, wake: make(chan struct{}, 1), done: make(chan completion, 50), config: model.DefaultConfig(), queued: make(map[int64]bool), inFlight: make(map[int64]bool), busy: make(map[int64]bool), cache: make(map[string]cachedProbe), changed: make(chan struct{}), probe: runDoggo}
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

func (m *Monitor) Status() model.RuntimeStatus { m.mu.Lock(); defer m.mu.Unlock(); return m.status }
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
	var cfg model.Config
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
				allowedServers[fmt.Sprintf("%d\x00%s", s.ID, s.Address)] = true
			}
		}
		allowedDomains := make(map[string]bool, len(c.Domains))
		for _, d := range c.Domains {
			allowedDomains[d.Name+"\x00"+d.Type] = true
		}
		for k, v := range m.cache {
			parts := strings.Split(k, "\x00")
			validKey := len(parts) == 4 && allowedServers[parts[0]+"\x00"+parts[1]] && allowedDomains[parts[2]+"\x00"+parts[3]]
			if !validKey || now-v.result.Timestamp > int64(c.ReferenceTTLSeconds)*1000 {
				delete(m.cache, k)
			}
		}
		m.config = c
		m.status.Paused = c.Concurrency == 0
		m.signalLocked()
		m.mu.Unlock()
		cfg = c
		servers = ss
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
			if m.queued[a] != m.queued[b] {
				return m.queued[a]
			}
			return nextDue[a] < nextDue[b]
		})
		for _, s := range candidates {
			if len(m.inFlight) >= cfg.Concurrency {
				break
			}
			if !s.Enabled || m.inFlight[s.ID] || (!m.queued[s.ID] && nextDue[s.ID] > now) {
				continue
			}
			delete(m.queued, s.ID)
			m.inFlight[s.ID] = true
			previous := failures[s.ID]
			refs := append([]model.Server(nil), servers...)
			workers.Add(1)
			go func(s model.Server, c model.Config, previous int, refs []model.Server) {
				defer workers.Done()
				result := m.runRound(ctx, s, c, refs, previous)
				select {
				case m.done <- result:
				case <-ctx.Done():
				}
			}(s, cfg, previous, refs)
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
			workers.Wait()
			return
		case completed := <-m.done:
			m.mu.Lock()
			delete(m.inFlight, completed.serverID)
			m.mu.Unlock()
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
		result, err := m.lookup(ctx, server, domain, cfg, false)
		if err != nil {
			break
		}
		refs := make([]model.Reference, 0)
		for _, reference := range servers {
			if !reference.Enabled || !reference.Trusted || reference.ID == server.ID || sameEndpoint(reference.Address, server.Address) {
				continue
			}
			current, err := m.st.GetServer(reference.ID)
			if err != nil || !current.Enabled || !current.Trusted || current.Address != reference.Address {
				continue
			}
			ref, err := m.lookup(ctx, reference, domain, cfg, true)
			if err != nil {
				continue
			}
			refs = append(refs, model.Reference{ServerID: reference.ID, Address: reference.Address, Timestamp: ref.Timestamp, Rcode: ref.Rcode, Answers: ref.Answers, Success: ref.Success, Error: ref.Error})
		}
		compare(&result, refs)
		round.Results = append(round.Results, result)
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
	if len(round.Results) > 0 {
		m.setError(m.st.SaveRound(round))
	}
	return completion{serverID: server.ID, nextDue: round.NextDue, failures: failures}
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
	return fmt.Sprintf("%d\x00%s\x00%s\x00%s", s.ID, s.Address, d.Name, d.Type)
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
		return v.result, ok && cfg.ReferenceTTLSeconds > 0 && time.Now().UnixMilli()-v.result.Timestamp < int64(cfg.ReferenceTTLSeconds)*1000
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
	result := m.probe(ctx, m.doggoPath, server, domain, time.Duration(cfg.TimeoutSeconds)*time.Second)
	if server.Trusted {
		m.mu.Lock()
		m.cache[key] = cachedProbe{result: result}
		// Cache bounds also apply to large TXT answers and large server lists.
		for {
			total := 0
			var oldestKey string
			var oldest int64
			for k, v := range m.cache {
				total += len(v.result.Raw) + len(v.result.Error) + 256
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
	return result, nil
}
