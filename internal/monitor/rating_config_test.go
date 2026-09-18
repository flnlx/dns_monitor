package monitor

import (
	"testing"

	"dnsmonitor/internal/model"
)

func TestRatingConfigBoundaries(t *testing.T) {
	for _, item := range []struct {
		name                      string
		window, samples, coverage int
		valid                     bool
	}{
		{"defaults", 60, 3, 5, true},
		{"minimums", 5, 1, 0, true},
		{"maximums", 1440, 100, 1440, true},
		{"coverage equals window", 5, 3, 5, true},
		{"window zero", 0, 3, 0, false},
		{"window below minimum", 4, 3, 0, false},
		{"window above maximum", 1441, 3, 5, false},
		{"samples zero", 60, 0, 5, false},
		{"samples negative", 60, -1, 5, false},
		{"samples above maximum", 60, 101, 5, false},
		{"coverage negative", 60, 3, -1, false},
		{"coverage above maximum", 1440, 3, 1441, false},
		{"coverage exceeds window", 5, 3, 6, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			cfg := model.DefaultConfig()
			cfg.RatingWindowMinutes = item.window
			cfg.RatingMinSamples = item.samples
			cfg.RatingMinCoverageMinutes = item.coverage
			err := ValidateConfig(&cfg)
			if (err == nil) != item.valid {
				t.Fatalf("valid=%v, error=%v", item.valid, err)
			}
		})
	}
}
