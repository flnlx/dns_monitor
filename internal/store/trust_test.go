package store

import (
	"errors"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestTrustedExemptionRestoresQualityAndPreservesHistoricalEvidence(t *testing.T) {
	s, server := testStore(t)
	at := time.Now().Add(-time.Hour).UnixMilli()
	mustSave(t, s, round(server.ID, at, 3600000, true, true, 10, 10, "polluted"))
	if err := s.SaveOverride(model.Override{ServerID: server.ID, Domain: "example.com", Type: "A", Verdict: "polluted"}); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := s.db.QueryRow("SELECT raw_json FROM results LIMIT 1").Scan(&original); err != nil {
		t.Fatal(err)
	}
	check := func(trusted bool, grade string) {
		t.Helper()
		summary, err := s.Summary(at, at+3600000)
		if err != nil {
			t.Fatal(err)
		}
		if summary[0].Metrics.Grade != grade {
			t.Fatalf("summary wanted %s: %+v", grade, summary[0].Metrics)
		}
		points, metrics, err := s.History(server.ID, at, at+3600000, 3600000)
		if err != nil {
			t.Fatal(err)
		}
		if metrics.Grade != grade || points[0].Grade != grade {
			t.Fatalf("history exemption mismatch: %+v %+v", metrics, points)
		}
		if trusted && (metrics.Pollution != "matched" || metrics.Score != 100 || summary[0].Metrics.Pollution != "matched") {
			t.Fatalf("trusted exemption did not precede scoring: %+v", metrics)
		}
		results, err := s.Results(server.ID, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		if results[0].Trusted != trusted || results[0].Pollution != "polluted" || results[0].Override != "polluted" {
			t.Fatalf("result lost raw evidence or current trust: %+v", results[0])
		}
		if err = s.WalkResults(server.ID, at, func(r model.ProbeResult) error {
			if r.Trusted != trusted {
				t.Fatalf("export used stale trust: %+v", r)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	check(false, "F")
	server.Trusted = true
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	check(true, "A")
	var after string
	if err := s.db.QueryRow("SELECT raw_json FROM results LIMIT 1").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != original {
		t.Fatal("trust toggle rewrote historical evidence")
	}
	server.Trusted = false
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	check(false, "F")
}

func TestTrustedOverrideGuardIsTransactionalAndAutoStillWorks(t *testing.T) {
	s, server := testStore(t)
	verdict := model.Override{ServerID: server.ID, Domain: "example.com", Type: "A", Verdict: "polluted"}
	if err := s.SaveOverride(verdict); err != nil {
		t.Fatal(err)
	}
	server.Trusted = true
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveOverride(verdict); !errors.Is(err, ErrTrustedOverride) {
		t.Fatalf("want trusted override rejection, got %v", err)
	}
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM override_audit").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("rejected override left audit mutation: %d", n)
	}
	verdict.Verdict = "auto"
	if err := s.SaveOverride(verdict); err != nil {
		t.Fatal(err)
	}
	verdict.Verdict = "clean"
	if err := s.SaveOverride(verdict); err != nil {
		t.Fatal(err)
	}
}

func TestTrustDoesNotPromotePoorQualityOrInsufficientData(t *testing.T) {
	s, server := testStore(t)
	server.Trusted = true
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Hour).UnixMilli()
	before, err := s.Summary(at, at+3600000)
	if err != nil {
		t.Fatal(err)
	}
	if before[0].Metrics.Grade != "pending" {
		t.Fatal("trust assigned a quality grade without samples")
	}
	mustSave(t, s, round(server.ID, at, 3600000, false, false, 10, 0, "polluted"))
	after, err := s.Summary(at, at+3600000)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Metrics.Grade != "D" || after[0].Metrics.Pollution != "matched" || after[0].Metrics.Score != 0 {
		t.Fatalf("trust inflated quality: %+v", after[0].Metrics)
	}
}
