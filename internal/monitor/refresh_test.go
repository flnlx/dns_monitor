package monitor

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func waitRefreshTest(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for monitor state")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func fakeResult(s model.Server, d model.Domain) model.ProbeResult {
	return model.ProbeResult{Timestamp: time.Now().UnixMilli(), ServerID: s.ID, Domain: d.Name, Type: d.Type, Received: true, Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.8"}}
}

func TestQueueAllSchedulesAfterExistingRound(t *testing.T) {
	st := testStore(t)
	cfg := model.DefaultConfig()
	cfg.Concurrency = 1
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	s, err := st.SaveServer(model.Server{Name: "target", Address: "udp://192.0.2.1", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	started := make(chan int32, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	m.probe = func(ctx context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		started <- calls.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return fakeResult(s, d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { m.Run(ctx); close(finished) }()
	defer func() { cancel(); <-finished }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first round missing")
	}
	status, err := m.QueueAll()
	if err != nil || !status.Pending || status.Total != 1 || status.Completed != 0 {
		t.Fatalf("%+v %v", status, err)
	}
	again, err := m.QueueAll()
	if err != nil || again.ID != status.ID {
		t.Fatalf("duplicate batch: %+v %v", again, err)
	}
	// The already-running round must not be counted as the requested fresh round.
	release <- struct{}{}
	select {
	case n := <-started:
		if n != 2 {
			t.Fatal(n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fresh round not scheduled")
	}
	if current := m.Status().Refresh; current == nil || !current.Pending || current.Completed != 0 {
		t.Fatalf("old round completed batch: %+v", current)
	}
	release <- struct{}{}
	waitRefreshTest(t, func() bool { return !m.Status().Refresh.Pending })
	current := m.Status().Refresh
	if current.Completed != 1 || current.Error != "" || calls.Load() != 2 {
		t.Fatalf("%+v calls=%d", current, calls.Load())
	}
	results, err := st.Results(s.ID, 10, 0)
	if err != nil || len(results) != 2 {
		t.Fatalf("results=%d error=%v", len(results), err)
	}
	current.Completed = 99
	if m.Status().Refresh.Completed != 1 {
		t.Fatal("status exposes mutable shared pointer")
	}
}

func TestQueueAllExceedsSingleQueueLimitAndAccountsRemoved(t *testing.T) {
	st := testStore(t)
	cfg := model.DefaultConfig()
	cfg.Concurrency = 5
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	const total = maxQueued + 5
	servers := make([]model.Server, total)
	for i := range servers {
		s, err := st.SaveServer(model.Server{Name: fmt.Sprintf("dns%d", i), Address: fmt.Sprintf("udp://192.0.%d.%d", i/250, i%250+1), Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		servers[i] = s
	}
	m := New(st, "")
	status, err := m.QueueAll()
	if err != nil || status.Total != total {
		t.Fatalf("batch was truncated: %+v %v", status, err)
	}
	if err := st.DeleteServer(servers[0].ID); err != nil {
		t.Fatal(err)
	}
	servers[1].Enabled = false
	if _, err := st.SaveServer(servers[1]); err != nil {
		t.Fatal(err)
	}
	var calls, active, maxActive atomic.Int32
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		n := active.Add(1)
		for old := maxActive.Load(); n > old && !maxActive.CompareAndSwap(old, n); old = maxActive.Load() {
		}
		calls.Add(1)
		defer active.Add(-1)
		return fakeResult(s, d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { m.Run(ctx); close(finished) }()
	defer func() { cancel(); <-finished }()
	waitRefreshTest(t, func() bool { return !m.Status().Refresh.Pending })
	current := m.Status().Refresh
	if current.Completed != total || current.Error == "" || calls.Load() != total-2 || maxActive.Load() > 5 {
		t.Fatalf("batch did not finish all enabled members: %+v calls=%d concurrency=%d", current, calls.Load(), maxActive.Load())
	}
}

func TestQueueAllPauseEndsBatchWithError(t *testing.T) {
	st := testStore(t)
	cfg := model.DefaultConfig()
	cfg.Concurrency = 1
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := st.SaveServer(model.Server{Name: fmt.Sprint(i), Address: fmt.Sprintf("udp://192.0.2.%d", i+1), Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	m := New(st, "")
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	m.probe = func(ctx context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return fakeResult(s, d)
	}
	if _, err := m.QueueAll(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { m.Run(ctx); close(finished) }()
	defer func() { cancel(); <-finished }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("batch not started")
	}
	cfg.Concurrency = 0
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	m.Wake()
	waitRefreshTest(t, func() bool { return m.Status().Paused && !m.Status().Refresh.Pending })
	if status := m.Status().Refresh; status.Error == "" || status.Completed >= status.Total {
		t.Fatalf("pause falsely reported completion: %+v", status)
	}
	if _, err := m.QueueAll(); err == nil {
		t.Fatal("paused batch accepted")
	}
	close(release)
}

func TestRefreshInvalidatesInFlightReferenceCache(t *testing.T) {
	st := testStore(t)
	s, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	started := make(chan struct{})
	release := make(chan struct{})
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		close(started)
		<-release
		return fakeResult(s, d)
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		if _, err := m.lookup(context.Background(), s, m.config.Domains[0], m.config, false); err != nil {
			t.Error(err)
		}
	}()
	<-started
	if _, err := m.QueueAll(); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-finished
	m.mu.Lock()
	cacheSize := len(m.cache)
	m.mu.Unlock()
	if cacheSize != 0 {
		t.Fatal("pre-refresh probe refilled invalidated reference cache")
	}
}

func TestTrustedTargetsSkipComparisonEvenWhenOffline(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			st := testStore(t)
			cfg := model.DefaultConfig()
			target, err := st.SaveServer(model.Server{Name: "trusted target", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
			if err != nil {
				t.Fatal(err)
			}
			other, err := st.SaveServer(model.Server{Name: "other trusted", Address: "udp://192.0.2.2", Enabled: true, Trusted: true})
			if err != nil {
				t.Fatal(err)
			}
			m := New(st, "")
			calls := 0
			m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
				calls++
				r := fakeResult(s, d)
				r.Success = success
				r.Received = success
				return r
			}
			m.runRound(context.Background(), target, cfg, []model.Server{target, other}, 0)
			results, err := st.Results(target.ID, 10, 0)
			if err != nil || len(results) != 1 {
				t.Fatalf("%v %v", results, err)
			}
			if calls != 1 || results[0].Pollution != "clean" || !results[0].Trusted || len(results[0].References) != 0 {
				t.Fatalf("calls=%d result=%+v", calls, results[0])
			}
		})
	}
}

func TestAddressKeyNormalizesStampsAndPreservesIdentity(t *testing.T) {
	for _, pair := range [][2]string{
		{"1.1.1.1", "udp://1.1.1.1:53"},
		{"https://DNS.Example.COM:443", "https://dns.example.com/dns-query"},
		{stampFixture(0, []byte("1.1.1.1:53")), "udp://1.1.1.1"},
		{stampFixture(2, nil, nil, []byte("DNS.Example.COM:443"), []byte("/dns-query")), "https://dns.example.com/dns-query"},
		{stampFixture(3, nil, nil, []byte("dns.example.com:853")), "tls://dns.example.com"},
		{stampFixture(4, nil, nil, []byte("dns.example.com:853")), "quic://dns.example.com"},
		{stampFixture(1, []byte("192.0.2.1:443"), make([]byte, 32), []byte("2.dnscrypt-cert.EXAMPLE.com")), stampFixture(1, []byte("192.0.2.1"), make([]byte, 32), []byte("2.dnscrypt-cert.example.com"))},
	} {
		if !sameEndpoint(pair[0], pair[1]) {
			t.Fatalf("aliases differ: %v", pair)
		}
	}
	if _, err := AddressKey("https://[::1]/dns-query"); err == nil {
		t.Fatal("invalid endpoint key accepted")
	}
	if sameEndpoint("https://dns.example.com/dns-query?a=1", "https://dns.example.com/dns-query?a=2") {
		t.Fatal("query strings ignored")
	}
}
