package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestFourVerdictsAggregateBySeverityAndTrustedExemptsEF(t *testing.T) {
	for _, tc := range []struct {
		states        []string
		status, grade string
	}{
		{[]string{"matched", "matched"}, "matched", "A"},
		{[]string{"matched", "clean"}, "clean", "A"},
		{[]string{"matched", "clean", "suspicious"}, "suspicious", "E"},
		{[]string{"suspicious", "polluted", "matched"}, "polluted", "F"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			s, server := testStore(t)
			at := time.Now().Add(-time.Hour).UnixMilli()
			r := round(server.ID, at, 3600000, true, true, 12, 10, tc.status)
			for i := range r.Results {
				r.Results[i].Pollution = tc.states[i%len(tc.states)]
			}
			mustSave(t, s, r)
			check := func(status, grade string) {
				t.Helper()
				summary, err := s.Summary(at, at+3600000)
				if err != nil {
					t.Fatal(err)
				}
				points, total, err := s.History(server.ID, at, at+3600000, 3600000)
				if err != nil {
					t.Fatal(err)
				}
				for _, metric := range []model.Metrics{summary[0].Metrics, points[0].Metrics, total} {
					if metric.Pollution != status || metric.Grade != grade {
						t.Fatalf("want %s/%s: %+v", status, grade, metric)
					}
				}
			}
			check(tc.status, tc.grade)
			server.Trusted = true
			if _, err := s.SaveServer(server); err != nil {
				t.Fatal(err)
			}
			check("matched", "A")
			results, err := s.Results(server.ID, 20, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range results {
				if r.EffectivePollution != "matched" {
					t.Fatalf("unexempt result: %+v", r)
				}
			}
		})
	}
}

func TestLegacyDatabaseMigrationPreservesEvidenceAndManualVerdicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO config(id,value) VALUES(1,'{"listen":"127.0.0.1:8080","concurrency":0,"domains":[{"name":"keep.example","type":"A"}]}')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO servers(id,name,provider,address,protocol,enabled,trusted,notes,created_at) VALUES(1,'legacy','','1.1.1.1','UDP',1,0,'',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO rounds(id,server_id,started_at,finished_at,next_due,covered_until,auxiliary,samples,received,successes,latency_count,latency_sum,pollution) VALUES(1,1,990,1000,3601000,3601000,0,12,12,12,12,120,2)`); err != nil {
		t.Fatal(err)
	}
	for i, domain := range []string{"automatic.example", "manual.example", "cleared.example", "clean.example"} {
		state, code := "polluted", 2
		if i == 3 {
			state, code = "clean", 1
		}
		raw, _ := json.Marshal(model.ProbeResult{ServerID: 1, Timestamp: 1000, Domain: domain, Type: "A", Pollution: state, Success: true, Received: true})
		if _, err = db.Exec(`INSERT INTO results(round_id,server_id,timestamp,domain,type,detected_pollution,effective_pollution,raw_json) VALUES(1,1,1000,?,'A',?,?,?)`, domain, code, code, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO overrides(server_id,domain,type,verdict,note,updated_at) VALUES(1,'manual.example','A','polluted','keep',1001),(1,'cleared.example','A','clean','keep',1001)`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	config, err := s.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.ReferenceHistoryHours != 1 || config.Domains[0].Name != "keep.example" || config.Concurrency != 0 {
		t.Fatalf("migration overwrote settings: %+v", config)
	}
	results, err := s.Results(1, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{"automatic.example": "unknown", "manual.example": "polluted", "cleared.example": "clean", "clean.example": "clean"}
	for _, result := range results {
		if result.EffectivePollution != wants[result.Domain] {
			t.Fatalf("wrong migrated verdict: %+v", result)
		}
		if result.Domain != "clean.example" && result.Pollution != "polluted" {
			t.Fatal("migration erased detected evidence")
		}
	}
	for _, domain := range []string{"manual.example", "cleared.example"} {
		if err = s.SaveOverride(model.Override{ServerID: 1, Domain: domain, Type: "A", Verdict: "auto"}); err != nil {
			t.Fatal(err)
		}
	}
	results, err = s.Results(1, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Domain != "clean.example" && result.EffectivePollution != "unknown" {
			t.Fatalf("auto reset revived legacy F: %+v", result)
		}
	}
	config.ReferenceHistoryHours = 0
	if err = s.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	config, err = s.GetConfig()
	if err != nil || config.ReferenceHistoryHours != 0 {
		t.Fatalf("explicit zero lost: %+v %v", config, err)
	}
	var version, code int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT pollution FROM rounds WHERE id=1").Scan(&code); err != nil {
		t.Fatal(err)
	}
	if version != 2 || code != 2 {
		t.Fatalf("wrong migration or MAX order: version=%d pollution=%d", version, code)
	}
}

func TestRawRecordsAndReferenceEvidenceSurvivePersistence(t *testing.T) {
	s, server := testStore(t)
	at := time.Now().UnixMilli()
	r := round(server.ID, at, 1000, true, true, 1, 10, "clean")
	record := model.AnswerRecord{Value: "192.0.2.1", TTLSeconds: 30, ObservedAt: at - 30000, ExpiresAt: at}
	r.Results[0].Records = []model.AnswerRecord{{Value: "192.0.2.1", TTLSeconds: 2, ObservedAt: at, ExpiresAt: at + 2000}}
	r.Results[0].References = []model.Reference{{ServerID: 99, Address: "tls://1.1.1.1", Timestamp: record.ObservedAt, Records: []model.AnswerRecord{record}, Answers: []string{record.Value}, Success: true}}
	mustSave(t, s, r)
	results, err := s.Results(server.ID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := results[0]
	if got.PolicyVersion != 2 || got.EffectivePollution != "clean" || len(got.Records) != 1 || len(got.References) != 1 || got.References[0].Records[0] != record {
		t.Fatalf("TTL/source evidence lost: %+v", got)
	}
}

func TestCleanupPrunesOldRecordInsideRecentlyUpdatedReference(t *testing.T) {
	s, server := testStore(t)
	server = trustedSource(t, s, server)
	cutoff := time.Now().Add(-model.Retention).UnixMilli()
	old := model.AnswerRecord{Value: "192.0.2.1", TTLSeconds: 60, ObservedAt: cutoff - 1, ExpiresAt: cutoff + 59999}
	current := model.AnswerRecord{Value: "192.0.2.2", TTLSeconds: 60, ObservedAt: cutoff + 1, ExpiresAt: cutoff + 60001}
	r := round(server.ID, cutoff+1000, 1000, true, true, 1, 10, "matched")
	r.Results[0].References = []model.Reference{{ServerID: server.ID, Timestamp: current.ObservedAt, Records: []model.AnswerRecord{old, current}, Answers: []string{old.Value, current.Value}}}
	mustSave(t, s, r)
	saveObservation(t, s, server, observation(cutoff-1, 60, old.Value))
	saveObservation(t, s, server, observation(cutoff+1, 60, current.Value))
	if err := s.Cleanup(cutoff); err != nil {
		t.Fatal(err)
	}
	results, err := s.Results(server.ID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	refs := results[0].References
	if len(refs) != 1 || len(refs[0].Records) != 1 || refs[0].Records[0] != current || len(refs[0].Answers) != 1 || refs[0].Answers[0] != current.Value {
		t.Fatalf("old nested evidence survived: %+v", refs)
	}
	var count int
	if err = s.db.QueryRow("SELECT COUNT(*) FROM trusted_observations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("old pool evidence survived: %d", count)
	}
}
