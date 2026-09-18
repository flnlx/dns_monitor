package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func csvPreview(t *testing.T, h http.Handler, token, source string) importPreview {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"csv": source})
	res := call(h, "POST", "/api/servers/import/preview", string(body), token)
	if res.Code != 200 {
		t.Fatalf("preview %d: %s", res.Code, res.Body.String())
	}
	var value importPreview
	if e := json.Unmarshal(res.Body.Bytes(), &value); e != nil {
		t.Fatal(e)
	}
	return value
}

func TestServerCSVTemplateAndValidation(t *testing.T) {
	_, h, key := testAPI(t)
	token := loginToken(t, h, key)
	if call(h, "GET", "/api/servers/template", "", "").Code != 401 {
		t.Fatal("template must require authentication")
	}
	response := call(h, "GET", "/api/servers/template", "", token)
	if response.Code != 200 || !strings.HasPrefix(response.Body.String(), "\ufeffname,provider,address,enabled,trusted,notes") {
		t.Fatal(response.Body.String())
	}
	preview := csvPreview(t, h, token, response.Body.String())
	if preview.Errors != 0 || len(preview.Rows) != 2 || preview.Rows[0].Server.Trusted {
		t.Fatalf("invalid template %+v", preview)
	}
	invalid := csvPreview(t, h, token, "name,address,enabled\nIPv6,[::1],true\nInvalid,1.1.1.1,maybe\n")
	if invalid.Errors != 2 {
		t.Fatalf("invalid rows not flagged: %+v", invalid)
	}
	chinese := csvPreview(t, h, token, "名称,供应商,地址,启用,可信,备注\n测试,提供商,1.1.1.1,是,否,示例\n")
	if chinese.Errors != 0 || !chinese.Rows[0].Server.Enabled || chinese.Rows[0].Server.Trusted {
		t.Fatal(chinese)
	}
}

func TestServerCSVDecisionsAliasesAndHistoryPreserved(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	original, e := s.Store.SaveServer(model.Server{Name: "Original", Provider: "Old", Address: "udp://1.1.1.1:53", Protocol: "UDP", Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UnixMilli()
	e = s.Store.SaveRound(model.Round{ServerID: original.ID, StartedAt: now - 10, FinishedAt: now, NextDue: now + 300000, Results: []model.ProbeResult{{Timestamp: now, Domain: "youtube.com", Type: "A", Success: true, Received: true, LatencyMS: 10, Pollution: "clean"}}})
	if e != nil {
		t.Fatal(e)
	}
	source := "name,provider,address,enabled,trusted,notes\nUpdated,New,1.1.1.1,true,true,changed\nNew node,Vendor,9.9.9.9,true,false,new\nSkip alias,Other,udp://9.9.9.9:53,false,false,skip\n"
	preview := csvPreview(t, h, token, source)
	if preview.Errors != 0 || !preview.Rows[0].Duplicate || preview.Rows[0].Existing.ID != original.ID || !preview.Rows[2].Duplicate || preview.Rows[2].DuplicateOfRow != 2 {
		t.Fatalf("wrong duplicates: %+v", preview)
	}
	input := importRequest{CSV: source, Snapshot: preview.Snapshot}
	body, _ := json.Marshal(input)
	if got := call(h, "POST", "/api/servers/import", string(body), token); got.Code != 400 {
		t.Fatal("duplicate without choice accepted", got.Body.String())
	}
	input.Decisions = []importDecision{{Row: 1, Action: "overwrite"}, {Row: 3, Action: "skip"}}
	body, _ = json.Marshal(input)
	got := call(h, "POST", "/api/servers/import", string(body), token)
	if got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	var result model.ServerImportResult
	_ = json.Unmarshal(got.Body.Bytes(), &result)
	if result.Created != 1 || result.Updated != 1 || result.Skipped != 1 {
		t.Fatal(result)
	}
	after, e := s.Store.GetServer(original.ID)
	if e != nil || after.Name != "Updated" || !after.Trusted || after.CreatedAt != original.CreatedAt {
		t.Fatal(after, e)
	}
	records, e := s.Store.Results(original.ID, 100, 0)
	if e != nil || len(records) != 1 {
		t.Fatal("overwriting removed history", records, e)
	}
	if !records[0].Trusted {
		t.Fatal("current trusted role not present on historical result")
	}
	exported := call(h, "GET", "/api/servers/export", "", token)
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(exported.Body.String(), "\ufeff")))
	rows, e := reader.ReadAll()
	if e != nil || len(rows) != 3 || len(rows[0]) != 6 {
		t.Fatal(rows, e)
	}
	reimport := csvPreview(t, h, token, exported.Body.String())
	if reimport.Errors != 0 || !reimport.Rows[0].Duplicate || !reimport.Rows[1].Duplicate {
		t.Fatal(reimport)
	}
}

