package monitor

import (
	"context"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestRoundPersistsAutomaticConvictionAndReference(t *testing.T) {
	st := testStore(t)
	cfg := model.DefaultConfig()
	cfg.Domains = []model.Domain{{Name: "example.com", Type: "A"}}
	target, err := st.SaveServer(model.Server{Name: "target", Address: "udp://192.0.2.1", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.2", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		answer := "198.51.100.100"
		if s.ID == trusted.ID {
			answer = "192.0.2.200"
		}
		return model.ProbeResult{Timestamp: time.Now().UnixMilli(), ServerID: s.ID, Domain: d.Name, Type: d.Type, Received: true, Success: true, Rcode: "NOERROR", Answers: []string{answer}, Raw: "original evidence"}
	}
	completion := m.runRound(context.Background(), target, cfg, []model.Server{target, trusted}, 0)
	if completion.nextDue <= time.Now().UnixMilli() {
		t.Fatal("next deadline not scheduled")
	}
	results, err := st.Results(target.ID, 10, 0)
	if err != nil || len(results) != 1 {
		t.Fatal(results, err)
	}
	if results[0].Pollution != "polluted" || len(results[0].References) != 1 || results[0].References[0].ServerID != trusted.ID {
		t.Fatalf("missing automatic conviction evidence: %+v", results[0])
	}
	references, err := st.Results(trusted.ID, 10, 0)
	if err != nil || len(references) != 2 || references[0].Raw != "original evidence" {
		t.Fatalf("actual reference was lost: %+v %v", references, err)
	}
}

func TestSchedulerPausePersistsCompletedPartialRound(t *testing.T) {
	st := testStore(t)
	cfg := model.DefaultConfig()
	cfg.Concurrency = 1
	cfg.Domains = []model.Domain{{Name: "example.com", Type: "A"}, {Name: "example.net", Type: "A"}}
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	s, err := st.SaveServer(model.Server{Name: "target", Address: "udp://192.0.2.1", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	m.probe = func(ctx context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return model.ProbeResult{Timestamp: time.Now().UnixMilli(), ServerID: s.ID, Domain: d.Name, Type: d.Type, Received: true, Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.8"}}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() { m.Run(ctx); close(finished) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not start")
	}
	if m.Queue(s.ID) == nil {
		t.Fatal("duplicate active round accepted")
	}
	cfg.Concurrency = 0
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	m.Wake()
	deadline := time.Now().Add(2 * time.Second)
	for !m.Status().Paused && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !m.Status().Paused {
		t.Fatal("live pause ignored")
	}
	close(release)
	for {
		results, err := st.Results(s.ID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(results) > 0 {
			if len(results) != 1 || results[0].Domain != "example.com" {
				t.Fatalf("paused next domain was still queried: %+v", results)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("completed probe discarded by pause")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if m.Queue(s.ID) == nil {
		t.Fatal("paused manual query accepted")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not cancel")
	}
	if m.Status().Active != 0 {
		t.Fatal("active slots leaked")
	}
}
