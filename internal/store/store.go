// Package store persists configuration, raw DNS evidence, and compact query statistics.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dnsmonitor/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct {
	db            *sql.DB
	version       atomic.Uint64
	summaryMu     sync.Mutex
	cache         []model.ServerSummary
	cacheVersion  uint64
	cacheDuration int64
	cacheAt       int64
}

const schema = `
CREATE TABLE IF NOT EXISTS config (id INTEGER PRIMARY KEY CHECK(id=1), value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS servers (
 id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, provider TEXT NOT NULL, address TEXT NOT NULL,
 protocol TEXT NOT NULL, enabled INTEGER NOT NULL, trusted INTEGER NOT NULL, notes TEXT NOT NULL, created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS rounds (
 id INTEGER PRIMARY KEY, server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 started_at INTEGER NOT NULL, finished_at INTEGER NOT NULL, next_due INTEGER NOT NULL,
 covered_until INTEGER NOT NULL, auxiliary INTEGER NOT NULL DEFAULT 0,
 samples INTEGER NOT NULL, received INTEGER NOT NULL, successes INTEGER NOT NULL,
 latency_count INTEGER NOT NULL, latency_sum REAL NOT NULL, pollution INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS rounds_server_time ON rounds(server_id, auxiliary, finished_at, id);
CREATE INDEX IF NOT EXISTS rounds_time ON rounds(auxiliary, finished_at);
CREATE TABLE IF NOT EXISTS results (
 id INTEGER PRIMARY KEY AUTOINCREMENT, round_id INTEGER NOT NULL REFERENCES rounds(id) ON DELETE CASCADE,
 server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 timestamp INTEGER NOT NULL, domain TEXT NOT NULL, type TEXT NOT NULL,
 detected_pollution INTEGER NOT NULL, effective_pollution INTEGER NOT NULL, oldest_reference INTEGER NOT NULL DEFAULT 0, raw_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS results_server_time ON results(server_id,timestamp DESC,id DESC);
CREATE INDEX IF NOT EXISTS results_time ON results(timestamp);
CREATE INDEX IF NOT EXISTS results_subject ON results(server_id,domain,type,round_id);
CREATE INDEX IF NOT EXISTS results_round ON results(round_id);
CREATE INDEX IF NOT EXISTS results_reference_time ON results(oldest_reference) WHERE oldest_reference>0;
CREATE TABLE IF NOT EXISTS latency_buckets (
 round_id INTEGER NOT NULL REFERENCES rounds(id) ON DELETE CASCADE,
 bucket_ms INTEGER NOT NULL, count INTEGER NOT NULL, PRIMARY KEY(round_id,bucket_ms)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS overrides (
 server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 domain TEXT NOT NULL, type TEXT NOT NULL, verdict TEXT NOT NULL,
 note TEXT NOT NULL, updated_at INTEGER NOT NULL,
 PRIMARY KEY(server_id,domain,type)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS override_audit (
 id INTEGER PRIMARY KEY, server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 domain TEXT NOT NULL, type TEXT NOT NULL, verdict TEXT NOT NULL, note TEXT NOT NULL, timestamp INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_time ON override_audit(timestamp);
`

func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	st := &Store{db: db}
	// Incremental vacuum keeps ordinary retention cleanup bounded; WAL avoids large rewrite transactions.
	for _, q := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", "PRAGMA auto_vacuum=INCREMENTAL", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA cache_size=-4096", schema} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize database: %w", err)
		}
	}
	if err = st.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	initial, _ := json.Marshal(model.DefaultConfig())
	if _, err = db.Exec("INSERT OR IGNORE INTO config(id,value) VALUES(1,?)", string(initial)); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) GetConfig() (model.Config, error) {
	var raw string
	c := model.DefaultConfig()
	if err := s.db.QueryRow("SELECT value FROM config WHERE id=1").Scan(&raw); err != nil {
		return c, err
	}
	err := json.Unmarshal([]byte(raw), &c)
	return c, err
}
func (s *Store) SaveConfig(c model.Config) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE config SET value=? WHERE id=1", string(b))
	s.version.Add(1)
	return err
}

