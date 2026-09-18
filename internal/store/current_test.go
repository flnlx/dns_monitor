package store

import (
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

const currentMinute = int64(time.Minute / time.Millisecond)

func currentAt(t *testing.T, s *Store, id, now int64) model.CurrentEvaluation {
	t.Helper()
	value, err := s.CurrentEvaluation(id, now)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func setCurrentThresholds(t *testing.T, s *Store, window, samples, coverage int) {
	t.Helper()
	config, err := s.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	config.RatingWindowMinutes = window
	config.RatingMinSamples = samples
	config.RatingMinCoverageMinutes = coverage
	if err = s.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentUnknownReplacesOldAWithoutChangingHistory(t *testing.T) {
	s, server := testStore(t)
	at := time.Now().Add(-time.Hour).UnixMilli()
	mustSave(t, s, round(server.ID, at, 31*currentMinute, true, true, 10, 10, "matched"))
	for i := 0; i < 17; i++ {
		mustSave(t, s, round(server.ID, at+int64(31+i)*currentMinute, currentMinute, true, true, 1, 10, "unknown"))
	}
	now := at + 48*currentMinute
	values, err := s.Summary(at, now)
	if err != nil {
		t.Fatal(err)
	}
	current := values[0].Current
	if values[0].Metrics.Grade != "A" || values[0].Metrics.Pollution != "matched" {
		t.Fatalf("historical aggregate changed: %+v", values[0].Metrics)
	}
	if current.Grade != "pending" || current.Pollution != "unknown" || current.PendingReason != "quality_unknown" || current.Samples != 27 {
		t.Fatalf("17 unknown results were masked by old A: %+v", current)
	}
	if current.Score != 100 || current.SuccessRate != 100 || current.Availability != 100 || current.QualityAt != now-currentMinute {
		t.Fatalf("unknown quality lost performance evidence: %+v", current)
	}
	mustSave(t, s, round(server.ID, now, currentMinute, true, true, 1, 10, "matched"))
	current = currentAt(t, s, server.ID, now)
	if current.Grade != "A" || current.PendingReason != "" || current.Pollution != "matched" || current.QualityAt != now {
		t.Fatalf("latest recovery did not take effect immediately: %+v", current)
	}
}

func TestCurrentRecoversFromOldEFAndExcludesAuxiliary(t *testing.T) {
	for _, status := range []string{"polluted", "suspicious"} {
		t.Run(status, func(t *testing.T) {
			s, server := testStore(t)
			at := time.Now().Add(-20 * time.Minute).UnixMilli()
			mustSave(t, s, round(server.ID, at, 10*currentMinute, true, true, 1, 10, status))
			first := currentAt(t, s, server.ID, at)
			if first.Grade != map[string]string{"polluted": "F", "suspicious": "E"}[status] || first.PendingReason != "" {
				t.Fatalf("current conviction waited for warm-up: %+v", first)
			}
			mustSave(t, s, round(server.ID, at+10*currentMinute, 5*currentMinute, true, true, 2, 10, "matched"))
			aux := round(server.ID, at+11*currentMinute, currentMinute, false, false, 50, 0, "polluted")
			aux.Auxiliary = true
			mustSave(t, s, aux)
			current := currentAt(t, s, server.ID, at+12*currentMinute)
			if current.Grade != "A" || current.Pollution != "matched" || current.Samples != 3 || current.QualityAt != at+10*currentMinute || current.CoveredMinutes != 12 {
				t.Fatalf("old or auxiliary conviction masked recovery: %+v", current)
			}
			values, err := s.Summary(at, at+12*currentMinute)
			if err != nil {
				t.Fatal(err)
			}
			if values[0].Metrics.Grade != first.Grade {
				t.Fatalf("historical conviction was erased: %+v", values[0].Metrics)
			}
		})
	}
}

func TestCurrentLatestRoundQualitySeverityAndNonA(t *testing.T) {
	cases := []struct {
		name                          string
		results                       []model.ProbeResult
		overrideType, overrideVerdict string
		quality, grade                string
	}{
		{"mixed_unknown", []model.ProbeResult{{Type: "A", Pollution: "matched"}, {Type: "A", Pollution: "unknown"}}, "", "", "unknown", "pending"},
		{"suspicious_beats_unknown", []model.ProbeResult{{Type: "A", Pollution: "suspicious"}, {Type: "A", Pollution: "unknown"}}, "", "", "suspicious", "E"},
		{"polluted_beats_suspicious", []model.ProbeResult{{Type: "A", Pollution: "polluted"}, {Type: "A", Pollution: "suspicious"}}, "", "", "polluted", "F"},
		{"clean_beats_matched", []model.ProbeResult{{Type: "A", Pollution: "clean"}, {Type: "A", Pollution: "matched"}}, "", "", "clean", "A"},
		{"non_a_unknown_ignored", []model.ProbeResult{{Type: "A", Pollution: "matched"}, {Type: "TXT", Pollution: "unknown"}}, "", "", "matched", "A"},
		{"only_non_a", []model.ProbeResult{{Type: "TXT", Pollution: "unknown"}}, "", "", "unknown", "pending"},
		{"non_a_manual_polluted", []model.ProbeResult{{Type: "A", Pollution: "matched"}, {Type: "TXT", Pollution: "unknown"}}, "TXT", "polluted", "polluted", "F"},
		{"non_a_manual_clean", []model.ProbeResult{{Type: "TXT", Pollution: "unknown"}}, "TXT", "clean", "clean", "A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, server := testStore(t)
			setCurrentThresholds(t, s, 60, 1, 0)
			at := time.Now().UnixMilli()
			r := round(server.ID, at, currentMinute, true, true, 0, 10, "matched")
			for _, item := range tc.results {
				item.Domain, item.Received, item.Success, item.LatencyMS, item.PolicyVersion = "example.com", true, true, 10, 2
				r.Results = append(r.Results, item)
			}
			mustSave(t, s, r)
			if tc.overrideType != "" {
				if err := s.SaveOverride(model.Override{ServerID: server.ID, Domain: "example.com", Type: tc.overrideType, Verdict: tc.overrideVerdict}); err != nil {
					t.Fatal(err)
				}
			}
			current := currentAt(t, s, server.ID, at)
			if current.Pollution != tc.quality || current.Grade != tc.grade {
				t.Fatalf("want %s/%s, got %+v", tc.quality, tc.grade, current)
			}
			if current.Availability != 100 || current.CoveredMinutes != 0 {
				t.Fatalf("zero elapsed coverage incorrectly penalized first sample: %+v", current)
			}
		})
	}
}

func TestCurrentWindowBoundariesBackoffAndLatency(t *testing.T) {
	s, server := testStore(t)
	setCurrentThresholds(t, s, 10, 2, 5)
	now := time.Now().UnixMilli()
	mustSave(t, s, round(server.ID, now-15*currentMinute, 20*currentMinute, false, false, 50, 0, "polluted"))
	mustSave(t, s, round(server.ID, now-5*currentMinute, 10*currentMinute, true, true, 1, 10.1, "matched"))
	mustSave(t, s, round(server.ID, now, currentMinute, true, true, 1, 20.1, "matched"))
	mustSave(t, s, round(server.ID, now+currentMinute, currentMinute, false, false, 100, 0, "polluted"))
	current := currentAt(t, s, server.ID, now)
	if current.Samples != 2 || current.Pollution != "matched" || current.QualityAt != now || current.Grade != "C" {
		t.Fatalf("past/future records contaminated current window: %+v", current)
	}
	closeFloat(t, 50, current.Availability)
	closeFloat(t, 100, current.SuccessRate)
	closeFloat(t, 100, current.Coverage)
	closeFloat(t, 10, current.CoveredMinutes)
	closeFloat(t, 15.1, current.AverageMS)
	closeFloat(t, 21, current.P95MS)
}

func TestCurrentExpiryAndTrustedExemption(t *testing.T) {
	s, server := testStore(t)
	setCurrentThresholds(t, s, 5, 1, 0)
	at := time.Now().Add(-10 * time.Minute).UnixMilli()
	mustSave(t, s, round(server.ID, at, 20*currentMinute, true, true, 1, 10, "polluted"))
	if got := currentAt(t, s, server.ID, at+5*currentMinute); got.Grade != "F" || got.Samples != 1 {
		t.Fatalf("exact lower boundary excluded: %+v", got)
	}
	got := currentAt(t, s, server.ID, at+5*currentMinute+1)
	if got.Grade != "pending" || got.PendingReason != "no_samples" || got.QualityAt != 0 || got.Pollution != "unknown" || got.Samples != 0 {
		t.Fatalf("expired evidence retained a conviction: %+v", got)
	}
	server.Trusted = true
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	got = currentAt(t, s, server.ID, at)
	if got.Grade != "A" || got.Pollution != "matched" || got.Score != 100 {
		t.Fatalf("trust exemption not applied before scoring: %+v", got)
	}
	got = currentAt(t, s, server.ID, at+5*currentMinute+1)
	if got.Grade != "pending" || got.PendingReason != "no_samples" || got.Pollution != "matched" {
		t.Fatalf("trust inflated expired evidence: %+v", got)
	}
	mustSave(t, s, round(server.ID, at+6*currentMinute, currentMinute, false, false, 1, 0, "unknown"))
	got = currentAt(t, s, server.ID, at+6*currentMinute)
	if got.Grade == "A" || got.Grade == "pending" || got.Grade == "E" || got.Grade == "F" || got.Pollution != "matched" {
		t.Fatalf("trust bypassed bad performance: %+v", got)
	}
	mustSave(t, s, round(server.ID, at+12*currentMinute, currentMinute, true, true, 1, 10, "unknown"))
	got = currentAt(t, s, server.ID, at+12*currentMinute)
	if got.Grade != "A" || got.Pollution != "matched" {
		t.Fatalf("trusted recovery did not age out failed metrics: %+v", got)
	}
}

func TestCurrentThresholdsAndMutationsInvalidateSummaryCache(t *testing.T) {
	s, server := testStore(t)
	at := time.Now().Add(-20 * time.Minute).UnixMilli()
	mustSave(t, s, round(server.ID, at, 20*currentMinute, true, true, 2, 10, "matched"))
	now := at + 5*currentMinute
	get := func() model.CurrentEvaluation {
		t.Helper()
		values, err := s.Summary(at-currentMinute, now)
		if err != nil {
			t.Fatal(err)
		}
		return values[0].Current
	}
	if got := get(); got.Grade != "pending" || got.PendingReason != "insufficient_samples" || got.MinSamples != 3 || got.MinCoverageMinutes != 5 || got.WindowMinutes != 60 {
		t.Fatalf("default thresholds not applied: %+v", got)
	}
	setCurrentThresholds(t, s, 60, 2, 10)
	if got := get(); got.Grade != "pending" || got.PendingReason != "insufficient_coverage" {
		t.Fatalf("coverage config did not invalidate cached grade: %+v", got)
	}
	setCurrentThresholds(t, s, 60, 2, 5)
	if got := get(); got.Grade != "A" || got.PendingReason != "" {
		t.Fatalf("new threshold did not take immediate effect: %+v", got)
	}
	if err := s.SaveOverride(model.Override{ServerID: server.ID, Domain: "example.com", Type: "A", Verdict: "polluted"}); err != nil {
		t.Fatal(err)
	}
	if got := get(); got.Grade != "F" {
		t.Fatalf("override left stale current: %+v", got)
	}
	server.Trusted = true
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	if got := get(); got.Grade != "A" || got.Pollution != "matched" {
		t.Fatalf("trust left stale current: %+v", got)
	}
	server.Trusted = false
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveOverride(model.Override{ServerID: server.ID, Domain: "example.com", Type: "A", Verdict: "auto"}); err != nil {
		t.Fatal(err)
	}
	if got := get(); got.Grade != "A" {
		t.Fatalf("manual reset left stale current: %+v", got)
	}
}

func TestCurrentUnavailableOverridesTrustAndRecovers(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		for _, received := range []bool{false, true} {
			s, server := testStore(t)
			server.Trusted = trusted
			if _, err := s.SaveServer(server); err != nil {
				t.Fatal(err)
			}
			at := time.Now().Add(-time.Hour).UnixMilli()
			// A first failure must not wait for sample or coverage thresholds.
			mustSave(t, s, round(server.ID, at, currentMinute, received, false, 1, 0, "unknown"))
			if got := currentAt(t, s, server.ID, at); got.Grade != "unavailable" || got.Score != 0 || got.PendingReason != "" {
				t.Fatalf("first failure trusted=%v received=%v: %+v", trusted, received, got)
			}
			// Historical successes must not hide a new failed round.
			mustSave(t, s, round(server.ID, at+currentMinute, 30*currentMinute, true, true, 100, 10, "matched"))
			mustSave(t, s, round(server.ID, at+31*currentMinute, currentMinute, received, false, 1, 0, "unknown"))
			summary, err := s.Summary(at, at+32*currentMinute)
			if err != nil {
				t.Fatal(err)
			}
			if got := summary[0].Current; got.Grade != "unavailable" || got.Score != 0 || got.SuccessRate < 95 {
				t.Fatalf("old successes masked current failure: %+v", got)
			}
			// Auxiliary success cannot clear the formal failure.
			aux := round(server.ID, at+32*currentMinute, currentMinute, true, true, 1, 10, "matched")
			aux.Auxiliary = true
			mustSave(t, s, aux)
			if got := currentAt(t, s, server.ID, at+32*currentMinute); got.Grade != "unavailable" {
				t.Fatalf("auxiliary cleared outage: %+v", got)
			}
			// Partial success is degraded service, not complete unavailability.
			recovery := round(server.ID, at+33*currentMinute, currentMinute, true, true, 2, 10, "matched")
			recovery.Results[1].Success = false
			mustSave(t, s, recovery)
			if got := currentAt(t, s, server.ID, at+34*currentMinute); got.Grade != "A" {
				t.Fatalf("recovery failed: %+v", got)
			}
			if got := currentAt(t, s, server.ID, at+94*currentMinute); got.Grade != "pending" || got.PendingReason != "no_samples" {
				t.Fatalf("stale status: %+v", got)
			}
		}
	}
}
