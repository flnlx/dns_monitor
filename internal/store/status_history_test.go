package store

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func historyAt(t *testing.T, s *Store, id, since, now int64) []model.StatusBucket {
	t.Helper()
	points, err := s.StatusHistory(id, since, now, true)
	if err != nil {
		t.Fatal(err)
	}
	return points
}

func TestStatusHistoryPreservesActualGradesAndGaps(t *testing.T) {
	s, server := testStore(t)
	setCurrentThresholds(t, s, 60, 1, 0)
	since := time.Now().Add(-24 * time.Hour).UnixMilli()
	now := since + (24 * time.Hour).Milliseconds()
	at := since + currentMinute
	mustSave(t, s, round(server.ID, at, currentMinute, true, true, 1, 10, "matched"))
	mustSave(t, s, round(server.ID, at+currentMinute, currentMinute, true, true, 1, 10, "polluted"))
	mustSave(t, s, round(server.ID, at+2*currentMinute, currentMinute, false, false, 1, 0, "unknown"))
	mustSave(t, s, round(server.ID, at+3*currentMinute, currentMinute, true, true, 1, 10, "matched"))
	aux := round(server.ID, since+40*currentMinute, currentMinute, true, true, 1, 10, "polluted")
	aux.Auxiliary = true
	mustSave(t, s, aux)
	// Include the exact right endpoint, but exclude future records.
	mustSave(t, s, round(server.ID, now, currentMinute, true, true, 1, 10, "suspicious"))
	mustSave(t, s, round(server.ID, now+1, currentMinute, true, true, 1, 10, "polluted"))
	points := historyAt(t, s, server.ID, since, now)
	if len(points) != 48 || points[0].Pollution != "polluted" || points[0].Grade != "unavailable" || points[0].Snapshots != 4 {
		t.Fatalf("short failures hidden: %+v", points[0])
	}
	if points[0].GradeCounts["A"] != 1 || points[0].GradeCounts["F"] != 1 || points[0].GradeCounts["unavailable"] != 1 || points[0].Latest.Current.Grade == "unavailable" {
		t.Fatalf("lost individual snapshots or recovery: %+v", points[0])
	}
	if points[1].Snapshots != 0 || points[1].Grade != "no_data" || points[1].Pollution != "no_data" || points[1].Latest != nil {
		t.Fatalf("auxiliary probe filled a gap: %+v", points[1])
	}
	if points[47].Snapshots != 1 || points[47].Grade != "E" || points[47].End != now {
		t.Fatalf("endpoint mismatch: %+v", points[47])
	}
	// Human overrides, trust and rating changes affect current state only.
	if err := s.SaveOverride(model.Override{ServerID: server.ID, Domain: "example.com", Type: "A", Verdict: "clean"}); err != nil {
		t.Fatal(err)
	}
	server.Trusted = true
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	setCurrentThresholds(t, s, 5, 100, 5)
	after := historyAt(t, s, server.ID, since, now)
	if !reflect.DeepEqual(points, after) {
		t.Fatal("later configuration/trust/review rewrote history")
	}
	summary, err := s.Summary(since, now)
	if err != nil {
		t.Fatal(err)
	}
	if summary[0].StatusHistory[0].Grade != "unavailable" || summary[0].StatusHistory[0].Latest != nil {
		t.Fatal("summary must expose compact immutable history")
	}
	for _, test := range []struct{ days, count int }{{7, 56}, {30, 60}} {
		p := historyAt(t, s, server.ID, now-int64(test.days)*86400000, now)
		if len(p) != test.count {
			t.Fatal("wrong resolution", test.days, len(p))
		}
		var n int64
		for _, v := range p {
			n += v.Snapshots
		}
		if n != 5 {
			t.Fatal("range aggregation lost snapshots", n)
		}
	}
}

func TestStatusHistoryPersistenceRetentionAndUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	server, err := s.SaveServer(model.Server{Name: "history", Address: "1.1.1.1", Protocol: "UDP", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Hour).UnixMilli()
	// Simulate an existing database with probes but no snapshot table yet.
	mustSave(t, s, round(server.ID, at, currentMinute, true, true, 1, 10, "clean"))
	if _, err = s.db.Exec("DROP TABLE evaluation_snapshots"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range historyAt(t, s, server.ID, at-1, at+2*currentMinute) {
		if p.Snapshots != 0 {
			t.Fatal("upgrade fabricated past ratings")
		}
	}
	mustSave(t, s, round(server.ID, at+currentMinute, currentMinute, true, true, 1, 10, "polluted"))
	before := historyAt(t, s, server.ID, at-1, at+2*currentMinute)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, historyAt(t, s, server.ID, at-1, at+2*currentMinute)) {
		t.Fatal("restart changed snapshots")
	}
	if err = s.Cleanup(at + currentMinute + 1); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT COUNT(*) FROM evaluation_snapshots").Scan(&count); err != nil || count != 0 {
		t.Fatal("retention left snapshots", count, err)
	}
	mustSave(t, s, round(server.ID, at+2*currentMinute, currentMinute, true, true, 1, 10, "clean"))
	if err = s.DeleteServer(server.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT COUNT(*) FROM evaluation_snapshots").Scan(&count); err != nil || count != 0 {
		t.Fatal("server deletion left snapshots", count, err)
	}
}

func TestStatusSnapshotMatchesCurrentEvaluationAndRollsBackOnFailure(t *testing.T) {
	s, server := testStore(t)
	at := time.Now().UnixMilli()
	for i, quality := range []string{"matched", "suspicious", "polluted", "unknown"} {
		now := at + int64(i)*currentMinute
		mustSave(t, s, round(server.ID, now, currentMinute, true, true, 1, 10, quality))
		want := currentAt(t, s, server.ID, now)
		var raw string
		if err := s.db.QueryRow("SELECT evaluation FROM evaluation_snapshots ORDER BY timestamp DESC LIMIT 1").Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var got model.CurrentEvaluation
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("snapshot differs from current: %+v vs %+v", got, want)
		}
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_snapshot BEFORE INSERT ON evaluation_snapshots BEGIN SELECT RAISE(ABORT, 'test snapshot failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRound(round(server.ID, at+5*currentMinute, currentMinute, true, true, 1, 10, "clean")); err == nil {
		t.Fatal("snapshot write failure ignored")
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM rounds").Scan(&count); err != nil || count != 4 {
		t.Fatal("partial round committed", count, err)
	}
}

func TestStatusHistoryTrustedFailureAndPending(t *testing.T) {
	s, server := testStore(t)
	server.Trusted = true
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UnixMilli()
	mustSave(t, s, round(server.ID, at, currentMinute, true, true, 1, 10, "polluted"))
	mustSave(t, s, round(server.ID, at+currentMinute, currentMinute, false, false, 1, 0, "unknown"))
	var pending, unavailable bool
	for _, point := range historyAt(t, s, server.ID, at, at+2*currentMinute) {
		if point.Snapshots == 0 {
			continue
		}
		if point.Pollution != "clean" || !point.Latest.Trusted {
			t.Fatal("incorrect trusted quality", point)
		}
		pending = pending || point.Grade == "pending"
		unavailable = unavailable || point.Grade == "unavailable"
	}
	if !pending || !unavailable {
		t.Fatal("missing warmup or failure")
	}
}
