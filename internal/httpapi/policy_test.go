package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestReferenceHistoryConfigCompatibility(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	cfg, err := s.Store.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReferenceHistoryHours != 1 || len(cfg.Domains) != 1 || cfg.Domains[0].Name != "www.youtube.com" {
		t.Fatalf("wrong defaults: %+v", cfg)
	}
	for _, hours := range []float64{2.5, 0, 720} {
		cfg.ReferenceHistoryHours = hours
		b, _ := json.Marshal(cfg)
		response := call(h, "PUT", "/api/config", string(b), token)
		if response.Code != 200 {
			t.Fatal(hours, response.Code, response.Body.String())
		}
		saved, _ := s.Store.GetConfig()
		if saved.ReferenceHistoryHours != hours {
			t.Fatal("history setting not persisted", saved)
		}
	}
	var legacy map[string]any
	b, _ := json.Marshal(cfg)
	_ = json.Unmarshal(b, &legacy)
	delete(legacy, "reference_history_hours")
	b, _ = json.Marshal(legacy)
	if response := call(h, "PUT", "/api/config", string(b), token); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	saved, _ := s.Store.GetConfig()
	if saved.ReferenceHistoryHours != 720 {
		t.Fatal("legacy client cleared new setting", saved)
	}
	for _, hours := range []float64{-0.01, 720.01} {
		cfg.ReferenceHistoryHours = hours
		b, _ := json.Marshal(cfg)
		if response := call(h, "PUT", "/api/config", string(b), token); response.Code != 400 {
			t.Fatal("invalid history accepted", hours, response.Code)
		}
	}
	saved, _ = s.Store.GetConfig()
	if saved.ReferenceHistoryHours != 720 {
		t.Fatal("invalid update wrote configuration")
	}
}

func TestPolicyV2ResultsAndRawCSV(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	now := time.Now().UnixMilli()
	inputs := []struct {
		status    string
		policy    int
		effective string
	}{{"matched", 2, "matched"}, {"clean", 2, "clean"}, {"suspicious", 2, "suspicious"}, {"polluted", 2, "polluted"}, {"polluted", 0, "unknown"}}
	for i, item := range inputs {
		node, err := s.Store.SaveServer(model.Server{Name: item.status, Address: "udp://192.0.2.1", Protocol: "UDP", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		at := now - int64(i+1)*100
		record := model.AnswerRecord{Value: "192.0.2.10", TTLSeconds: 30, ObservedAt: at, ExpiresAt: at + 30000}
		result := model.ProbeResult{Timestamp: at, Domain: "www.youtube.com", Type: "A", Received: true, Success: true, LatencyMS: 10, Rcode: "NOERROR", Answers: []string{record.Value}, Records: []model.AnswerRecord{record}, Pollution: item.status, PolicyVersion: item.policy}
		if err := s.Store.SaveRound(model.Round{ServerID: node.ID, StartedAt: at - 1, FinishedAt: at + 1, NextDue: at + 300000, Results: []model.ProbeResult{result}}); err != nil {
			t.Fatal(err)
		}
		records, err := s.Store.Results(node.ID, 10, 0)
		if err != nil || len(records) != 1 {
			t.Fatal(records, err)
		}
		if records[0].Pollution != item.status || records[0].EffectivePollution != item.effective || len(records[0].Records) != 1 {
			t.Fatalf("lost detected/effective/TTL evidence: %+v", records[0])
		}
	}
	response := call(h, "GET", "/api/export?range=24h", "", token)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(response.Body.String(), "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 6 {
		t.Fatal(rows, err)
	}
	cols := map[string]int{}
	for i, name := range rows[0] {
		cols[name] = i
	}
	for _, column := range []string{"records_json", "policy_version", "detected_pollution", "effective_pollution"} {
		if _, ok := cols[column]; !ok {
			t.Fatal("missing column", column)
		}
	}
	for i, row := range rows[1:] {
		expected := inputs[i]
		if row[cols["detected_pollution"]] != expected.status || row[cols["effective_pollution"]] != expected.effective {
			t.Fatal("wrong CSV effective policy", row)
		}
		var records []model.AnswerRecord
		if err := json.Unmarshal([]byte(row[cols["records_json"]]), &records); err != nil || len(records) != 1 || records[0].TTLSeconds != 30 {
			t.Fatal("missing TTL evidence", records, err)
		}
	}
}
