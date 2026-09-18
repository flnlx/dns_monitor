package store

import (
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestRetentionCutoffInsideRoundRebuildsStatistics(t *testing.T) {
	s, v := testStore(t)
	cutoff := time.Now().Add(-model.Retention).UnixMilli()
	r := round(v.ID, cutoff+1000, 3600000, true, true, 2, 12.5, "clean")
	r.Results[0].Timestamp = cutoff - 1
	r.Results[0].Pollution = "polluted"
	r.Results[0].LatencyMS = 999
	mustSave(t, s, r)
	if err := s.Cleanup(cutoff); err != nil {
		t.Fatal(err)
	}
	values, err := s.Summary(cutoff, cutoff+3601000)
	if err != nil {
		t.Fatal(err)
	}
	m := values[0].Metrics
	if m.Samples != 1 || m.Pollution != "clean" || m.AverageMS != 12.5 || m.P95MS != 13 {
		t.Fatalf("expired sample affected statistics: %+v", m)
	}
}

func TestResultCursorPreservesIdenticalTimestamps(t *testing.T) {
	s, v := testStore(t)
	at := time.Now().UnixMilli()
	mustSave(t, s, round(v.ID, at, 1000, true, true, 201, 10, "clean"))
	first, err := s.ResultsPage(v.ID, 200, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 200 {
		t.Fatalf("first page size %d", len(first))
	}
	last := first[len(first)-1]
	second, err := s.ResultsPage(v.ID, 200, last.Timestamp, last.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].ID >= last.ID {
		t.Fatalf("identical-timestamp cursor skipped or repeated results: %+v", second)
	}
}

func TestDeletedServerIDCannotBeReused(t *testing.T) {
	s, old := testStore(t)
	if err := s.DeleteServer(old.ID); err != nil {
		t.Fatal(err)
	}
	current, err := s.SaveServer(model.Server{Name: "new", Address: "8.8.8.8", Protocol: "UDP", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if current.ID <= old.ID {
		t.Fatal("new server reused an ID of an in-flight deleted server")
	}
	if err := s.SaveRound(round(old.ID, time.Now().UnixMilli(), 1000, true, true, 1, 10, "clean")); err == nil {
		t.Fatal("old in-flight result was accepted after server deletion")
	}
}
