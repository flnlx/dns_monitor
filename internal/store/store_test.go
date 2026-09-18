package store

import (
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func testStore(t *testing.T) (*Store, model.Server) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	server, err := s.SaveServer(model.Server{Name: "test", Address: "1.1.1.1", Protocol: "UDP", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return s, server
}
func round(server, at, duration int64, received, success bool, count int, latency float64, pollution string) model.Round {
	r := model.Round{ServerID: server, StartedAt: at - 10, FinishedAt: at, NextDue: at + duration}
	for i := 0; i < count; i++ {
		r.Results = append(r.Results, model.ProbeResult{Timestamp: at, Domain: "example.com", Type: "A", Received: received, Success: success, LatencyMS: latency, Pollution: pollution, Answers: []string{"93.184.216.34"}})
	}
	return r
}
func mustSave(t *testing.T, s *Store, r model.Round) {
	t.Helper()
	if err := s.SaveRound(r); err != nil {
		t.Fatal(err)
	}
}
func closeFloat(t *testing.T, want, got float64) {
	t.Helper()
	if math.Abs(want-got) > 0.00001 {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestConfigurationAndServerPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portable", "data.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "0.0.0.0:8080" || c.Concurrency != 2 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	c.Concurrency = 0
	c.MaxBackoffHours = 24
	c.Domains = []model.Domain{{Name: "example.org", Type: "A"}}
	if err = s.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	v, err := s.SaveServer(model.Server{Name: "original", Address: "9.9.9.9", Protocol: "UDP", Trusted: true, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	originalCreated := v.CreatedAt
	v.Name = "renamed"
	v.CreatedAt = 1
	v, err = s.SaveServer(v)
	if err != nil {
		t.Fatal(err)
	}
	if v.CreatedAt != originalCreated {
		t.Fatal("server update replaced creation timestamp")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c, err = s.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Concurrency != 0 || c.MaxBackoffHours != 24 || c.Domains[0].Name != "example.org" {
		t.Fatalf("config did not survive reopen: %+v", c)
	}
	got, err := s.GetServer(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "renamed" || !got.Trusted {
		t.Fatalf("server did not survive reopen: %+v", got)
	}
}

func TestBackoffWeightsAvailabilityButNotQuerySuccess(t *testing.T) {
	s, v := testStore(t)
	start := time.Now().Add(-2 * time.Hour).UnixMilli()
	minute := int64(time.Minute / time.Millisecond)
	mustSave(t, s, round(v.ID, start, 5*minute, true, true, 5, 10.2, "clean"))
	mustSave(t, s, round(v.ID, start+5*minute, 60*minute, false, false, 5, 0, "unknown"))
	mustSave(t, s, round(v.ID, start+65*minute, 5*minute, true, true, 5, 20.2, "clean"))
	values, err := s.Summary(start, start+70*minute)
	if err != nil {
		t.Fatal(err)
	}
	m := values[0].Metrics
	closeFloat(t, 100*10.0/70, m.Availability)
	closeFloat(t, 100*10.0/15, m.SuccessRate)
	closeFloat(t, 100, m.Coverage)
	closeFloat(t, 15.2, m.AverageMS)
	closeFloat(t, 21, m.P95MS) // Histogram rounds up by less than 1 ms.
	if m.Grade != "D" || m.Samples != 15 {
		t.Fatalf("unexpected metrics: %+v", m)
	}
	points, hm, err := s.History(v.ID, start, start+70*minute, 5*minute)
	if err != nil {
		t.Fatal(err)
	}
	closeFloat(t, m.Availability, hm.Availability)
	closeFloat(t, m.SuccessRate, hm.SuccessRate)
	closeFloat(t, m.P95MS, hm.P95MS)
	if len(points) != 14 {
		t.Fatalf("got %d buckets", len(points))
	}
	if points[2].Samples != 0 || points[2].Availability != 0 || points[2].Coverage != 100 {
		t.Fatalf("failed backoff was not carried through chart: %+v", points[2])
	}
}

func TestUnknownGapsAndCrossingInterval(t *testing.T) {
	s, v := testStore(t)
	start := time.Now().Add(-2 * time.Hour).UnixMilli()
	minute := int64(time.Minute / time.Millisecond)
	mustSave(t, s, round(v.ID, start, 60*minute, false, false, 1, 0, "unknown"))
	mustSave(t, s, round(v.ID, start+90*minute, 5*minute, true, true, 1, 8, "clean"))
	values, err := s.Summary(start+30*minute, start+120*minute)
	if err != nil {
		t.Fatal(err)
	}
	m := values[0].Metrics
	closeFloat(t, 100*5.0/35, m.Availability)
	closeFloat(t, 100*35.0/90, m.Coverage)
	closeFloat(t, 100, m.SuccessRate)
	if m.Samples != 1 || m.Grade != "pending" {
		t.Fatalf("expected insufficient samples: %+v", m)
	}
}

func TestOverrideRetainsEvidenceAndUpdatesHistory(t *testing.T) {
	s, v := testStore(t)
	at := time.Now().Add(-time.Hour).UnixMilli()
	duration := int64(time.Hour / time.Millisecond)
	mustSave(t, s, round(v.ID, at, duration, true, true, 10, 5, "polluted"))
	check := func(grade, detected, override string) {
		t.Helper()
		summary, err := s.Summary(at, at+duration)
		if err != nil {
			t.Fatal(err)
		}
		if summary[0].Metrics.Grade != grade {
			t.Fatalf("grade want %s got %+v", grade, summary[0].Metrics)
		}
		results, err := s.Results(v.ID, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		if results[0].Pollution != detected || results[0].Override != override {
			t.Fatalf("evidence or override altered: %+v", results[0])
		}
	}
	check("F", "polluted", "")
	if err := s.SaveOverride(model.Override{ServerID: v.ID, Domain: "Example.COM.", Type: "a", Verdict: "clean", Note: "reviewed CDN answer"}); err != nil {
		t.Fatal(err)
	}
	check("A", "polluted", "clean")
	if err := s.SaveOverride(model.Override{ServerID: v.ID, Domain: "example.com", Type: "A", Verdict: "auto"}); err != nil {
		t.Fatal(err)
	}
	check("F", "polluted", "")
	if err := s.SaveOverride(model.Override{ServerID: v.ID, Domain: "example.com", Type: "A", Verdict: "polluted"}); err != nil {
		t.Fatal(err)
	}
	mustSave(t, s, round(v.ID, at+duration, 1000, true, true, 1, 4, "clean"))
	results, err := s.Results(v.ID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Pollution != "clean" || results[0].Override != "polluted" {
		t.Fatalf("override not applied to future sample: %+v", results[0])
	}
	var events int
	if err = s.db.QueryRow("SELECT COUNT(*) FROM override_audit").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 3 {
		t.Fatalf("want 3 audit events got %d", events)
	}
}

func TestCleanupRemovesExpiredRawAndEmbeddedEvidence(t *testing.T) {
	s, v := testStore(t)
	cutoff := time.Now().Add(-model.Retention).UnixMilli()
	old := round(v.ID, cutoff-1000, 500, true, true, 1, 5, "clean")
	mustSave(t, s, old)
	current := round(v.ID, cutoff+1000, 500, true, true, 1, 6, "polluted")
	current.Results[0].References = []model.Reference{{ServerID: 99, Timestamp: cutoff - 1, Raw: "expired"}, {ServerID: 99, Timestamp: cutoff + 1, Raw: "retained"}}
	mustSave(t, s, current)
	if err := s.SaveOverride(model.Override{ServerID: v.ID, Domain: "example.com", Type: "A", Verdict: "clean"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE override_audit SET timestamp=?", cutoff-1); err != nil {
		t.Fatal(err)
	}
	if err := s.Cleanup(cutoff); err != nil {
		t.Fatal(err)
	}
	results, err := s.Results(v.ID, 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(results[0].References) != 1 || results[0].References[0].Raw != "retained" {
		t.Fatalf("expired evidence survived: %+v", results)
	}
	if results[0].Override != "clean" {
		t.Fatal("cleanup erased active manual configuration")
	}
	for table, want := range map[string]int{"rounds": 1, "results": 1, "latency_buckets": 1, "override_audit": 0} {
		var got int
		if err = s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count want %d got %d", table, want, got)
		}
	}
}

func TestAuxiliaryEvidenceDoesNotDistortTimeline(t *testing.T) {
	s, v := testStore(t)
	at := time.Now().Add(-time.Hour).UnixMilli()
	hour := int64(time.Hour / time.Millisecond)
	mustSave(t, s, round(v.ID, at, hour, false, false, 1, 0, "unknown"))
	ref := round(v.ID, at+hour/2, hour, true, true, 20, 1, "polluted")
	ref.Auxiliary = true
	mustSave(t, s, ref)
	values, err := s.Summary(at, at+hour)
	if err != nil {
		t.Fatal(err)
	}
	m := values[0].Metrics
	if m.Samples != 1 || m.Availability != 0 || m.Pollution == "polluted" || values[0].LastProbe != at || values[0].Failures != 1 {
		t.Fatalf("auxiliary polluted scheduled metrics: %+v", values)
	}
	results, err := s.Results(v.ID, 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 21 {
		t.Fatalf("auxiliary evidence was lost: %d", len(results))
	}
}

func TestEarlyAndOutOfOrderRoundsDoNotDoubleCountCoverage(t *testing.T) {
	s, v := testStore(t)
	at := time.Now().Add(-time.Hour).UnixMilli()
	minute := int64(time.Minute / time.Millisecond)
	mustSave(t, s, round(v.ID, at, 60*minute, false, false, 1, 0, "unknown"))
	mustSave(t, s, round(v.ID, at+30*minute, 30*minute, true, true, 1, 10, "clean"))
	mustSave(t, s, round(v.ID, at+15*minute, 45*minute, true, true, 1, 20, "clean"))
	values, err := s.Summary(at, at+60*minute)
	if err != nil {
		t.Fatal(err)
	}
	closeFloat(t, 75, values[0].Metrics.Availability)
	closeFloat(t, 100, values[0].Metrics.Coverage)
}

func TestDeleteCascadesAndHistoryIsBounded(t *testing.T) {
	s, v := testStore(t)
	at := time.Now().Add(-time.Hour).UnixMilli()
	mustSave(t, s, round(v.ID, at, 3600000, true, true, 1, 4, "clean"))
	if err := s.SaveOverride(model.Override{ServerID: v.ID, Domain: "example.com", Type: "A", Verdict: "polluted"}); err != nil {
		t.Fatal(err)
	}
	points, _, err := s.History(v.ID, at, at+int64(model.Retention/time.Millisecond), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) > 1000 {
		t.Fatalf("unbounded chart %d", len(points))
	}
	count := 0
	if err = s.WalkResults(v.ID, at, func(r model.ProbeResult) error { count++; return nil }); err != nil || count != 1 {
		t.Fatalf("export count=%d err=%v", count, err)
	}
	if err = s.DeleteServer(v.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"rounds", "results", "latency_buckets", "overrides", "override_audit"} {
		var n int
		if err = s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("orphan rows in %s", table)
		}
	}
	if _, err = s.GetServer(v.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted server returned %v", err)
	}
}
