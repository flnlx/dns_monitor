package monitor

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dnsmonitor/internal/model"
	"dnsmonitor/internal/store"
)

func TestValidateAddresses(t *testing.T) {
	for _, tt := range []struct{ input, protocol string }{
		{"1.1.1.1", "UDP"}, {"8.8.8.8:5353", "UDP"}, {"udp://9.9.9.9", "UDP"}, {"tcp://9.9.9.9:53", "TCP"}, {"tls://dns.google", "DoT"}, {"quic://dns.adguard-dns.com", "DoQ"}, {"https://dns.google/dns-query", "DoH"}, {"h3://dns.google/dns-query", "DoH3"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			_, p, e := ValidateAddress(tt.input)
			if e != nil || p != tt.protocol {
				t.Fatalf("%s, %v", p, e)
			}
		})
	}
	for _, bad := range []string{"", "::1", "[::1]:53", "udp://dns.google", "http://1.1.1.1/dns-query", "tls://[2001:db8::1]", "https://dns.google:65536/dns-query", "https://dns.google/dns-query#insecure", "[/example.com/]1.1.1.1", "1.1.1.1:0", "1.1.1.1:", "udp://1.1.1.1/", "https://a@dns.google/query", "--help", "1.1.1.1\n8.8.8.8"} {
		if _, _, e := ValidateAddress(bad); e == nil {
			t.Errorf("accepted invalid upstream %q", bad)
		}
	}
	stamp := append(make([]byte, 9), byte(len("1.1.1.1:53")))
	stamp = append(stamp, []byte("1.1.1.1:53")...)
	if _, p, e := ValidateAddress("sdns://" + base64.RawURLEncoding.EncodeToString(stamp)); e != nil || p != "UDP" {
		t.Fatalf("valid stamp rejected: %s %v", p, e)
	}
	stamp = append(make([]byte, 9), byte(len("[::1]:53")))
	stamp = append(stamp, []byte("[::1]:53")...)
	if _, _, e := ValidateAddress("sdns://" + base64.RawURLEncoding.EncodeToString(stamp)); e == nil {
		t.Fatal("IPv6 stamp accepted")
	}
}

