package monitor

import (
	"context"
	"database/sql"
	"dnsmonitor/internal/model"
	"dnsmonitor/internal/store"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func referenceFixture(now int64, ttl int64, age time.Duration, ips ...string) model.Reference {
	r := model.Reference{ServerID: 1, Timestamp: now - age.Milliseconds(), Success: true, Rcode: "NOERROR", Answers: ips}
	for _, ip := range ips {
		r.Records = append(r.Records, model.AnswerRecord{Value: ip, TTLSeconds: ttl, ObservedAt: r.Timestamp, ExpiresAt: r.Timestamp + ttl*1000})
	}
	return r
}

func TestFourLevelsUseFreshHistoryAndNumericPrefix(t *testing.T) {
	now := time.Now().UnixMilli()
	fresh := referenceFixture(now, 60, 0, "57.144.152.1", "157.240.1.1")
	history := referenceFixture(now, 5, 30*time.Minute, "57.144.186.1")
	expired := referenceFixture(now, 5, 2*time.Hour, "192.0.2.1")
	for _, test := range []struct {
		name  string
		ips   []string
		refs  []model.Reference
		hours float64
		want  string
	}{
		{"fresh subset", []string{"57.144.152.1"}, []model.Reference{fresh}, 1, "matched"},
		{"fresh union", []string{"57.144.152.1", "157.240.1.1"}, []model.Reference{fresh}, 1, "matched"},
		{"historical rotation", []string{"57.144.186.1"}, []model.Reference{fresh, history}, 1, "clean"},
		{"historical and fresh worst", []string{"57.144.152.1", "57.144.186.1"}, []model.Reference{fresh, history}, 1, "clean"},
		{"historical disabled", []string{"57.144.186.1"}, []model.Reference{fresh, history}, 0, "suspicious"},
		{"new same prefix", []string{"57.144.9.9"}, []model.Reference{fresh, history}, 1, "suspicious"},
		{"new foreign prefix", []string{"198.51.100.1"}, []model.Reference{fresh}, 1, "polluted"},
		{"worst IP", []string{"57.144.152.1", "57.144.9.9", "198.51.100.1"}, []model.Reference{fresh}, 1, "polluted"},
		{"prefix not text prefix", []string{"1.10.2.3"}, []model.Reference{referenceFixture(now, 60, 0, "1.1.2.3")}, 1, "polluted"},
		{"expired reference", []string{"192.0.2.1"}, []model.Reference{expired}, 1, "unknown"},
		{"longer history", []string{"192.0.2.1"}, []model.Reference{expired}, 3, "clean"},
		{"zero TTL historical", []string{"192.0.2.1"}, []model.Reference{referenceFixture(now, 0, 0, "192.0.2.1")}, 1, "clean"},
		{"zero TTL history off", []string{"192.0.2.1"}, []model.Reference{referenceFixture(now, 0, 0, "192.0.2.1")}, 0, "unknown"},
		{"empty reference", []string{"192.0.2.1"}, nil, 1, "unknown"},
		{"no A", nil, []model.Reference{fresh}, 1, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: test.ips}
			compareAt(&r, test.refs, now, test.hours)
			if r.Pollution != test.want || r.PolicyVersion != 2 {
				t.Fatalf("want %s, got %+v", test.want, r)
			}
		})
	}
	for _, test := range []model.ProbeResult{{Type: "TXT", Success: true, Rcode: "NOERROR", Answers: []string{"text"}}, {Type: "A", Success: true, Rcode: "NXDOMAIN"}, {Type: "A", Success: false, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}} {
		compareAt(&test, []model.Reference{fresh}, now, 1)
		if test.Pollution != "unknown" {
			t.Fatal("noncomparable result convicted", test)
		}
	}
	failed := fresh
	failed.Success = false
	r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
	compareAt(&r, []model.Reference{failed}, now, 1)
	if r.Pollution != "unknown" {
		t.Fatal("failed reference used", r)
	}
}

