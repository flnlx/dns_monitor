package monitor

import (
	"context"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestSameEndpointDefaultPortAliases(t *testing.T) {
	for _, pair := range [][2]string{
		{"udp://1.1.1.1", "udp://1.1.1.1:53"},
		{"tcp://1.1.1.1", "tcp://1.1.1.1:53"},
		{"tls://dns.example.com", "tls://dns.example.com:853"},
		{"quic://dns.example.com", "quic://dns.example.com:853"},
		{"https://dns.example.com/dns-query", "https://dns.example.com:443/dns-query"},
		{"h3://dns.example.com/dns-query", "h3://dns.example.com:443/dns-query"},
	} {
		if !sameEndpoint(pair[0], pair[1]) || !sameEndpoint(pair[1], pair[0]) {
			t.Errorf("default-port aliases differ: %v", pair)
		}
	}
	for _, pair := range [][2]string{
		{"udp://1.1.1.1", "udp://1.1.1.1:5353"},
		{"udp://1.1.1.1", "tcp://1.1.1.1"},
		{"https://dns.example.com/dns-query", "https://dns.example.com/query"},
		{"https://dns.example.com/dns-query", "https://dns.example.com:8443/dns-query"},
	} {
		if sameEndpoint(pair[0], pair[1]) {
			t.Errorf("distinct endpoints conflated: %v", pair)
		}
	}
}

func TestExplicitTrustedAliasContributesToUnion(t *testing.T) {
	st := testStore(t)
	cfg := model.DefaultConfig()
	cfg.Domains = []model.Domain{{Name: "example.com", Type: "A"}}
	target, err := st.SaveServer(model.Server{Name: "target", Address: "udp://192.0.2.1", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := st.SaveServer(model.Server{Name: "same endpoint", Address: "udp://192.0.2.1:53", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	calls := 0
	m.probe = func(_ context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		calls++
		return model.ProbeResult{Timestamp: time.Now().UnixMilli(), ServerID: s.ID, Domain: d.Name, Type: d.Type, Received: true, Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.8"}}
	}
	m.runRound(context.Background(), target, cfg, []model.Server{target, alias}, 0)
	results, err := st.Results(target.ID, 10, 0)
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%v err=%v", results, err)
	}
	if calls != 2 || len(results[0].References) != 1 || results[0].Pollution != "clean" {
		t.Fatalf("explicitly trusted default-port alias was excluded from union: calls=%d result=%+v", calls, results[0])
	}
}