func TestValidateConfigBoundaries(t *testing.T) {
	for _, c := range []int{0, 2, 50} {
		cfg := model.DefaultConfig()
		cfg.Concurrency = c
		if e := ValidateConfig(&cfg); e != nil {
			t.Fatal(e)
		}
	}
	for _, c := range []int{-1, 51} {
		cfg := model.DefaultConfig()
		cfg.Concurrency = c
		if ValidateConfig(&cfg) == nil {
			t.Fatal("bad concurrency accepted")
		}
	}
	for _, h := range []float64{0, 1, 24} {
		cfg := model.DefaultConfig()
		cfg.MaxBackoffHours = h
		if e := ValidateConfig(&cfg); e != nil {
			t.Fatal(e)
		}
	}
	cfg := model.DefaultConfig()
	cfg.MaxBackoffHours = 24.01
	if ValidateConfig(&cfg) == nil {
		t.Fatal("bad backoff accepted")
	}
	cfg = model.DefaultConfig()
	cfg.Listen = "[::]:8080"
	if ValidateConfig(&cfg) == nil {
		t.Fatal("IPv6 listen accepted")
	}
	cfg = model.DefaultConfig()
	cfg.Domains = []model.Domain{{Name: "EXAMPLE.COM.", Type: "a"}, {Name: "example.com", Type: "A"}}
	if ValidateConfig(&cfg) == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestDecodeDoggo(t *testing.T) {
	positive := `{"responses":[{"answers":[{"name":"example.com.","type":"A","address":"192.0.2.2","rtt":"7ms","ttl":"50s","status":""},{"name":"example.com.","type":"A","address":"192.0.2.1","rtt":"7ms"},{"type":"CNAME","address":"alias.example.com."}],"questions":[{"name":"example.com.","type":"A"}]}]}`
	d := model.Domain{Name: "example.com", Type: "A"}
	r, e := decodeDoggo([]byte(positive), d)
	if e != nil || !r.Received || !r.Success || r.Rcode != "NOERROR" || r.LatencyMS != 7 || strings.Join(r.Answers, ",") != "192.0.2.1,192.0.2.2" {
		t.Fatalf("bad positive decode: %+v, %v", r, e)
	}
	negative := `{"responses":[{"answers":null,"authorities":[{"type":"SOA","status":"NXDOMAIN","rtt":"5ms"}],"questions":[{"name":"example.com.","type":"A"}]}]}`
	r, e = decodeDoggo([]byte(negative), d)
	if e != nil || !r.Success || r.Rcode != "NXDOMAIN" {
		t.Fatalf("bad NXDOMAIN: %+v %v", r, e)
	}
	empty := `{"responses":[{"answers":null,"questions":[{"name":"example.com.","type":"A"}]}]}`
	r, e = decodeDoggo([]byte(empty), d)
	if e != nil || !r.Received || r.Success || r.Rcode != "UNKNOWN" {
		t.Fatalf("unknown empty response trusted: %+v %v", r, e)
	}
	r, e = decodeDoggo([]byte(`{"errors":[{"error":"timeout"}],"error":"lookup timeout"}`), d)
	if e != nil || r.Received || r.Success || r.Error != "lookup timeout" {
		t.Fatalf("bad timeout decode: %+v %v", r, e)
	}
	if _, e := decodeDoggo([]byte(positive+"garbage"), d); e == nil {
		t.Fatal("extra stdout accepted")
	}
	if _, e := decodeDoggo([]byte(positive), model.Domain{Name: "other.com", Type: "A"}); e == nil {
		t.Fatal("mismatched response accepted")
	}
}

func TestComparisonRequiresUsableAgreement(t *testing.T) {
	fresh := func() model.ProbeResult {
		return model.ProbeResult{Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.1"}}
	}
	a := model.Reference{ServerID: 1, Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.1"}}
	b := model.Reference{ServerID: 2, Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.2"}}
	r := fresh()
	compare(&r, []model.Reference{a})
	if r.Pollution != "clean" {
		t.Fatal(r)
	}
	r = fresh()
	compare(&r, []model.Reference{b})
	if r.Pollution != "polluted" {
		t.Fatal(r)
	}
	r = fresh()
	compare(&r, []model.Reference{a, b})
	if r.Pollution == "polluted" || r.Pollution == "clean" {
		t.Fatal("disagreeing refs yielded certainty", r)
	}
	a.Success = false
	r = fresh()
	compare(&r, []model.Reference{a})
	if r.Pollution != "unknown" {
		t.Fatal("failed reference used", r)
	}
	r = fresh()
	r.Success = false
	compare(&r, []model.Reference{b})
	if r.Pollution != "unknown" {
		t.Fatal("failed target convicted", r)
	}
	r = fresh()
	compare(&r, []model.Reference{{Success: true, Rcode: "NXDOMAIN"}})
	if r.Pollution != "polluted" {
		t.Fatal("NXDOMAIN mismatch omitted", r)
	}
}

func TestBackoffAndRecovery(t *testing.T) {
	cfg := model.DefaultConfig()
	for i := 0; i < 200; i++ {
		if d := nextDelay(cfg, 20, 19); d < 54*time.Minute || d > time.Hour {
			t.Fatal("backoff outside ceiling", d)
		}
		if d := nextDelay(cfg, 0, 20); d < 27*time.Second || d > 30*time.Second {
			t.Fatal("no fast recovery", d)
		}
		cfg.MaxBackoffHours = 0
		if d := nextDelay(cfg, 20, 19); d < 270*time.Second || d > 300*time.Second {
			t.Fatal("0 did not disable backoff", d)
		}
		cfg.MaxBackoffHours = 1
	}
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, e := store.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestGlobalConcurrencyCacheAndPersistence(t *testing.T) {
	st := testStore(t)
	cfg := model.DefaultConfig()
	cfg.Concurrency = 2
	cfg.Domains = []model.Domain{{Name: "example.com", Type: "A"}}
	cfg.ReferenceTTLSeconds = 60
	if e := st.SaveConfig(cfg); e != nil {
		t.Fatal(e)
	}
	s, err := st.SaveServer(model.Server{Name: "trusted", Address: "udp://192.0.2.1", Protocol: "UDP", Enabled: true, Trusted: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, "")
	m.config = cfg
	var active, maxActive, calls atomic.Int32
	m.probe = func(ctx context.Context, _ string, s model.Server, d model.Domain, _ time.Duration) model.ProbeResult {
		n := active.Add(1)
		for {
			old := maxActive.Load()
			if n <= old || maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		calls.Add(1)
		defer active.Add(-1)
		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Millisecond):
		}
		return model.ProbeResult{Timestamp: time.Now().UnixMilli(), ServerID: s.ID, Domain: d.Name, Type: d.Type, Received: true, Success: true, Rcode: "NOERROR", Answers: []string{"192.0.2.8"}}
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := m.lookup(context.Background(), s, cfg.Domains[0], cfg, true); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 || maxActive.Load() != 1 {
		t.Fatalf("references not singleflight cached: calls=%d active=%d", calls.Load(), maxActive.Load())
	}
	results, e := st.Results(s.ID, 100, 0)
	if e != nil || len(results) != 1 {
		t.Fatalf("reference not retained: %d %v", len(results), e)
	}
	summary, e := st.Summary(time.Now().Add(-time.Hour).UnixMilli(), time.Now().UnixMilli())
	if e != nil {
		t.Fatal(e)
	}
	if len(summary) != 1 || summary[0].LastProbe != 0 {
		t.Fatalf("reference modified scheduled timeline: %+v", summary)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			other := s
			other.ID = id
			other.Trusted = false
			if _, e := m.lookup(context.Background(), other, cfg.Domains[0], cfg, false); e != nil {
				t.Error(e)
			}
		}(int64(100 + i))
	}
	wg.Wait()
	if maxActive.Load() > 2 || m.Status().Active != 0 {
		t.Fatal("global concurrency exceeded", maxActive.Load(), m.Status())
	}
}

func TestPauseAndCancellation(t *testing.T) {
	m := New(nil, "")
	m.config.Concurrency = 0
	if e := m.acquire(context.Background(), 1); e == nil {
		t.Fatal("pause accepted probe")
	}
	m.config.Concurrency = 1
	if e := m.acquire(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := m.acquire(ctx, 2); e != context.Canceled {
		t.Fatal("blocked acquisition not canceled", e)
	}
	m.release(1)
}

// Real DOGGO integration uses a local UDP resolver and no external network.
func TestDoggoLocalIntegration(t *testing.T) {
	path, err := filepath.Abs(filepath.Join("..", "..", "doggo", "doggo.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Skip("bundled doggo unavailable")
	}
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() {
		b := make([]byte, 2048)
		for {
			n, addr, e := conn.ReadFrom(b)
			if e != nil {
				return
			}
			if n < 12 {
				continue
			}
			response := append([]byte(nil), b[:n]...)
			response[2] = 0x81
			response[3] = 0x80
			response[6] = 0
			response[7] = 1
			response[8] = 0
			response[9] = 0
			response[10] = 0
			response[11] = 0
			response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, 9)
			conn.WriteTo(response, addr)
		}
	}()
	s := model.Server{ID: 1, Address: "udp://" + conn.LocalAddr().String()}
	r := runDoggo(context.Background(), path, s, model.Domain{Name: "example.com", Type: "A"}, time.Second)
	if !r.Success || !r.Received || len(r.Answers) != 1 || r.Answers[0] != "192.0.2.9" {
		t.Fatalf("real doggo failed: %+v", r)
	}
	if len(r.Raw) == 0 {
		t.Fatal("missing raw evidence")
	}
	t.Log(fmt.Sprintf("real DOGGO parsed local answer in %.2f ms", r.LatencyMS))
}