func TestTTLandAnswerOwnerChain(t *testing.T) {
	for raw, want := range map[string]int64{`"6s"`: 6, `"1m2s"`: 62, `60`: 60, `"60"`: 60, `"0s"`: 0, `"-1s"`: 0, `-1`: 0, `null`: 0, `"broken"`: 0, `"1.5s"`: 0, `"48h"`: 86400, `4294967295`: 86400} {
		if got := recordTTL(json.RawMessage(raw)); got != want {
			t.Fatalf("%s: %d != %d", raw, got, want)
		}
	}
	domain := model.Domain{Name: "www.example.com", Type: "A"}
	for _, test := range []struct {
		name, answers string
		wantTTL       int64
		count         int
	}{
		{"CNAME cannot extend A", `{"name":"www.example.com.","type":"CNAME","address":"edge.example.com.","ttl":"1h"},{"name":"edge.example.com.","type":"A","address":"192.0.2.1","ttl":"6s"},{"name":"unrelated.example.com.","type":"A","address":"198.51.100.1","ttl":"1h"}`, 6, 1},
		{"short alias caps A", `{"name":"www.example.com.","type":"CNAME","address":"edge.example.com.","ttl":"4s"},{"name":"edge.example.com.","type":"A","address":"192.0.2.1","ttl":"1h"}`, 4, 1},
		{"missing alias TTL", `{"name":"www.example.com.","type":"CNAME","address":"edge.example.com."},{"name":"edge.example.com.","type":"A","address":"192.0.2.1","ttl":"1h"}`, 0, 1},
		{"unrelated only", `{"name":"unrelated.example.com.","type":"A","address":"192.0.2.1","ttl":"1h"}`, 0, 0},
		{"cycle", `{"name":"www.example.com.","type":"CNAME","address":"www.example.com.","ttl":"1h"}`, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := `{"responses":[{"questions":[{"name":"www.example.com.","type":"A"}],"answers":[` + test.answers + `]}]}`
			r, err := decodeDoggo([]byte(data), domain)
			if err != nil || len(r.Records) != test.count {
				t.Fatalf("%+v %v", r, err)
			}
			if test.count > 0 && (r.Records[0].TTLSeconds != test.wantTTL || r.Records[0].ExpiresAt-r.Records[0].ObservedAt != test.wantTTL*1000) {
				t.Fatal(r)
			}
		})
	}
}

func TestReferenceHistoryConfig(t *testing.T) {
	for _, hours := range []float64{0, 0.25, 1, 720} {
		cfg := model.DefaultConfig()
		cfg.ReferenceHistoryHours = hours
		if err := ValidateConfig(&cfg); err != nil {
			t.Fatal(err)
		}
	}
	for _, hours := range []float64{-1, 720.1, math.NaN(), math.Inf(1)} {
		cfg := model.DefaultConfig()
		cfg.ReferenceHistoryHours = hours
		if ValidateConfig(&cfg) == nil {
			t.Fatal("accepted invalid history", hours)
		}
	}
}

func TestSharedColdStartUsesExistingPoolWithoutRecheck(t *testing.T) {
	st := testStore(t)
	source, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	cfg := model.DefaultConfig()
	cfg.ReferenceTTLSeconds = 0
	m := New(st, "")
	m.config = cfg
	var calls, active, maxActive atomic.Int32
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		calls.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); n > old && !maxActive.CompareAndSwap(old, n); old = maxActive.Load() {
		}
		time.Sleep(5 * time.Millisecond)
		return fakeResult(s, d)
	}
	domain := cfg.Domains[0]
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
			if err := m.classify(context.Background(), &r, domain, cfg); err != nil || r.Pollution != "polluted" {
				t.Errorf("%+v %v", r, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 || maxActive.Load() > int32(cfg.Concurrency) {
		t.Fatalf("shared budget exceeded: %d calls %d active", calls.Load(), maxActive.Load())
	}
	m.mu.Lock()
	m.referenceCollections[domain.Name+"\x00"+domain.Type].nextAt = 0
	m.mu.Unlock()
	r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
	if err := m.classify(context.Background(), &r, domain, cfg); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("existing pool triggered redundant collection", calls.Load())
	}
	// A separate domain cannot inherit another domain's observed pool.
	other := model.Domain{Name: "other.example", Type: "A"}
	refs, err := st.TrustedReferences(other, time.Now().UnixMilli(), 1)
	if err != nil || len(refs) != 0 {
		t.Fatal("domain pool leak", refs, err)
	}
	before, err := st.TrustedReferences(domain, time.Now().UnixMilli(), 1)
	if err != nil || len(before) != 1 || before[0].ServerID != source.ID {
		t.Fatal(before, err)
	}
	if _, err := m.QueueAll(); err != nil {
		t.Fatal(err)
	}
	after, err := st.TrustedReferences(domain, time.Now().UnixMilli(), 1)
	if err != nil || len(after) != len(before) {
		t.Fatal("refresh erased observation history", after, err)
	}
}

