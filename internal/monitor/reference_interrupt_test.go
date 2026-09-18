package monitor

import (
	"context"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestInterruptedColdStartPreservesEvidenceAndResumesUsingAvailablePool(t *testing.T) {
	for _, interruption := range []string{"pause", "stop"} {
		t.Run(interruption, func(t *testing.T) {
			st := testStore(t)
			servers := make([]model.Server, 3)
			for i := range servers {
				server, err := st.SaveServer(model.Server{Name: "fixture", Address: "udp://192.0.2." + string(rune('1'+i)), Enabled: true, Trusted: i > 0})
				if err != nil {
					t.Fatal(err)
				}
				servers[i] = server
			}
			cfg := model.DefaultConfig()
			cfg.ReferenceTTLSeconds = 0
			m := New(st, "")
			m.config = cfg
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			interrupt := true
			secondCalls := 0
			m.probe = func(_ context.Context, _ string, server model.Server, domain model.Domain, _ time.Duration) model.ProbeResult {
				result := fakeResult(server, domain)
				switch server.ID {
				case servers[0].ID:
					result.Answers = []string{"192.0.2.10", "192.0.2.20"}
				case servers[1].ID:
					result.Answers = []string{"192.0.2.10"}
					if interrupt {
						if interruption == "stop" {
							cancel()
						} else {
							m.mu.Lock()
							m.config.Concurrency = 0
							m.signalLocked()
							m.mu.Unlock()
						}
					}
				case servers[2].ID:
					secondCalls++
					result.Answers = []string{"192.0.2.20"}
				}
				return result
			}
			m.runRound(ctx, servers[0], cfg, servers, 0)
			results, err := st.Results(servers[0].ID, 10, 0)
			if err != nil || len(results) != 1 {
				t.Fatalf("completed target evidence not retained: %+v %v", results, err)
			}
			result := results[0]
			if result.Pollution != "unknown" || !result.Success || len(result.References) != 1 || secondCalls != 0 {
				t.Fatalf("interrupted union must remain unknown with evidence intact: %+v secondCalls=%d", result, secondCalls)
			}
			refs, err := st.Results(servers[1].ID, 10, 0)
			if err != nil || len(refs) != 1 {
				t.Fatalf("completed reference evidence not retained: %+v %v", refs, err)
			}

			// Resume uses the completed observation immediately without collecting another source.
			interrupt = false
			m.config = cfg
			m.runRound(context.Background(), servers[0], cfg, servers, 0)
			results, err = st.Results(servers[0].ID, 10, 0)
			if err != nil || len(results) != 2 || results[0].Pollution != "suspicious" || len(results[0].References) != 1 || secondCalls != 0 {
				t.Fatalf("resume did not use the available pool immediately: %+v %v", results, err)
			}
		})
	}
}
