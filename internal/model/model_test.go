package model

import (
	"encoding/json"
	"testing"
)

func TestLegacyConfigurationKeepsRatingDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(`{"interval_seconds":120,"domains":[{"name":"example.com","type":"A"}]}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.RatingWindowMinutes != 60 || cfg.RatingMinSamples != 3 || cfg.RatingMinCoverageMinutes != 5 {
		t.Fatalf("legacy configuration lost rating defaults: %+v", cfg)
	}
	if cfg.RatingWAvail != .35 || cfg.RatingWSuccess != .30 || cfg.RatingWLatency != .35 {
		t.Fatalf("legacy configuration lost rating weight defaults: %+v", cfg)
	}
	if err := json.Unmarshal([]byte(`{"rating_min_coverage_minutes":0}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.RatingMinCoverageMinutes != 0 {
		t.Fatal("explicit zero coverage replaced with default")
	}
}