func TestExistingPoolClassifiesWithoutOfflineSourceOrForcedRecheck(t *testing.T) {
	st := testStore(t)
	source, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.SaveServer(model.Server{Name: "offline", Address: "udp://192.0.2.2", Enabled: true, Trusted: true}); err != nil {
		t.Fatal(err)
	}
	cfg := model.DefaultConfig()
	domain := cfg.Domains[0]
	now := time.Now().UnixMilli()
	history := fakeResult(source, domain)
	history.Records = []model.AnswerRecord{{Value: "192.0.2.8", TTLSeconds: 1, ObservedAt: now - 60000, ExpiresAt: now - 59000}, {Value: "192.0.2.9", TTLSeconds: 60, ObservedAt: now, ExpiresAt: now + 60000}}
	if err := st.SaveTrustedObservation(source, history); err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	m.probe = func(_ context.Context, _ string, _ model.Server, _ model.Domain, _ time.Duration) model.ProbeResult {
		t.Error("existing pool must not query any reference, including the offline source")
		return model.ProbeResult{}
	}
	for _, test := range []struct{ answer, want string }{
		{"192.0.2.9", "matched"}, {"192.0.2.8", "clean"},
		{"192.0.100.1", "suspicious"}, {"198.51.100.1", "polluted"},
	} {
		r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{test.answer}}
		if err := m.classify(context.Background(), &r, domain, cfg); err != nil || r.Pollution != test.want {
			t.Fatalf("want %s: %+v %v", test.want, r, err)
		}
	}
}

func TestColdStartStopsAtFirstUsableSourceAndRateLimitsEmptyPool(t *testing.T) {
	for _, usable := range []bool{false, true} {
		t.Run(fmt.Sprint(usable), func(t *testing.T) {
			st := testStore(t)
			for i := 1; i <= 3; i++ {
				if _, err := st.SaveServer(model.Server{Name: fmt.Sprint(i), Address: fmt.Sprintf("udp://192.0.2.%d", i), Enabled: true, Trusted: true}); err != nil {
					t.Fatal(err)
				}
			}
			m := New(st, "")
			var calls atomic.Int32
			m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
				calls.Add(1)
				r := fakeResult(s, d)
				r.Success = usable && s.ID == 2
				if !r.Success {
					r.Answers = nil
					r.Error = "timeout"
				}
				return r
			}
			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
					want := "unknown"
					if usable {
						want = "polluted"
					}
					if err := m.classify(context.Background(), &r, m.config.Domains[0], m.config); err != nil || r.Pollution != want {
						t.Errorf("want %s: %+v %v", want, r, err)
					}
				}()
			}
			wg.Wait()
			wantCalls := int32(3)
			if usable {
				wantCalls = 2
			}
			if calls.Load() != wantCalls {
				t.Fatalf("shared cold-start queried %d times, want %d", calls.Load(), wantCalls)
			}
		})
	}
}