func TestServerCSVRejectsStalePreviewAndInvalidBatches(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	source := "name,address\nFirst,1.1.1.1\n"
	preview := csvPreview(t, h, token, source)
	_, e := s.Store.SaveServer(model.Server{Name: "Other change", Address: "udp://8.8.8.8", Protocol: "UDP"})
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(importRequest{CSV: source, Snapshot: preview.Snapshot})
	if res := call(h, "POST", "/api/servers/import", string(body), token); res.Code != 409 {
		t.Fatal(res.Code, res.Body.String())
	}
	servers, _ := s.Store.ListServers()
	if len(servers) != 1 {
		t.Fatal("stale batch had writes")
	}
	if _, err := prepareImport("name,address\n"+strings.Repeat("DNS,1.1.1.1\n", 1001), nil); err == nil {
		t.Fatal("unbounded row count accepted")
	}
	if _, err := prepareImport("name,address\n\xff,1.1.1.1", nil); err == nil {
		t.Fatal("invalid UTF8 accepted")
	}
	if _, err := prepareImport("name,address,unexpected\nDNS,1.1.1.1,true", nil); err == nil {
		t.Fatal("unknown columns accepted")
	}
}

func TestServerCSVSameFileOverwriteAndFormulaEscaping(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	source := "name,address,notes\nFirst,1.1.1.1,first\nLast,udp://1.1.1.1:53,\"comma, and a \"\"quote\"\"\"\n"
	preview := csvPreview(t, h, token, source)
	body, _ := json.Marshal(importRequest{CSV: source, Snapshot: preview.Snapshot, Decisions: []importDecision{{Row: 2, Action: "overwrite"}}})
	result := call(h, "POST", "/api/servers/import", string(body), token)
	if result.Code != 200 {
		t.Fatal(result.Code, result.Body.String())
	}
	servers, _ := s.Store.ListServers()
	if len(servers) != 1 || servers[0].Name != "Last" || servers[0].Notes != "comma, and a \"quote\"" {
		t.Fatal(servers)
	}
	servers[0].Name = "=danger"
	_, _ = s.Store.SaveServer(servers[0])
	out := call(h, "GET", "/api/servers/export", "", token)
	if !strings.Contains(out.Body.String(), "'=danger") {
		t.Fatal("spreadsheet formula not escaped")
	}
}

func TestTrustedOverrideRejectedAndYouTubeDefault(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	cfg, e := s.Store.GetConfig()
	if e != nil || len(cfg.Domains) != 1 || cfg.Domains[0].Name != "www.youtube.com" {
		t.Fatal(cfg, e)
	}
	server, e := s.Store.SaveServer(model.Server{Name: "Trusted", Address: "udp://1.1.1.1", Protocol: "UDP", Enabled: true, Trusted: true})
	if e != nil {
		t.Fatal(e)
	}
	body := fmt.Sprintf(`{"server_id":%d,"domain":"youtube.com","type":"A","verdict":"polluted"}`, server.ID)
	if res := call(h, "PUT", "/api/overrides", body, token); res.Code != 400 {
		t.Fatal("trusted could be convicted", res.Code, res.Body.String())
	}
}

func TestGlobalRefreshAndReadOnlyPolling(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	_, _ = s.Store.SaveServer(model.Server{Name: "Enabled", Address: "udp://1.1.1.1", Protocol: "UDP", Enabled: true})
	_, _ = s.Store.SaveServer(model.Server{Name: "Disabled", Address: "udp://8.8.8.8", Protocol: "UDP", Enabled: false})
	if s.Monitor.Status().Refresh != nil {
		t.Fatal("unexpected initial refresh")
	}
	if call(h, "GET", "/api/state", "", token).Code != 200 {
		t.Fatal("state failed")
	}
	if s.Monitor.Status().Refresh != nil {
		t.Fatal("reading state started probes")
	}
	res := call(h, "POST", "/api/probes", "", token)
	if res.Code != 202 {
		t.Fatal(res.Code, res.Body.String())
	}
	var batch model.RefreshStatus
	_ = json.Unmarshal(res.Body.Bytes(), &batch)
	if batch.Total != 1 || !batch.Pending {
		t.Fatal(batch)
	}
	cfg, _ := s.Store.GetConfig()
	cfg.Concurrency = 0
	_ = s.Store.SaveConfig(cfg)
	if call(h, "POST", "/api/probes", "", token).Code != 409 {
		t.Fatal("paused refresh accepted")
	}
}