type scanner interface{ Scan(...any) error }

const serverColumns = "id,name,provider,address,protocol,enabled,trusted,notes,created_at,trust_epoch"

func scanServer(row scanner) (v model.Server, err error) {
	err = row.Scan(&v.ID, &v.Name, &v.Provider, &v.Address, &v.Protocol, &v.Enabled, &v.Trusted, &v.Notes, &v.CreatedAt, &v.TrustEpoch)
	return
}
func (s *Store) ListServers() ([]model.Server, error) {
	rows, err := s.db.Query("SELECT " + serverColumns + " FROM servers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]model.Server, 0)
	for rows.Next() {
		v, e := scanServer(rows)
		if e != nil {
			return nil, e
		}
		values = append(values, v)
	}
	return values, rows.Err()
}
func (s *Store) GetServer(id int64) (model.Server, error) {
	return scanServer(s.db.QueryRow("SELECT "+serverColumns+" FROM servers WHERE id=?", id))
}
func (s *Store) SaveServer(v model.Server) (model.Server, error) {
	if v.ID == 0 {
		v.CreatedAt = time.Now().UnixMilli()
		r, err := s.db.Exec("INSERT INTO servers(name,provider,address,protocol,enabled,trusted,notes,created_at) VALUES(?,?,?,?,?,?,?,?)", v.Name, v.Provider, v.Address, v.Protocol, v.Enabled, v.Trusted, v.Notes, v.CreatedAt)
		if err != nil {
			return v, err
		}
		v.ID, err = r.LastInsertId()
		if err != nil {
			return v, err
		}
	} else {
		r, err := s.db.Exec("UPDATE servers SET name=?,provider=?,address=?,protocol=?,enabled=?,trusted=?,notes=? WHERE id=?", v.Name, v.Provider, v.Address, v.Protocol, v.Enabled, v.Trusted, v.Notes, v.ID)
		if err != nil {
			return v, err
		}
		n, _ := r.RowsAffected()
		if n == 0 {
			return v, sql.ErrNoRows
		}
	}
	s.version.Add(1)
	return s.GetServer(v.ID)
}
func (s *Store) DeleteServer(id int64) error {
	r, err := s.db.Exec("DELETE FROM servers WHERE id=?", id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	s.version.Add(1)
	return nil
}

func normalizeSubject(domain, typ string) (string, string) {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), "."), strings.ToUpper(strings.TrimSpace(typ))
}

// Ordered by severity for SQL MAX aggregation. Do not reuse the legacy 0/1/2 codes.
func pollutionCode(v string) int {
	switch v {
	case "matched":
		return 1
	case "clean":
		return 2
	case "suspicious":
		return 3
	case "polluted":
		return 4
	default:
		return 0
	}
}
func pollutionName(code int) string {
	switch code {
	case 1:
		return "matched"
	case 2:
		return "clean"
	case 3:
		return "suspicious"
	case 4:
		return "polluted"
	default:
		return "unknown"
	}
}
func effectiveCode(detected int, policy int, override string) int {
	if override == "clean" {
		return 2
	}
	if override == "polluted" {
		return 4
	}
	// An old automatic conviction used exact IP equality and cannot establish F
	// under the TTL/history policy. Keep the original evidence for inspection.
	if policy < 2 && detected == 4 {
		return 0
	}
	return detected
}