func TestRevokedInFlightSourceCannotRepopulate(t *testing.T) {
	st := testStore(t)
	source, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	started, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		close(started)
		<-release
		return fakeResult(s, d)
	}
	go func() {
		_, err := m.lookup(context.Background(), source, m.config.Domains[0], m.config, true)
		finished <- err
	}()
	<-started
	source.Trusted = false
	source, err = st.SaveServer(source)
	if err != nil {
		t.Fatal(err)
	}
	source.Trusted = true
	_, err = st.SaveServer(source)
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-finished; err == nil {
		t.Fatal("old epoch result accepted")
	}
	refs, err := st.TrustedReferences(m.config.Domains[0], time.Now().UnixMilli(), 1)
	if err != nil || len(refs) != 0 || len(m.cache) != 0 {
		t.Fatal("revoked observation resurrected", refs, err)
	}
	raw, err := st.Results(source.ID, 10, 0)
	if err != nil || len(raw) != 1 {
		t.Fatal("completed raw evidence lost", raw, err)
	}
}

func TestFailedObservationWriteCannotConvict(t *testing.T) {
	dbpath := filepath.Join(t.TempDir(), "failure.db")
	st, err := store.Open(dbpath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	source, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate storage failure only in the trusted pool, keeping raw evidence writable.
	db, err := sql.Open("sqlite", dbpath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TRIGGER fail_observation BEFORE INSERT ON trusted_observations BEGIN SELECT RAISE(ABORT,'fixture disk write failure'); END`); err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		return fakeResult(s, d)
	}
	r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
	if err = m.classify(context.Background(), &r, m.config.Domains[0], m.config); err == nil || r.Pollution != "unknown" {
		t.Fatalf("write failure did not block judgment: %+v %v", r, err)
	}
	results, err := st.Results(source.ID, 10, 0)
	if err != nil || len(results) != 1 {
		t.Fatal("raw reference evidence discarded", results, err)
	}
}

func TestPartialEligibleSourcePoolCanClassify(t *testing.T) {
	st := testStore(t)
	sources := make([]model.Server, 2)
	for i := range sources {
		var err error
		sources[i], err = st.SaveServer(model.Server{Name: fmt.Sprint(i), Address: fmt.Sprintf("udp://192.0.2.%d", i+1), Enabled: true, Trusted: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	m := New(st, "")
	cfg := model.DefaultConfig()
	cfg.ReferenceHistoryHours = 0
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		r := fakeResult(s, d)
		if s.ID == sources[1].ID {
			r.Records = []model.AnswerRecord{{Value: r.Answers[0], TTLSeconds: 60}}
		}
		return r
	}
	r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
	if err := m.classify(context.Background(), &r, cfg.Domains[0], cfg); err != nil || r.Pollution != "polluted" {
		t.Fatalf("partial TTL-eligible source set did not classify: %+v %v", r, err)
	}
}

func TestNewMonitorUsesPersistedHistoryDuringReferenceFailure(t *testing.T) {
	st := testStore(t)
	source, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		return fakeResult(s, d)
	}
	if _, err = m.lookup(context.Background(), source, m.config.Domains[0], m.config, true); err != nil {
		t.Fatal(err)
	}
	replacement := New(st, "")
	replacement.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		r := fakeResult(s, d)
		r.Success = false
		r.Answers = nil
		r.Error = "timeout"
		return r
	}
	r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.8"}}
	if err := replacement.classify(context.Background(), &r, replacement.config.Domains[0], replacement.config); err != nil || r.Pollution != "clean" {
		t.Fatalf("persisted positive history lost: %+v %v", r, err)
	}
}

func TestRefreshDuringEmptyPoolCollectionPreservesEvidenceAndRetries(t *testing.T) {
	st := testStore(t)
	source, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	domain := m.config.Domains[0]
	started, release := make(chan struct{}), make(chan struct{})
	calls := 0
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		calls++
		if calls == 1 {
			close(started)
			<-release
		}
		r := fakeResult(s, d)
		r.Answers = []string{"198.51.100.1"}
		return r
	}
	result := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
	finished := make(chan error, 1)
	go func() { finished <- m.classify(context.Background(), &result, domain, m.config) }()
	<-started
	if _, err := m.QueueAll(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-finished; err == nil || result.Pollution != "unknown" || calls != 1 {
		t.Fatalf("old generation entered the reference pool: %+v %v calls=%d", result, err, calls)
	}
	raw, err := st.Results(source.ID, 10, 0)
	if err != nil || len(raw) != 1 {
		t.Fatal("completed old-generation evidence lost", raw, err)
	}
	if err := m.classify(context.Background(), &result, domain, m.config); err != nil || result.Pollution != "clean" || calls != 2 {
		t.Fatalf("fresh collection not restored: %+v %v calls=%d", result, err, calls)
	}
}

func TestExpiryBetweenReadAndComparisonUsesRemainingEligibleSources(t *testing.T) {
	observed := time.Now().UnixMilli()
	a := referenceFixture(observed, 1, 0, "57.144.152.1")
	b := referenceFixture(observed, 60, 0, "198.51.100.1")
	b.ServerID = 2
	for _, test := range []struct {
		name         string
		at           int64
		history      float64
		answer, want string
	}{
		{"before expiry", observed + 999, 0, "57.144.152.1", "matched"},
		{"at expiry", observed + 1000, 0, "57.144.152.1", "polluted"},
		{"after expiry", observed + 1001, 0, "57.144.152.1", "polluted"},
		{"partial set can E", observed + 1001, 0, "198.51.100.2", "suspicious"},
		{"all expired", observed + 60000, 0, "198.51.100.2", "unknown"},
		{"history keeps source usable", observed + 1001, 1, "57.144.152.1", "clean"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{test.answer}}
			compareAt(&r, []model.Reference{a, b}, test.at, test.history)
			if r.Pollution != test.want || r.ComparedAt != test.at {
				t.Fatalf("want %s: %+v", test.want, r)
			}
		})
	}
}

func TestTrustedSourceAddedDuringColdStartDoesNotDelayVerdict(t *testing.T) {
	st := testStore(t)
	source, err := st.SaveServer(model.Server{Name: "first", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	domain := m.config.Domains[0]
	started, release := make(chan struct{}), make(chan struct{})
	calls := 0
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		calls++
		if calls == 1 {
			close(started)
			<-release
		}
		r := fakeResult(s, d)
		if s.ID != source.ID {
			r.Answers = []string{"198.51.100.1"}
		}
		return r
	}
	r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
	finished := make(chan error, 1)
	go func() { finished <- m.classify(context.Background(), &r, domain, m.config) }()
	<-started
	if _, err := st.SaveServer(model.Server{Name: "new trusted", Address: "udp://192.0.2.2", Enabled: true, Trusted: true}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-finished; err != nil || r.Pollution != "polluted" || calls != 1 {
		t.Fatalf("new trusted source blocked existing usable reference: %+v %v", r, err)
	}
	if err := m.classify(context.Background(), &r, domain, m.config); err != nil || r.Pollution != "polluted" || len(r.References) != 1 || calls != 1 {
		t.Fatalf("existing pool unnecessarily waited for newly added source: %+v %v", r, err)
	}
}

func TestReferenceReadFailureCannotClassifyOrTriggerCollection(t *testing.T) {
	st := testStore(t)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	m.probe = func(_ context.Context, _ string, _ model.Server, _ model.Domain, _ time.Duration) model.ProbeResult {
		t.Error("reference read error must not be treated as a cold empty pool")
		return model.ProbeResult{}
	}
	r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
	if err := m.classify(context.Background(), &r, m.config.Domains[0], m.config); err == nil || r.Pollution != "unknown" {
		t.Fatalf("read error allowed comparison: %+v %v", r, err)
	}
}

func TestRevokedExistingPoolCannotBeUsedFromLookupCache(t *testing.T) {
	st := testStore(t)
	source, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	calls := 0
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		calls++
		r := fakeResult(s, d)
		r.Records = []model.AnswerRecord{{Value: r.Answers[0], TTLSeconds: 60}}
		return r
	}
	if _, err := m.lookup(context.Background(), source, m.config.Domains[0], m.config, true); err != nil {
		t.Fatal(err)
	}
	source.Trusted = false
	if _, err := st.SaveServer(source); err != nil {
		t.Fatal(err)
	}
	r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"198.51.100.1"}}
	if err := m.classify(context.Background(), &r, m.config.Domains[0], m.config); err != nil || r.Pollution != "unknown" || len(r.References) != 0 || calls != 1 {
		t.Fatalf("revoked pool survived through cache: %+v %v calls=%d", r, err, calls)
	}
}

func TestColdStartWaitersUseNewScheduledReferenceBeforeSlowCollectorFinishes(t *testing.T) {
	st := testStore(t)
	slow, err := st.SaveServer(model.Server{Name: "slow", Address: "udp://192.0.2.1", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	fast, err := st.SaveServer(model.Server{Name: "fast", Address: "udp://192.0.2.2", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	domain := m.config.Domains[0]
	started, release, ownerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int32
	m.probe = func(ctx context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		calls.Add(1)
		r := fakeResult(s, d)
		r.Raw = "completed reference evidence"
		if s.ID == slow.ID {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
			r.Answers = []string{"192.0.2.10"}
		} else {
			r.Answers = []string{"192.0.2.20"}
		}
		return r
	}
	type verdict struct {
		result model.ProbeResult
		err    error
	}
	classify := func(ctx context.Context) verdict {
		r := model.ProbeResult{Type: "A", Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.20"}}
		err := m.classify(ctx, &r, domain, m.config)
		return verdict{r, err}
	}
	owner := make(chan verdict, 1)
	go func() { defer close(ownerDone); owner <- classify(context.Background()) }()
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); <-ownerDone })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("cold-start collector did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	const waiterCount = 4
	waiting := make(chan struct{}, waiterCount)
	finished := make(chan verdict, waiterCount)
	var wg sync.WaitGroup
	t.Cleanup(func() { cancel(); wg.Wait() })
	for i := 0; i < waiterCount; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); waiting <- struct{}{}; finished <- classify(ctx) }()
	}
	for i := 0; i < waiterCount; i++ {
		<-waiting
	}
	// With an empty pool, waiters must join the in-flight collection. Leave it
	// blocked while an ordinary scheduled probe supplies a different source.
	select {
	case v := <-finished:
		t.Fatalf("empty pool returned before reference: %+v", v)
	case <-time.After(20 * time.Millisecond):
	}
	m.runRound(context.Background(), fast, m.config, []model.Server{slow, fast}, 0)
	for i := 0; i < waiterCount; i++ {
		select {
		case v := <-finished:
			if v.err != nil || v.result.Pollution != "clean" || len(v.result.References) != 1 || v.result.References[0].ServerID != fast.ID {
				t.Fatalf("waiter missed newly available reference: %+v", v)
			}
		case <-time.After(time.Second):
			t.Fatal("waiter remained blocked by slow collector after scheduled source filled pool")
		}
	}
	select {
	case v := <-owner:
		t.Fatalf("slow collector unexpectedly finished: %+v", v)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case v := <-owner:
		if v.err != nil || v.result.Pollution != "clean" || len(v.result.References) != 2 {
			t.Fatalf("collector lost final union: %+v", v)
		}
	case <-time.After(time.Second):
		t.Fatal("collector did not finish after release")
	}
	<-ownerDone
	if calls.Load() != 2 {
		t.Fatal("waiters issued redundant probes", calls.Load())
	}
	raw, err := st.Results(slow.ID, 10, 0)
	if err != nil || len(raw) != 1 || raw[0].Raw != "completed reference evidence" {
		t.Fatal("slow collector evidence lost", raw, err)
	}
	m.mu.Lock()
	collecting := m.referenceCollections[domain.Name+"\x00"+domain.Type].done != nil
	m.mu.Unlock()
	if collecting {
		t.Fatal("finished collector left shared state open")
	}
}
