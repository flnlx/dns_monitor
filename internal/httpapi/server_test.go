package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnsmonitor/internal/model"
	"dnsmonitor/internal/monitor"
	"dnsmonitor/internal/store"
)

func testAPI(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	st, e := store.Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	mon := monitor.New(st, "missing-doggo.exe")
	s, e := New(st, mon, dir, "dns-monitor.exe", "doggo.exe", "0.0.0.0:8080")
	if e != nil {
		t.Fatal(e)
	}
	key, e := os.ReadFile(filepath.Join(dir, "access-key.txt"))
	if e != nil {
		t.Fatal(e)
	}
	return s, s.Handler(), strings.TrimSpace(string(key))
}
func loginToken(t *testing.T, h http.Handler, key string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/session", nil)
	req.SetBasicAuth("admin", key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	var result map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	return result["token"]
}
func call(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-DNSMonitor-Token", token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthenticationAndSameOrigin(t *testing.T) {
	_, h, key := testAPI(t)
	if got := call(h, "GET", "/api/state", "", ""); got.Code != 401 {
		t.Fatal(got.Code)
	}
	if got := call(h, "GET", "/api/health", "", ""); got.Code != 200 {
		t.Fatal(got.Code)
	}
	if got := call(h, "GET", "/api/session", "", ""); got.Code != 401 {
		t.Fatal(got.Code)
	}
	token := loginToken(t, h, key)
	if token == "" {
		t.Fatal("empty token")
	}
	got := call(h, "GET", "/api/state", "", token)
	if got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	req := httptest.NewRequest("PUT", "/api/config", strings.NewReader("{}"))
	req.Header.Set("X-DNSMonitor-Token", token)
	req.Header.Set("Origin", "http://evil.invalid")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal("cross-origin accepted", rec.Code)
	}
	if call(h, "DELETE", "/api/session", "", token).Code != 200 {
		t.Fatal("logout failed")
	}
	if call(h, "GET", "/api/state", "", token).Code != 401 {
		t.Fatal("logged-out token still usable")
	}
}

func TestServerAndConfigWorkflow(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	created := call(h, "POST", "/api/servers", `{"name":"Trusted DNS","provider":"Example","address":"1.1.1.1","trusted":true,"enabled":true}`, token)
	if created.Code != 200 {
		t.Fatal(created.Code, created.Body.String())
	}
	var server model.Server
	if e := json.Unmarshal(created.Body.Bytes(), &server); e != nil || server.ID == 0 || !server.Trusted {
		t.Fatal(server, e)
	}
	bad := call(h, "POST", "/api/servers", `{"name":"IPv6","address":"[2606:4700:4700::1111]"}`, token)
	if bad.Code != 400 {
		t.Fatal("IPv6 accepted", bad.Body.String())
	}
	bad = call(h, "POST", "/api/servers", `{"name":"X","address":"1.1.1.1","unexpected":true}`, token)
	if bad.Code != 400 {
		t.Fatal("unknown fields accepted")
	}
	cfg := model.DefaultConfig()
	cfg.Concurrency = 0
	cfg.MaxBackoffHours = 24
	cfg.Listen = "127.0.0.1:9090"
	b, _ := json.Marshal(cfg)
	saved := call(h, "PUT", "/api/config", string(b), token)
	if saved.Code != 200 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	var result struct {
		Restart bool `json:"restart_required"`
	}
	_ = json.Unmarshal(saved.Body.Bytes(), &result)
	if !result.Restart {
		t.Fatal("listen change must require restart")
	}
	stored, e := s.Store.GetConfig()
	if e != nil || stored.Concurrency != 0 {
		t.Fatal(stored, e)
	}
	cfg.Concurrency = 51
	b, _ = json.Marshal(cfg)
	if call(h, "PUT", "/api/config", string(b), token).Code != 400 {
		t.Fatal("concurrency 51 accepted")
	}
	cfg.Concurrency = 2
	cfg.MaxBackoffHours = 25
	b, _ = json.Marshal(cfg)
	if call(h, "PUT", "/api/config", string(b), token).Code != 400 {
		t.Fatal("backoff 25 accepted")
	}
}

func TestCSVFormulaProtection(t *testing.T) {
	for _, v := range []string{"=SUM(A1)", "+cmd", "-cmd", "@cmd", "\tcmd", "\rcmd"} {
		if !strings.HasPrefix(csvSafe(v), "'") {
			t.Errorf("unsafe %q", v)
		}
	}
	if csvSafe("example.com") != "example.com" {
		t.Fatal("changed normal field")
	}
}
