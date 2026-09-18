package monitor

import (
	"encoding/base64"
	"testing"
)

func stampFixture(proto byte, fields ...[]byte) string {
	b := make([]byte, 9)
	b[0] = proto
	for _, field := range fields {
		b = append(b, byte(len(field)))
		b = append(b, field...)
	}
	return "sdns://" + base64.RawURLEncoding.EncodeToString(b)
}

func TestStampProtocolLayouts(t *testing.T) {
	for _, test := range []struct{ stamp, protocol string }{
		{stampFixture(1, []byte("192.0.2.1:443"), make([]byte, 32), []byte("2.dnscrypt-cert.example.com")), "DNSCrypt"},
		{stampFixture(2, nil, nil, []byte("dns.example.com:8443"), []byte("/dns-query")), "DoH"},
		{stampFixture(3, nil, nil, []byte("dns.example.com:8853")), "DoT"},
		{stampFixture(4, nil, nil, []byte("dns.example.com")), "DoQ"},
	} {
		a, p, err := ValidateAddress(test.stamp)
		if err != nil || p != test.protocol || a != test.stamp {
			t.Errorf("%s stamp rejected: %s %v", test.protocol, p, err)
		}
	}
	for _, stamp := range []string{
		stampFixture(1, []byte("192.0.2.1"), make([]byte, 31), []byte("provider")),
		stampFixture(2, nil, nil, []byte("[::1]:443"), []byte("/dns-query")),
		stampFixture(2, nil, []byte{1}, []byte("dns.example.com"), []byte("/dns-query")),
	} {
		if _, _, err := ValidateAddress(stamp); err == nil {
			t.Error("malformed or IPv6 stamp accepted", stamp)
		}
	}
}

func TestAddressKeySupportsEmptyStampHashContinuations(t *testing.T) {
	b := make([]byte, 9)
	b[0] = 2
	b = append(b, 0, 128, 0)
	host, path := []byte("DNS.EXAMPLE.COM:443"), []byte("/dns-query")
	b = append(b, byte(len(host)))
	b = append(b, host...)
	b = append(b, byte(len(path)))
	b = append(b, path...)
	stamp := "sdns://" + base64.RawURLEncoding.EncodeToString(b)
	got, err := AddressKey(stamp)
	if err != nil || got != "https://dns.example.com/dns-query" {
		t.Fatalf("got=%s err=%v", got, err)
	}
}
