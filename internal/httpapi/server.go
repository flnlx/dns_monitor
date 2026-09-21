package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dnsmonitor/internal/model"
	"dnsmonitor/internal/monitor"
	"dnsmonitor/internal/store"
	"dnsmonitor/internal/web"
	"dnsmonitor/internal/winservice"
)

const Version = "1.4.1"

type session struct{ expires time.Time }
type Server struct {
	Store                                            *store.Store
	Monitor                                          *monitor.Monitor
	DataDir, Exe, DoggoPath, InitialConfiguredListen string
	key                                              string
	mu                                               sync.Mutex
	sessions                                         map[string]session
}

func New(st *store.Store, mon *monitor.Monitor, dataDir, exe, doggoPath, listen string) (*Server, error) {
	keyPath := filepath.Join(dataDir, "access-key.txt")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		b := make([]byte, 24)
		if _, err = rand.Read(b); err != nil {
			return nil, err
		}
		key = []byte(hex.EncodeToString(b))
		err = os.WriteFile(keyPath, append(key, '\n'), 0600)
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(key))) < 16 {
		return nil, fmt.Errorf("access-key.txt 必须包含至少 16 字符的访问密钥")
	}
	return &Server{Store: st, Monitor: mon, DataDir: dataDir, Exe: exe, DoggoPath: doggoPath, InitialConfiguredListen: listen, key: strings.TrimSpace(string(key)), sessions: map[string]session{}}, nil
}

