package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

// Opt-in reproducible storage measurement; excluded from routine correctness tests.
func TestSyntheticStoragePerformance(t *testing.T) {
	if os.Getenv("DNSMONITOR_STORAGE_BENCH") != "1" {
		t.Skip("set DNSMONITOR_STORAGE_BENCH=1 for synthetic storage measurement")
	}
	path := filepath.Join(t.TempDir(), "performance.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	servers := make([]model.Server, 100)
	for i := range servers {
		servers[i], err = s.SaveServer(model.Server{Name: "synthetic", Address: "1.1.1.1", Protocol: "UDP", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UnixMilli()
	since := now - int64(24*time.Hour/time.Millisecond)
	start := time.Now()
	for i, server := range servers {
		for j := 0; j < 288; j++ {
			at := since + int64(j)*300000
			r := round(server.ID, at, 300000, true, true, 2, float64(10+i+j%30), "clean")
			r.Results[1].Domain = "cloudflare.com"
			mustSave(t, s, r)
		}
	}
	seedDuration := time.Since(start)
	start = time.Now()
	values, err := s.Summary(since, now)
	cold := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 100 || values[0].Metrics.Samples != 576 {
		t.Fatalf("unexpected synthetic summary: %+v", values)
	}
	start = time.Now()
	_, err = s.Summary(since, now)
	warm := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	points, _, err := s.History(servers[0].ID, now-int64(model.Retention/time.Millisecond), now, 3600000)
	history := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) > 1000 {
		t.Fatal("unbounded history")
	}
	if _, err = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Synthetic dataset: 100 servers, 28800 rounds, 57600 raw queries; ingest=%s; cold 24h summary=%s; cached summary=%s; 30d chart with 24h data=%s (%d points); database=%d bytes", seedDuration, cold, warm, history, len(points), info.Size())
}
