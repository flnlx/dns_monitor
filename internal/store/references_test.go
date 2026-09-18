package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func trustedSource(t *testing.T, s *Store, server model.Server) model.Server {
	t.Helper()
	server.Trusted = true
	server.Enabled = true
	server, err := s.SaveServer(server)
	if err != nil {
		t.Fatal(err)
	}
	return server
}
func observation(at, ttl int64, values ...string) model.ProbeResult {
	r := model.ProbeResult{Domain: "WWW.Example.COM.", Type: "a", Timestamp: at, Received: true, Success: true, Rcode: "NOERROR", Answers: values, PolicyVersion: 2}
	for _, value := range values {
		r.Records = append(r.Records, model.AnswerRecord{Value: value, TTLSeconds: ttl, ObservedAt: at, ExpiresAt: at + ttl*1000})
	}
	return r
}
func saveObservation(t *testing.T, s *Store, server model.Server, r model.ProbeResult) {
	t.Helper()
	if err := s.SaveTrustedObservation(server, r); err != nil {
		t.Fatal(err)
	}
}
func referencesAt(t *testing.T, s *Store, at int64, hours float64) []model.Reference {
	t.Helper()
	refs, err := s.TrustedReferences(model.Domain{Name: "www.example.com", Type: "A"}, at, hours)
	if err != nil {
		t.Fatal(err)
	}
	return refs
}

func TestTrustedHistoryTTLWindowAndActualObservationTime(t *testing.T) {
	s, server := testStore(t)
	server = trustedSource(t, s, server)
	at := time.Now().UnixMilli()
	saveObservation(t, s, server, observation(at, 10, "192.0.2.1"))
	saveObservation(t, s, server, observation(at+1000, 0, "192.0.2.2"))
	refs := referencesAt(t, s, at+5000, 0)
	if len(refs) != 1 || len(refs[0].Records) != 1 || refs[0].Records[0].Value != "192.0.2.1" {
		t.Fatalf("zero history must retain only TTL fresh records: %+v", refs)
	}
	refs = referencesAt(t, s, at+20000, 1)
	if len(refs) != 1 || len(refs[0].Records) != 2 || refs[0].Records[0].ObservedAt != at {
		t.Fatalf("expired TTL history lost or read refreshed it: %+v", refs)
	}
	failed := observation(at+30000, 300, "192.0.2.1")
	failed.Success = false
	saveObservation(t, s, server, failed)
	saveObservation(t, s, server, observation(at-1000, 300, "192.0.2.1"))
	refs = referencesAt(t, s, at+30000, 1)
	if refs[0].Records[0].ObservedAt != at || refs[0].Records[0].ExpiresAt != at+10000 {
		t.Fatal("failure or older in-flight answer refreshed history")
	}
	if len(referencesAt(t, s, at+3601000, 1)) != 0 {
		t.Fatal("history expiry boundary retained expired records")
	}
	if err := s.PruneTrustedReferences(at+20000, 0); err != nil {
		t.Fatal(err)
	}
	if len(referencesAt(t, s, at+20000, 1)) != 0 {
		t.Fatal("turning history off did not remove expired entries")
	}
	saveObservation(t, s, server, observation(at, 7200, "192.0.2.3"))
	if err := s.PruneTrustedReferences(at+3601000, 0); err != nil {
		t.Fatal(err)
	}
	if len(referencesAt(t, s, at+3601000, 0)) != 1 {
		t.Fatal("pruning discarded TTL that outlived history")
	}
}

func TestTrustedReferencesSurviveReopenAndKeepSourcesSeparate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first := trustedSource(t, s, model.Server{Name: "one", Address: "1.1.1.1", Protocol: "UDP"})
	second := trustedSource(t, s, model.Server{Name: "two", Address: "9.9.9.9", Protocol: "UDP"})
	at := time.Now().UnixMilli()
	saveObservation(t, s, first, observation(at, 30, "192.0.2.1"))
	saveObservation(t, s, second, observation(at+1000, 60, "192.0.2.2"))
	other := observation(at, 60, "198.51.100.1")
	other.Domain = "other.example.com"
	saveObservation(t, s, first, other)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	refs := referencesAt(t, s, at+40000, 1)
	if len(refs) != 2 || refs[0].ServerID != first.ID || refs[1].Address != second.Address || refs[0].Records[0].ExpiresAt > at+40000 || refs[1].Records[0].ExpiresAt <= at+40000 {
		t.Fatalf("fresh/history source distinction lost on restart: %+v", refs)
	}
	if len(refs[0].Answers) != 1 {
		t.Fatal("unrelated domain expanded reference pool")
	}
}

