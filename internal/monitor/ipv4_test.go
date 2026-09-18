package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestIPv4SourceAndProxyIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	local, err := ipv4Source(ctx, "https://127.0.0.1:443/dns-query")
	if err != nil || local != "127.0.0.1" {
		t.Fatalf("IPv4 route source: %s %v", local, err)
	}
	env := probeEnvironment([]string{"PATH=a", "DOGGO_IPV6=true", "https_proxy=http://proxy", "ALL_PROXY=x", "SystemRoot=C:\\Windows"})
	if len(env) != 2 || env[0] != "PATH=a" || !strings.HasPrefix(env[1], "SystemRoot=") {
		t.Fatal("inherited probe settings", env)
	}
	for _, address := range []string{"h3://dns.example.com/dns-query", "quic://dns.example.com"} {
		args := doggoArgs(model.Server{Address: address}, model.Domain{Name: "example.com", Type: "A"}, time.Second)
		if !strings.Contains(strings.Join(args, " "), "--source=0.0.0.0") {
			t.Fatal("UDP encrypted transport not bound to IPv4", args)
		}
	}
}

func TestStampCommandTranslationAndUnsupportedConstraints(t *testing.T) {
	for _, v := range []struct{ stamp, want string }{
		{stampFixture(0, []byte("192.0.2.1:5353")), "udp://192.0.2.1:5353"},
		{stampFixture(3, nil, nil, []byte("dns.example.com:8853")), "tls://dns.example.com:8853"},
		{stampFixture(4, nil, nil, []byte("dns.example.com")), "quic://dns.example.com"},
	} {
		if got := commandAddress(v.stamp); got != v.want {
			t.Errorf("got %s, want %s", got, v.want)
		}
	}
	for _, v := range []string{
		stampFixture(2, []byte("192.0.2.1"), nil, []byte("dns.example.com"), []byte("/dns-query")),
		stampFixture(2, nil, make([]byte, 32), []byte("dns.example.com"), []byte("/dns-query")),
		stampFixture(3, nil, nil, []byte("dns.example.com"), []byte("192.0.2.1")),
	} {
		if _, _, err := ValidateAddress(v); err == nil {
			t.Fatal("silently accepted unsupported stamp security constraint", v)
		}
	}
}