func jsonOut(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func problem(w http.ResponseWriter, status int, err any) {
	jsonOut(w, status, map[string]any{"error": fmt.Sprint(err)})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("请求应为一个 JSON 对象")
	}
	return nil
}
func idFrom(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("无效的服务器编号")
	}
	return id, nil
}
func period(r *http.Request) (since, now, step int64) {
	now = time.Now().UnixMilli()
	span := 24 * time.Hour
	step = int64((5 * time.Minute) / time.Millisecond)
	switch r.URL.Query().Get("range") {
	case "7d":
		span = 7 * 24 * time.Hour
		step = int64((30 * time.Minute) / time.Millisecond)
	case "30d":
		span = 30 * 24 * time.Hour
		step = int64(time.Hour / time.Millisecond)
	}
	return now - span.Milliseconds(), now, step
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]any{"ok": true, "version": Version})
	})
	mux.HandleFunc("GET /api/session", s.login)
	mux.HandleFunc("DELETE /api/session", s.logout)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("POST /api/servers", s.saveServer)
	mux.HandleFunc("POST /api/probes", s.probeAll)
	mux.HandleFunc("GET /api/servers/export", s.exportServers)
	mux.HandleFunc("GET /api/servers/template", s.serverTemplate)
	mux.HandleFunc("POST /api/servers/import/preview", s.previewServerImport)
	mux.HandleFunc("POST /api/servers/import", s.importServers)
	mux.HandleFunc("PUT /api/servers/{id}", s.saveServer)
	mux.HandleFunc("DELETE /api/servers/{id}", s.deleteServer)
	mux.HandleFunc("GET /api/servers/{id}/history", s.history)
	mux.HandleFunc("GET /api/servers/{id}/results", s.results)
	mux.HandleFunc("POST /api/servers/{id}/probe", s.probe)
	mux.HandleFunc("PUT /api/config", s.config)
	mux.HandleFunc("PUT /api/overrides", s.override)
	mux.HandleFunc("GET /api/export", s.export)
	mux.HandleFunc("GET /api/service", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, winservice.Inspect()) })
	mux.HandleFunc("POST /api/service/{action}", s.service)
	assets, _ := fs.Sub(web.FS, "assets")
	mux.Handle("GET /", http.FileServer(http.FS(assets)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
					problem(w, 403, "不接受跨站请求")
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				problem(w, 403, "不接受跨站请求")
				return
			}
			if r.URL.Path != "/api/health" && !(r.URL.Path == "/api/session" && r.Method == "GET") && !s.authorized(r.Header.Get("X-DNSMonitor-Token")) {
				problem(w, 401, "请使用 data/access-key.txt 中的访问密钥登录")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[token]
	if !ok || time.Now().After(v.expires) {
		delete(s.sessions, token)
		return false
	}
	return true
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	user, password, ok := r.BasicAuth()
	if !ok || user != "admin" || subtle.ConstantTimeCompare([]byte(password), []byte(s.key)) != 1 {
		problem(w, 401, "访问密钥不正确，请查看程序目录中的 data/access-key.txt")
		return
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		problem(w, 500, e)
		return
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	for k, v := range s.sessions {
		if time.Now().After(v.expires) {
			delete(s.sessions, k)
		}
	}
	if len(s.sessions) >= 64 {
		s.mu.Unlock()
		problem(w, 429, "会话数量已满，请先退出其他页面或重启程序")
		return
	}
	s.sessions[token] = session{expires: time.Now().Add(12 * time.Hour)}
	s.mu.Unlock()
	jsonOut(w, 200, map[string]string{"token": token})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	delete(s.sessions, r.Header.Get("X-DNSMonitor-Token"))
	s.mu.Unlock()
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	since, now, _ := period(r)
	items, e := s.Store.Summary(since, now)
	if e != nil {
		problem(w, 500, e)
		return
	}
	cfg, e := s.Store.GetConfig()
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, map[string]any{"servers": items, "config": cfg, "runtime": s.Monitor.Status(), "service": winservice.Inspect(), "version": Version, "now": now, "listen_restart_required": cfg.Listen != s.InitialConfiguredListen})
}
func (s *Server) saveServer(w http.ResponseWriter, r *http.Request) {
	var server model.Server
	if e := decode(w, r, &server); e != nil {
		problem(w, 400, e)
		return
	}
	server.ID = 0
	if r.Method == "PUT" {
		id, e := idFrom(r)
		if e != nil {
			problem(w, 400, e)
			return
		}
		if _, e = s.Store.GetServer(id); e != nil {
			problem(w, 404, "服务器不存在")
			return
		}
		server.ID = id
	}
	if e := normalizeServerInput(&server); e != nil {
		problem(w, 400, e)
		return
	}
	var e error
	server, e = s.Store.SaveServer(server)
	if e != nil {
		problem(w, 400, e)
		return
	}
	s.Monitor.Wake()
	jsonOut(w, 200, server)
}
func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request) {
	id, e := idFrom(r)
	if e != nil {
		problem(w, 400, e)
		return
	}
	if e = s.Store.DeleteServer(id); e != nil {
		problem(w, 400, e)
		return
	}
	s.Monitor.Wake()
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	id, e := idFrom(r)
	if e != nil {
		problem(w, 400, e)
		return
	}
	since, now, step := period(r)
	points, metrics, e := s.Store.History(id, since, now, step)
	if e != nil {
		problem(w, 500, e)
		return
	}
	current, e := s.Store.CurrentEvaluation(id, now)
	if e != nil {
		problem(w, 500, e)
		return
	}
	statusHistory, e := s.Store.StatusHistory(id, since, now, true)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, map[string]any{"points": points, "metrics": metrics, "current": current, "status_history": statusHistory})
}
func (s *Server) results(w http.ResponseWriter, r *http.Request) {
	id, e := idFrom(r)
	if e != nil {
		problem(w, 400, e)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	beforeID, _ := strconv.ParseInt(r.URL.Query().Get("before_id"), 10, 64)
	records, e := s.Store.ResultsPage(id, limit, before, beforeID)
	if e != nil {
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, map[string]any{"results": records})
}
func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	id, e := idFrom(r)
	if e != nil {
		problem(w, 400, e)
		return
	}
	if e = s.Monitor.Queue(id); e != nil {
		problem(w, 409, e)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	prior, err := s.Store.GetConfig()
	if err != nil {
		problem(w, 500, err)
		return
	}
	// Older clients omit newer settings; preserve their saved values. Decode
	// explicit zero values normally so zero coverage remains a supported choice.
	cfg := model.Config{
		ReferenceHistoryHours:    prior.ReferenceHistoryHours,
		RatingWindowMinutes:      prior.RatingWindowMinutes,
		RatingMinSamples:         prior.RatingMinSamples,
		RatingMinCoverageMinutes: prior.RatingMinCoverageMinutes,
	}
	if e := decode(w, r, &cfg); e != nil {
		problem(w, 400, e)
		return
	}
	if e := monitor.ValidateConfig(&cfg); e != nil {
		problem(w, 400, e)
		return
	}
	if e := s.Store.SaveConfig(cfg); e != nil {
		problem(w, 500, e)
		return
	}
	s.Monitor.Wake()
	jsonOut(w, 200, map[string]any{"config": cfg, "restart_required": cfg.Listen != s.InitialConfiguredListen})
}
func (s *Server) override(w http.ResponseWriter, r *http.Request) {
	var v model.Override
	if e := decode(w, r, &v); e != nil {
		problem(w, 400, e)
		return
	}
	if v.Verdict != "auto" && v.Verdict != "clean" && v.Verdict != "polluted" {
		problem(w, 400, "无效判定")
		return
	}
	cfg := model.DefaultConfig()
	cfg.Domains = []model.Domain{{Name: v.Domain, Type: v.Type}}
	if e := monitor.ValidateConfig(&cfg); e != nil {
		problem(w, 400, e)
		return
	}
	v.Domain = cfg.Domains[0].Name
	v.Type = cfg.Domains[0].Type
	if len(v.Note) > 2000 {
		problem(w, 400, "说明过长")
		return
	}
	if _, e := s.Store.GetServer(v.ServerID); e != nil {
		problem(w, 404, "服务器不存在")
		return
	}
	v.UpdatedAt = time.Now().UnixMilli()
	if e := s.Store.SaveOverride(v); e != nil {
		if errors.Is(e, store.ErrTrustedOverride) {
			problem(w, 400, e)
			return
		}
		problem(w, 500, e)
		return
	}
	jsonOut(w, 200, v)
}
func (s *Server) service(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	switch action {
	case "install", "uninstall", "start", "stop", "restart":
	default:
		problem(w, 400, "未知操作")
		return
	}
	if e := winservice.Request(action, s.Exe, s.DataDir, s.DoggoPath); e != nil {
		log.Printf("网页请求服务操作失败: action=%s error=%v", action, e)
		problem(w, 400, e)
		return
	}
	log.Printf("网页请求服务操作: %s", action)
	jsonOut(w, 202, map[string]any{"ok": true, "message": "已提交服务操作；如有 Windows 管理员授权提示请确认。操作结果见 logs/service-action.log，随后刷新状态。"})
}
func csvSafe(v string) string {
	if strings.HasPrefix(v, "=") || strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") || strings.HasPrefix(v, "@") || strings.HasPrefix(v, "\t") || strings.HasPrefix(v, "\r") {
		return "'" + v
	}
	return v
}
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("server_id"), 10, 64)
	since, _, _ := period(r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=dns-monitor-results.csv")
	_, _ = io.WriteString(w, "\xef\xbb\xbf")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"timestamp", "server_id", "domain", "type", "received", "success", "latency_ms", "rcode", "answers", "detected_pollution", "override", "reason", "error", "references_json", "raw", "trusted", "effective_pollution", "records_json", "policy_version", "compared_at"})
	exportErr := s.Store.WalkResults(id, since, func(p model.ProbeResult) error {
		select {
		case <-r.Context().Done():
			return r.Context().Err()
		default:
		}
		refs, _ := json.Marshal(p.References)
		records, _ := json.Marshal(p.Records)
		effective := p.EffectivePollution
		if effective == "" {
			effective = p.Pollution
		}
		comparedAt := ""
		if p.ComparedAt > 0 {
			comparedAt = time.UnixMilli(p.ComparedAt).Format(time.RFC3339Nano)
		}
		row := []string{time.UnixMilli(p.Timestamp).Format(time.RFC3339Nano), strconv.FormatInt(p.ServerID, 10), p.Domain, p.Type, strconv.FormatBool(p.Received), strconv.FormatBool(p.Success), strconv.FormatFloat(p.LatencyMS, 'f', 3, 64), p.Rcode, strings.Join(p.Answers, ";"), p.Pollution, p.Override, p.Reason, p.Error, string(refs), p.Raw, strconv.FormatBool(p.Trusted), effective, string(records), strconv.Itoa(p.PolicyVersion), comparedAt}
		for i := range row {
			row[i] = csvSafe(row[i])
		}
		if e := cw.Write(row); e != nil {
			return e
		}
		cw.Flush()
		return cw.Error()
	})
	cw.Flush()
	if exportErr != nil {
		log.Printf("CSV export interrupted: %v", exportErr)
		panic(http.ErrAbortHandler)
	}
}