func TestTrustEpochAndImportBlockStaleReferences(t *testing.T) {
	for _, change := range []string{"untrust", "disable", "address", "import"} {
		t.Run(change, func(t *testing.T) {
			s, server := testStore(t)
			server = trustedSource(t, s, server)
			at := time.Now().UnixMilli()
			saveObservation(t, s, server, observation(at, 60, "192.0.2.1"))
			updated := server
			switch change {
			case "untrust", "import":
				updated.Trusted = false
			case "disable":
				updated.Enabled = false
			case "address":
				updated.Address = "8.8.8.8"
			}
			if change == "import" {
				if _, err := s.ImportServers(snapshot(t, s), []model.ServerImportItem{{Row: 2, Key: server.Address, Action: "overwrite", Server: updated}}); err != nil {
					t.Fatal(err)
				}
			} else if _, err := s.SaveServer(updated); err != nil {
				t.Fatal(err)
			}
			if len(referencesAt(t, s, at+1000, 1)) != 0 {
				t.Fatal("revoked reference still available")
			}
			// Return to the old visible settings; an old request still must not re-fill the pool.
			current, err := s.SaveServer(server)
			if err != nil {
				t.Fatal(err)
			}
			if current.TrustEpoch <= server.TrustEpoch {
				t.Fatal("reference epoch did not advance")
			}
			saveObservation(t, s, server, observation(at+2000, 60, "192.0.2.2"))
			if len(referencesAt(t, s, at+3000, 1)) != 0 {
				t.Fatal("old in-flight request repopulated revoked pool")
			}
			saveObservation(t, s, current, observation(at+3000, 60, "192.0.2.3"))
			if len(referencesAt(t, s, at+4000, 1)) != 1 {
				t.Fatal("current trusted source could not refill pool")
			}
			current.Name = "renamed"
			renamed, err := s.SaveServer(current)
			if err != nil {
				t.Fatal(err)
			}
			if renamed.TrustEpoch != current.TrustEpoch || len(referencesAt(t, s, at+5000, 1)) != 1 {
				t.Fatal("unrelated rename invalidated reference pool")
			}
		})
	}
}

func TestTrustedPoolLimitsAndInvalidRecords(t *testing.T) {
	s, server := testStore(t)
	server = trustedSource(t, s, server)
	at := time.Now().UnixMilli()
	r := observation(at, 60, "2001:db8::1", "invalid", "192.0.2.1")
	r.Type = "AAAA"
	saveObservation(t, s, server, r)
	if len(referencesAt(t, s, at+1000, 1)) != 0 {
		t.Fatal("non-A record stored")
	}
	r.Type = "A"
	saveObservation(t, s, server, r)
	refs := referencesAt(t, s, at+1000, 1)
	if len(refs) != 1 || len(refs[0].Records) != 1 {
		t.Fatalf("non-IPv4 trusted entries accepted: %+v", refs)
	}
	for i := 0; i < maxTrustedSubjectAddresses+5; i++ {
		saveObservation(t, s, server, observation(at+int64(i)+1, 60, fmt.Sprintf("198.51.%d.%d", i/256, i%256)))
	}
	refs = referencesAt(t, s, at+1000, 1)
	if len(refs[0].Records) != maxTrustedSubjectAddresses {
		t.Fatalf("per-subject bound not enforced: %d", len(refs[0].Records))
	}
	for _, record := range refs[0].Records {
		if record.Value == "192.0.2.1" {
			t.Fatal("oldest reference was not evicted")
		}
	}
	// Seed the global bound directly; a normal observation must trim its overflow.
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO trusted_observations(server_id,address,domain,type,value,ttl_seconds,observed_at,expires_at) VALUES(1,'1.1.1.1',?,'A','203.0.113.1',60,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxTrustedAddresses; i++ {
		if _, err = stmt.Exec(fmt.Sprintf("%d.example", i), at-1000, at+60000); err != nil {
			t.Fatal(err)
		}
	}
	stmt.Close()
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	saveObservation(t, s, server, observation(at+2000, 60, "203.0.113.2"))
	var count int
	if err = s.db.QueryRow("SELECT COUNT(*) FROM trusted_observations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != maxTrustedAddresses {
		t.Fatalf("global trusted pool unbounded: %d", count)
	}
}
