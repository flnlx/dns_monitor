package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestRatingConfigPersistenceAndLegacyClients(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	cfg, err := s.Store.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RatingWindowMinutes != 60 || cfg.RatingMinSamples != 3 || cfg.RatingMinCoverageMinutes != 5 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	cfg.RatingWindowMinutes = 90
	cfg.RatingMinSamples = 5
	for _, coverage := range []int{30, 0} {
		cfg.RatingMinCoverageMinutes = coverage
		body, _ := json.Marshal(cfg)
		response := call(h, "PUT", "/api/config", string(body), token)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		var legacy map[string]any
		if err := json.Unmarshal(body, &legacy); err != nil {
			t.Fatal(err)
		}
		delete(legacy, "rating_window_minutes")
		delete(legacy, "rating_min_samples")
		delete(legacy, "rating_min_coverage_minutes")
		legacy["interval_seconds"] = 120
		body, _ = json.Marshal(legacy)
		response = call(h, "PUT", "/api/config", string(body), token)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		saved, err := s.Store.GetConfig()
		if err != nil || saved.RatingWindowMinutes != 90 || saved.RatingMinSamples != 5 || saved.RatingMinCoverageMinutes != coverage || saved.IntervalSeconds != 120 {
			t.Fatalf("legacy update lost new settings: %+v, %v", saved, err)
		}
	}
}

func TestRatingConfigRejectsInvalidUpdates(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	initial := model.DefaultConfig()
	encoded, _ := json.Marshal(initial)
	for _, item := range []struct {
		name, field string
		value       any
	}{
		{"zero window", "rating_window_minutes", 0},
		{"window too short", "rating_window_minutes", 4},
		{"window too long", "rating_window_minutes", 1441},
		{"zero samples", "rating_min_samples", 0},
		{"too many samples", "rating_min_samples", 101},
		{"negative coverage", "rating_min_coverage_minutes", -1},
		{"coverage beyond window", "rating_min_coverage_minutes", 61},
		{"coverage too long", "rating_min_coverage_minutes", 1441},
		{"fractional window", "rating_window_minutes", 60.5},
		{"fractional samples", "rating_min_samples", 3.5},
		{"fractional coverage", "rating_min_coverage_minutes", 0.5},
		{"string value", "rating_min_samples", "3"},
	} {
		t.Run(item.name, func(t *testing.T) {
			var body map[string]any
			if err := json.Unmarshal(encoded, &body); err != nil {
				t.Fatal(err)
			}
			body[item.field] = item.value
			request, _ := json.Marshal(body)
			response := call(h, "PUT", "/api/config", string(request), token)
			if response.Code != 400 {
				t.Fatal("invalid configuration accepted", response.Code, response.Body.String())
			}
			saved, err := s.Store.GetConfig()
			if err != nil || saved.RatingWindowMinutes != initial.RatingWindowMinutes || saved.RatingMinSamples != initial.RatingMinSamples || saved.RatingMinCoverageMinutes != initial.RatingMinCoverageMinutes {
				t.Fatalf("invalid request modified configuration: %+v, %v", saved, err)
			}
		})
	}
}

func TestHistoryAndSummaryExposeCurrentEvaluation(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	node, err := s.Store.SaveServer(model.Server{Name: "recent healthy", Address: "udp://192.0.2.1", Protocol: "UDP", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	for i, age := range []time.Duration{2 * time.Hour, 10 * time.Minute, 5 * time.Minute, time.Minute} {
		at := now - age.Milliseconds()
		pollution := "matched"
		if i == 0 {
			pollution = "polluted"
		}
		probe := model.ProbeResult{Timestamp: at, Domain: "www.youtube.com", Type: "A", Received: true, Success: true, LatencyMS: 10, Pollution: pollution, PolicyVersion: 2}
		if err := s.Store.SaveRound(model.Round{ServerID: node.ID, StartedAt: at - 10, FinishedAt: at, NextDue: at + (5 * time.Minute).Milliseconds(), Results: []model.ProbeResult{probe}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, span := range []string{"24h", "7d", "30d"} {
		response := call(h, "GET", fmt.Sprintf("/api/servers/%d/history?range=%s", node.ID, span), "", token)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		var result struct {
			StatusHistory []model.StatusBucket    `json:"status_history"`
			Points        []model.HistoryPoint    `json:"points"`
			Metrics       model.Metrics           `json:"metrics"`
			Current       model.CurrentEvaluation `json:"current"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.StatusHistory) != map[string]int{"24h": 48, "7d": 56, "30d": 60}[span] {
			t.Fatal("missing status history", span, len(result.StatusHistory))
		}
		var snapshots int64
		for _, point := range result.StatusHistory {
			snapshots += point.Snapshots
		}
		if snapshots != 4 {
			t.Fatal("wrong snapshot count", snapshots)
		}
		if len(result.Points) == 0 || result.Metrics.Grade != "F" || result.Metrics.Pollution != "polluted" {
			t.Fatalf("history contract lost historical metrics: %+v", result)
		}
		if result.Current.Grade != "A" || result.Current.Pollution != "matched" || result.Current.Samples != 3 || result.Current.WindowMinutes != 60 || result.Current.MinSamples != 3 || result.Current.MinCoverageMinutes != 5 || result.Current.QualityAt == 0 {
			t.Fatalf("current evaluation mixed with historical range %s: %+v", span, result.Current)
		}
	}
	response := call(h, "GET", "/api/state?range=30d", "", token)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var state struct {
		Servers []model.ServerSummary `json:"servers"`
		Version string                `json:"version"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Servers) != 1 || len(state.Servers[0].StatusHistory) != 60 {
		t.Fatal("state missing compact status history")
	}
	if state.Version != "1.4.0" || len(state.Servers) != 1 || state.Servers[0].Metrics.Grade != "F" || state.Servers[0].Current.Grade != "A" {
		t.Fatalf("state contract lost current or historical evaluation: %+v", state)
	}
}