// SaveRound commits a round and all raw answers atomically. Auxiliary reference lookups
// remain exportable evidence but never alter the monitored server's scheduled timeline.
func (s *Store) SaveRound(v model.Round) error {
	if v.ServerID <= 0 || v.StartedAt <= 0 || v.FinishedAt < v.StartedAt || v.NextDue <= v.FinishedAt {
		return errors.New("invalid probe round timestamps or server")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var received, successes int64
	var latencySum float64
	buckets := map[int64]int64{}
	for _, r := range v.Results {
		if r.Received {
			received++
		}
		if r.Success {
			successes++
			if r.LatencyMS >= 0 && !math.IsNaN(r.LatencyMS) && !math.IsInf(r.LatencyMS, 0) {
				latencySum += r.LatencyMS
				buckets[int64(math.Ceil(math.Min(r.LatencyMS, 3600000)))]++
			}
		}
	}
	var latencyCount int64
	for _, n := range buckets {
		latencyCount += n
	}
	coveredUntil := v.NextDue
	if !v.Auxiliary {
		var next sql.NullInt64
		if err = tx.QueryRow("SELECT MIN(finished_at) FROM rounds WHERE server_id=? AND auxiliary=0 AND finished_at>=?", v.ServerID, v.FinishedAt).Scan(&next); err != nil {
			return err
		}
		if next.Valid && next.Int64 < coveredUntil {
			coveredUntil = next.Int64
		}
	}
	insert, err := tx.Exec("INSERT INTO rounds(server_id,started_at,finished_at,next_due,covered_until,auxiliary,samples,received,successes,latency_count,latency_sum) VALUES(?,?,?,?,?,?,?,?,?,?,?)", v.ServerID, v.StartedAt, v.FinishedAt, v.NextDue, coveredUntil, v.Auxiliary, len(v.Results), received, successes, latencyCount, latencySum)
	if err != nil {
		return err
	}
	roundID, err := insert.LastInsertId()
	if err != nil {
		return err
	}
	if !v.Auxiliary {
		if _, err = tx.Exec("UPDATE rounds SET covered_until=MIN(next_due,?) WHERE id=(SELECT id FROM rounds WHERE server_id=? AND auxiliary=0 AND id<>? AND finished_at<=? ORDER BY finished_at DESC,id DESC LIMIT 1)", v.FinishedAt, v.ServerID, roundID, v.FinishedAt); err != nil {
			return err
		}
	}
	maxPollution := 0
	for _, r := range v.Results {
		r.ID = 0
		r.RoundID = roundID
		r.ServerID = v.ServerID
		r.Override = ""
		r.EffectivePollution = ""
		if r.Timestamp == 0 {
			r.Timestamp = v.FinishedAt
		}
		r.Domain, r.Type = normalizeSubject(r.Domain, r.Type)
		var override string
		err = tx.QueryRow("SELECT verdict FROM overrides WHERE server_id=? AND domain=? AND type=?", v.ServerID, r.Domain, r.Type).Scan(&override)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		code := effectiveCode(pollutionCode(r.Pollution), r.PolicyVersion, override)
		if code > maxPollution {
			maxPollution = code
		}
		raw, e := json.Marshal(r)
		if e != nil {
			return e
		}
		if _, err = tx.Exec("INSERT INTO results(round_id,server_id,timestamp,domain,type,detected_pollution,effective_pollution,oldest_reference,raw_json,policy_version) VALUES(?,?,?,?,?,?,?,?,?,?)", roundID, v.ServerID, r.Timestamp, r.Domain, r.Type, pollutionCode(r.Pollution), code, oldestReference(r.References), string(raw), r.PolicyVersion); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("UPDATE rounds SET pollution=? WHERE id=?", maxPollution, roundID); err != nil {
		return err
	}
	for bucket, n := range buckets {
		if _, err = tx.Exec("INSERT INTO latency_buckets(round_id,bucket_ms,count) VALUES(?,?,?)", roundID, bucket, n); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err == nil {
		s.version.Add(1)
	}
	return err
}

func (s *Store) SaveOverride(v model.Override) error {
	if v.Verdict != "auto" && v.Verdict != "clean" && v.Verdict != "polluted" {
		return errors.New("invalid verdict")
	}
	v.Domain, v.Type = normalizeSubject(v.Domain, v.Type)
	if v.Domain == "" || v.Type == "" {
		return errors.New("domain and type are required")
	}
	v.UpdatedAt = time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if v.Verdict == "polluted" {
		var trusted bool
		if err = tx.QueryRow("SELECT trusted FROM servers WHERE id=?", v.ServerID).Scan(&trusted); err != nil {
			return err
		}
		if trusted {
			return ErrTrustedOverride
		}
	}
	// Keep an audit event even for auto reset; configuration and original evidence are distinct.
	if _, err = tx.Exec("INSERT INTO override_audit(server_id,domain,type,verdict,note,timestamp) VALUES(?,?,?,?,?,?)", v.ServerID, v.Domain, v.Type, v.Verdict, v.Note, v.UpdatedAt); err != nil {
		return err
	}
	if v.Verdict == "auto" {
		_, err = tx.Exec("DELETE FROM overrides WHERE server_id=? AND domain=? AND type=?", v.ServerID, v.Domain, v.Type)
	} else {
		_, err = tx.Exec("INSERT INTO overrides(server_id,domain,type,verdict,note,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(server_id,domain,type) DO UPDATE SET verdict=excluded.verdict,note=excluded.note,updated_at=excluded.updated_at", v.ServerID, v.Domain, v.Type, v.Verdict, v.Note, v.UpdatedAt)
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec("UPDATE results SET effective_pollution=CASE ? WHEN 'clean' THEN 2 WHEN 'polluted' THEN 4 ELSE CASE WHEN policy_version<2 AND detected_pollution=4 THEN 0 ELSE detected_pollution END END WHERE server_id=? AND domain=? AND type=?", v.Verdict, v.ServerID, v.Domain, v.Type)
	if err != nil {
		return err
	}
	_, err = tx.Exec("UPDATE rounds SET pollution=COALESCE((SELECT MAX(effective_pollution) FROM results WHERE round_id=rounds.id),0) WHERE id IN (SELECT round_id FROM results WHERE server_id=? AND domain=? AND type=?)", v.ServerID, v.Domain, v.Type)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		s.version.Add(1)
	}
	return err
}

func decodeResult(row scanner) (model.ProbeResult, error) {
	var r model.ProbeResult
	var raw, override string
	var trusted bool
	var effective int
	var id int64
	if err := row.Scan(&id, &raw, &override, &trusted, &effective); err != nil {
		return r, err
	}
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return r, err
	}
	r.ID = id
	r.Override = override
	r.Trusted = trusted
	r.EffectivePollution = pollutionName(effective)
	if trusted {
		r.EffectivePollution = "matched"
	}
	if r.Answers == nil {
		r.Answers = []string{}
	}
	if r.References == nil {
		r.References = []model.Reference{}
	}
	return r, nil
}

const resultSelect = `SELECT r.id,r.raw_json,COALESCE(o.verdict,''),s.trusted,r.effective_pollution FROM results r JOIN servers s ON s.id=r.server_id LEFT JOIN overrides o ON o.server_id=r.server_id AND o.domain=r.domain AND o.type=r.type `

func (s *Store) Results(serverID int64, limit int, before int64) ([]model.ProbeResult, error) {
	return s.ResultsPage(serverID, limit, before, 0)
}
func (s *Store) ResultsPage(serverID int64, limit int, before, beforeID int64) ([]model.ProbeResult, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	if before <= 0 {
		before = math.MaxInt64
	}
	condition := "r.timestamp<?"
	args := []any{serverID, before}
	if beforeID > 0 {
		condition = "(r.timestamp<? OR (r.timestamp=? AND r.id<?))"
		args = append(args, before, beforeID)
	}
	args = append(args, limit)
	rows, err := s.db.Query(resultSelect+"WHERE r.server_id=? AND "+condition+" ORDER BY r.timestamp DESC,r.id DESC LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]model.ProbeResult, 0, limit)
	for rows.Next() {
		r, e := decodeResult(rows)
		if e != nil {
			return nil, e
		}
		values = append(values, r)
	}
	return values, rows.Err()
}
func (s *Store) WalkResults(serverID, since int64, fn func(model.ProbeResult) error) error {
	var snapshotID int64
	if err := s.db.QueryRow("SELECT COALESCE(MAX(id),0) FROM results").Scan(&snapshotID); err != nil {
		return err
	}
	before, beforeID := int64(math.MaxInt64), int64(math.MaxInt64)
	for {
		q := resultSelect + "WHERE r.timestamp>=? AND r.id<=? AND (r.timestamp<? OR (r.timestamp=? AND r.id<?))"
		args := []any{since, snapshotID, before, before, beforeID}
		if serverID > 0 {
			q += " AND r.server_id=?"
			args = append(args, serverID)
		}
		q += " ORDER BY r.timestamp DESC,r.id DESC LIMIT 200"
		rows, err := s.db.Query(q, args...)
		if err != nil {
			return err
		}
		batch := make([]model.ProbeResult, 0, 200)
		for rows.Next() {
			r, e := decodeResult(rows)
			if e != nil {
				rows.Close()
				return e
			}
			batch = append(batch, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		// Release the sole database connection before potentially slow network callbacks.
		for _, r := range batch {
			if err = fn(r); err != nil {
				return err
			}
		}
		if len(batch) < 200 {
			return nil
		}
		last := batch[len(batch)-1]
		before, beforeID = last.Timestamp, last.ID
	}
}

// Cleanup deletes bounded batches in short transactions, including raw reference evidence.
// Active manual overrides are configuration; their audit history follows the same retention.
func (s *Store) Cleanup(before int64) error {
	if _, err := s.db.Exec("DELETE FROM trusted_observations WHERE observed_at<?", before); err != nil {
		return err
	}
	// Cached reference evidence can predate a retained target. Prune it by indexed timestamp.
	for {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		rows, err := tx.Query("SELECT id,raw_json FROM results WHERE oldest_reference>0 AND oldest_reference<? LIMIT 250", before)
		if err != nil {
			tx.Rollback()
			return err
		}
		type replacement struct {
			id     int64
			result model.ProbeResult
		}
		batch := make([]replacement, 0, 250)
		for rows.Next() {
			var v replacement
			var raw string
			if err = rows.Scan(&v.id, &raw); err != nil {
				rows.Close()
				tx.Rollback()
				return err
			}
			if err = json.Unmarshal([]byte(raw), &v.result); err != nil {
				rows.Close()
				tx.Rollback()
				return err
			}
			batch = append(batch, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			tx.Rollback()
			return err
		}
		for _, v := range batch {
			kept := make([]model.Reference, 0, len(v.result.References))
			for _, ref := range v.result.References {
				if len(ref.Records) > 0 {
					records := make([]model.AnswerRecord, 0, len(ref.Records))
					answers := make([]string, 0, len(ref.Records))
					for _, record := range ref.Records {
						if record.ObservedAt >= before {
							records = append(records, record)
							answers = append(answers, record.Value)
						}
					}
					ref.Records, ref.Answers = records, answers
					if len(records) == 0 {
						continue
					}
				}
				if ref.Timestamp >= before {
					kept = append(kept, ref)
				}
			}
			v.result.References = kept
			raw, e := json.Marshal(v.result)
			if e != nil {
				tx.Rollback()
				return e
			}
			if _, err = tx.Exec("UPDATE results SET raw_json=?,oldest_reference=? WHERE id=?", string(raw), oldestReference(kept), v.id); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		if len(batch) < 250 {
			break
		}
	}
	// Whole expired rounds go first; cascading foreign keys remove their raw and histogram rows.
	for {
		r, err := s.db.Exec("DELETE FROM rounds WHERE id IN (SELECT id FROM rounds WHERE finished_at<? LIMIT 1000)", before)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n < 1000 {
			break
		}
	}
	// A cutoff can fall inside a multi-domain round. Rebuild only those partial rounds.
	for {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		rows, err := tx.Query("SELECT DISTINCT round_id FROM results WHERE timestamp<? LIMIT 100", before)
		if err != nil {
			tx.Rollback()
			return err
		}
		ids := make([]int64, 0, 100)
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				tx.Rollback()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			tx.Rollback()
			return err
		}
		for _, id := range ids {
			if _, err = tx.Exec("DELETE FROM results WHERE round_id=? AND timestamp<?", id, before); err != nil {
				tx.Rollback()
				return err
			}
			if err = rebuildRound(tx, id); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		if len(ids) < 100 {
			break
		}
	}
	for _, q := range []string{
		"DELETE FROM override_audit WHERE id IN (SELECT id FROM override_audit WHERE timestamp<? LIMIT 1000)",
	} {
		for {
			r, err := s.db.Exec(q, before)
			if err != nil {
				return err
			}
			n, err := r.RowsAffected()
			if err != nil {
				return err
			}
			if n < 1000 {
				break
			}
		}
	}
	s.version.Add(1)
	if _, err := s.db.Exec("PRAGMA incremental_vacuum(256)"); err != nil {
		return err
	}
	_, err := s.db.Exec("PRAGMA wal_checkpoint(PASSIVE)")
	return err
}

type accumulator struct {
	m                              model.Metrics
	covered, available, latencySum float64
	successes, latencyCount        int64
	pollution                      int
	buckets                        map[int64]int64
}

func (a *accumulator) finish(window int64) model.Metrics {
	m := a.m
	if a.covered > 0 {
		m.Availability = 100 * a.available / a.covered
	}
	if window > 0 {
		m.Coverage = math.Min(100, 100*a.covered/float64(window))
	}
	if m.Samples > 0 {
		m.SuccessRate = 100 * float64(a.successes) / float64(m.Samples)
	}
	if a.latencyCount > 0 {
		m.AverageMS = a.latencySum / float64(a.latencyCount)
		keys := make([]int64, 0, len(a.buckets))
		for k := range a.buckets {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		threshold := int64(math.Ceil(float64(a.latencyCount) * .95))
		var count int64
		for _, k := range keys {
			count += a.buckets[k]
			if count >= threshold {
				m.P95MS = float64(k)
				break
			}
		}
	}
	m.Pollution = pollutionName(a.pollution)
	// 50 ms earns the full latency component; 2 s or more earns zero.
	latencyScore := math.Max(0, math.Min(100, 100*(2000-m.P95MS)/1950))
	if a.latencyCount == 0 {
		latencyScore = 0
	}
	m.Score = .45*m.Availability + .35*m.SuccessRate + .20*latencyScore
	switch {
	case a.pollution == 4:
		m.Grade = "F"
		m.Score = 0
	case a.pollution == 3:
		m.Grade = "E"
	case m.Samples < 10 || a.covered < 30*60*1000:
		m.Grade = "pending"
	case m.Score >= 95:
		m.Grade = "A"
	case m.Score >= 85:
		m.Grade = "B"
	case m.Score >= 70:
		m.Grade = "C"
	default:
		m.Grade = "D"
	}
	return m
}

func (s *Store) Summary(since, now int64) ([]model.ServerSummary, error) {
	if now <= since {
		return nil, errors.New("invalid time range")
	}
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()
	version := s.version.Load()
	if s.cache != nil && s.cacheVersion == version && s.cacheDuration == now-since && now >= s.cacheAt && now-s.cacheAt < 5000 {
		return append([]model.ServerSummary{}, s.cache...), nil
	}
	servers, err := s.ListServers()
	if err != nil {
		return nil, err
	}
	values := make([]model.ServerSummary, 0, len(servers))
	acc := map[int64]*accumulator{}
	for _, v := range servers {
		acc[v.ID] = &accumulator{buckets: map[int64]int64{}}
	}
	q := `SELECT server_id,
 SUM(CASE WHEN finished_at>=? THEN samples ELSE 0 END),
 SUM(CASE WHEN finished_at>=? THEN successes ELSE 0 END),
 SUM(CASE WHEN finished_at>=? THEN latency_count ELSE 0 END),
 SUM(CASE WHEN finished_at>=? THEN latency_sum ELSE 0 END),
 SUM(MAX(0,MIN(covered_until,?)-MAX(finished_at,?))),
 SUM(MAX(0,MIN(covered_until,?)-MAX(finished_at,?))*1.0*received/MAX(1,samples)),
 MAX(CASE WHEN finished_at>=? THEN pollution ELSE 0 END)
 FROM rounds WHERE auxiliary=0 AND finished_at<=? AND (finished_at>=? OR covered_until>?) GROUP BY server_id`
	rows, err := s.db.Query(q, since, since, since, since, now, since, now, since, since, now, since, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var a accumulator
		if err = rows.Scan(&id, &a.m.Samples, &a.successes, &a.latencyCount, &a.latencySum, &a.covered, &a.available, &a.pollution); err != nil {
			rows.Close()
			return nil, err
		}
		a.buckets = map[int64]int64{}
		acc[id] = &a
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = s.db.Query(`SELECT r.server_id,b.bucket_ms,SUM(b.count) FROM rounds r JOIN latency_buckets b ON b.round_id=r.id WHERE r.auxiliary=0 AND r.finished_at>=? AND r.finished_at<=? GROUP BY r.server_id,b.bucket_ms`, since, now)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, b, n int64
		if err = rows.Scan(&id, &b, &n); err != nil {
			rows.Close()
			return nil, err
		}
		if a := acc[id]; a != nil {
			a.buckets[b] = n
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, v := range servers {
		value := model.ServerSummary{Server: v, Metrics: acc[v.ID].finishForServer(now-since, v.Trusted)}
		err = s.db.QueryRow("SELECT finished_at,next_due,successes>0 FROM rounds WHERE server_id=? AND auxiliary=0 ORDER BY finished_at DESC,id DESC LIMIT 1", v.ID).Scan(&value.LastProbe, &value.NextDue, &value.LastSuccess)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if value.LastProbe > 0 {
			err = s.db.QueryRow(`SELECT COUNT(*) FROM rounds WHERE server_id=? AND auxiliary=0 AND received=0 AND finished_at>COALESCE((SELECT MAX(finished_at) FROM rounds WHERE server_id=? AND auxiliary=0 AND received>0),0)`, v.ID, v.ID).Scan(&value.Failures)
			if err != nil {
				return nil, err
			}
		}
		values = append(values, value)
	}
	if s.version.Load() == version {
		s.cache = append([]model.ServerSummary{}, values...)
		s.cacheAt = now
		s.cacheDuration = now - since
		s.cacheVersion = version
	}
	return values, nil
}

func (s *Store) History(serverID, since, now, stepMS int64) ([]model.HistoryPoint, model.Metrics, error) {
	if now <= since {
		return nil, model.Metrics{}, errors.New("invalid time range")
	}
	server, err := s.GetServer(serverID)
	if err != nil {
		return nil, model.Metrics{}, err
	}
	if stepMS < 1 {
		stepMS = 5 * 60 * 1000
	}
	if minimum := (now - since + 999) / 1000; stepMS < minimum {
		stepMS = minimum
	}
	count := int((now - since + stepMS - 1) / stepMS)
	points := make([]accumulator, count)
	total := accumulator{buckets: map[int64]int64{}}
	for i := range points {
		points[i].buckets = map[int64]int64{}
	}
	rows, err := s.db.Query(`SELECT finished_at,covered_until,samples,received,successes,latency_count,latency_sum,pollution FROM rounds WHERE server_id=? AND auxiliary=0 AND finished_at<=? AND (finished_at>=? OR covered_until>?) ORDER BY finished_at`, serverID, now, since, since)
	if err != nil {
		return nil, model.Metrics{}, err
	}
	for rows.Next() {
		var at, end, samples, received, successes, latencyCount int64
		var latencySum float64
		var pollution int
		if err = rows.Scan(&at, &end, &samples, &received, &successes, &latencyCount, &latencySum, &pollution); err != nil {
			rows.Close()
			return nil, model.Metrics{}, err
		}
		if at >= since {
			i := int((at - since) / stepMS)
			if i >= count {
				i = count - 1
			}
			a := &points[i]
			a.m.Samples += samples
			a.successes += successes
			a.latencyCount += latencyCount
			a.latencySum += latencySum
			if pollution > a.pollution {
				a.pollution = pollution
			}
			total.m.Samples += samples
			total.successes += successes
			total.latencyCount += latencyCount
			total.latencySum += latencySum
			if pollution > total.pollution {
				total.pollution = pollution
			}
		}
		start := max(at, since)
		end = min(end, now)
		if end <= start {
			continue
		}
		duration := float64(end - start)
		ratio := float64(received) / float64(max(samples, 1))
		total.covered += duration
		total.available += duration * ratio
		for i := int((start - since) / stepMS); i < count; i++ {
			bucketStart := since + int64(i)*stepMS
			if bucketStart >= end {
				break
			}
			d := float64(min(end, bucketStart+stepMS) - max(start, bucketStart))
			if d > 0 {
				points[i].covered += d
				points[i].available += d * ratio
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, model.Metrics{}, err
	}
	rows, err = s.db.Query(`SELECT MIN(?,(r.finished_at-?)/?),b.bucket_ms,SUM(b.count) FROM rounds r JOIN latency_buckets b ON b.round_id=r.id WHERE r.server_id=? AND r.auxiliary=0 AND r.finished_at>=? AND r.finished_at<=? GROUP BY 1,b.bucket_ms`, count-1, since, stepMS, serverID, since, now)
	if err != nil {
		return nil, model.Metrics{}, err
	}
	for rows.Next() {
		var i int
		var bucket, n int64
		if err = rows.Scan(&i, &bucket, &n); err != nil {
			rows.Close()
			return nil, model.Metrics{}, err
		}
		points[i].buckets[bucket] += n
		total.buckets[bucket] += n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, model.Metrics{}, err
	}
	values := make([]model.HistoryPoint, count)
	for i := range points {
		at := since + int64(i)*stepMS
		values[i] = model.HistoryPoint{Timestamp: at, Metrics: points[i].finishForServer(min(stepMS, now-at), server.Trusted)}
	}
	return values, total.finishForServer(now-since, server.Trusted), nil
}

func oldestReference(refs []model.Reference) int64 {
	var at int64
	for _, ref := range refs {
		if ref.Timestamp > 0 && (at == 0 || ref.Timestamp < at) {
			at = ref.Timestamp
		}
		for _, record := range ref.Records {
			if record.ObservedAt > 0 && (at == 0 || record.ObservedAt < at) {
				at = record.ObservedAt
			}
		}
	}
	return at
}
func rebuildRound(tx *sql.Tx, id int64) error {
	rows, err := tx.Query("SELECT raw_json,effective_pollution FROM results WHERE round_id=?", id)
	if err != nil {
		return err
	}
	var samples, received, successes, latencyCount int64
	var latencySum float64
	var pollution int
	buckets := map[int64]int64{}
	for rows.Next() {
		var raw string
		var effective int
		var r model.ProbeResult
		if err = rows.Scan(&raw, &effective); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			rows.Close()
			return err
		}
		samples++
		if r.Received {
			received++
		}
		if r.Success {
			successes++
			if r.LatencyMS >= 0 {
				latencyCount++
				latencySum += r.LatencyMS
				buckets[int64(math.Ceil(math.Min(r.LatencyMS, 3600000)))]++
			}
		}
		if effective > pollution {
			pollution = effective
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if samples == 0 {
		_, err = tx.Exec("DELETE FROM rounds WHERE id=?", id)
		return err
	}
	if _, err = tx.Exec("UPDATE rounds SET samples=?,received=?,successes=?,latency_count=?,latency_sum=?,pollution=? WHERE id=?", samples, received, successes, latencyCount, latencySum, pollution, id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM latency_buckets WHERE round_id=?", id); err != nil {
		return err
	}
	for bucket, n := range buckets {
		if _, err = tx.Exec("INSERT INTO latency_buckets(round_id,bucket_ms,count) VALUES(?,?,?)", id, bucket, n); err != nil {
			return err
		}
	}
	return nil
}
