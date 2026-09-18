package store

import (
	"errors"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func snapshot(t *testing.T, s *Store) string {
	t.Helper()
	servers, err := s.ListServers()
	if err != nil {
		t.Fatal(err)
	}
	return ServerSnapshot(servers)
}
func imported(address, name string) model.Server {
	return model.Server{Name: name, Address: address, Protocol: "UDP", Enabled: true}
}

func TestServerSnapshotStableAndDoesNotMutateInput(t *testing.T) {
	a := model.Server{ID: 2, Address: "2.2.2.2", CreatedAt: 11}
	b := model.Server{ID: 1, Address: "1.1.1.1", CreatedAt: 10}
	input := []model.Server{a, b}
	first := ServerSnapshot(input)
	if first != ServerSnapshot([]model.Server{b, a}) || input[0].ID != 2 {
		t.Fatal("snapshot is order-sensitive or reordered caller input")
	}
	b.CreatedAt++
	if first == ServerSnapshot([]model.Server{b, a}) {
		t.Fatal("snapshot ignored creation timestamp")
	}
	if ServerSnapshot(nil) != ServerSnapshot([]model.Server{}) {
		t.Fatal("empty snapshot representations differ")
	}
}

func TestImportOverwritesInPlaceAndPreservesHistory(t *testing.T) {
	s, original := testStore(t)
	at := time.Now().Add(-time.Hour).UnixMilli()
	mustSave(t, s, round(original.ID, at, 3600000, true, true, 10, 10, "polluted"))
	// Prime the summary cache; an import trust change must invalidate it.
	if _, err := s.Summary(at, at+3600000); err != nil {
		t.Fatal(err)
	}
	overwrite := original
	overwrite.Name = "updated"
	overwrite.Provider = "new provider"
	overwrite.Notes = "import"
	overwrite.Trusted = true
	overwrite.CreatedAt = 1
	result, err := s.ImportServers(snapshot(t, s), []model.ServerImportItem{
		{Row: 2, Key: "1.1.1.1", Action: "overwrite", Server: overwrite},
		{Row: 3, Key: "8.8.8.8", Action: "create", Server: imported("8.8.8.8", "new")},
		{Row: 4, Action: "skip"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Updated != 1 || result.Skipped != 1 {
		t.Fatalf("unexpected import totals: %+v", result)
	}
	got, err := s.GetServer(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt != original.CreatedAt || got.Name != "updated" || !got.Trusted || got.Provider != "new provider" {
		t.Fatalf("overwrite changed identity or lost fields: %+v", got)
	}
	results, err := s.Results(original.ID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 10 || !results[0].Trusted || results[0].Pollution != "polluted" {
		t.Fatalf("overwrite lost history: %+v", results)
	}
	summary, err := s.Summary(at, at+3600000)
	if err != nil {
		t.Fatal(err)
	}
	if summary[0].Metrics.Grade != "A" {
		t.Fatalf("import left stale cached trust: %+v", summary[0])
	}
}

func TestImportSameFileDuplicateOverwritesEarlierCreate(t *testing.T) {
	s, _ := testStore(t)
	result, err := s.ImportServers(snapshot(t, s), []model.ServerImportItem{
		{Row: 2, Key: "8.8.8.8", Action: "create", Server: imported("8.8.8.8:53", "first")},
		{Row: 3, Key: "8.8.8.8", Action: "overwrite", Server: imported("8.8.8.8", "last")},
		{Row: 4, Key: "8.8.8.8", Action: "skip", Server: imported("8.8.8.8", "skipped")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Updated != 1 || result.Skipped != 1 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	servers, err := s.ListServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[1].Name != "last" || servers[1].CreatedAt == 0 {
		t.Fatalf("same-file duplicate created multiple rows: %+v", servers)
	}
}

func TestImportStalePreviewHasNoSideEffects(t *testing.T) {
	s, server := testStore(t)
	preview := snapshot(t, s)
	server.Name = "changed after preview"
	if _, err := s.SaveServer(server); err != nil {
		t.Fatal(err)
	}
	result, err := s.ImportServers(preview, []model.ServerImportItem{{Row: 2, Key: "8.8.8.8", Action: "create", Server: imported("8.8.8.8", "new")}})
	if !errors.Is(err, ErrImportConflict) || result != (model.ServerImportResult{}) {
		t.Fatalf("stale preview accepted: %+v %v", result, err)
	}
	servers, e := s.ListServers()
	if e != nil {
		t.Fatal(e)
	}
	if len(servers) != 1 || servers[0].Name != server.Name {
		t.Fatalf("stale import mutated servers: %+v", servers)
	}
}

func TestImportRollsBackEarlierRowsOnUnresolvedOverwrite(t *testing.T) {
	s, server := testStore(t)
	before := snapshot(t, s)
	updated := server
	updated.Name = "must roll back"
	result, err := s.ImportServers(before, []model.ServerImportItem{
		{Row: 2, Key: "8.8.8.8", Action: "create", Server: imported("8.8.8.8", "must disappear")},
		{Row: 3, Key: server.Address, Action: "overwrite", Server: updated},
		{Row: 4, Key: "9.9.9.9", Action: "overwrite", Server: imported("9.9.9.9", "unresolved")},
	})
	if err == nil || result != (model.ServerImportResult{}) {
		t.Fatalf("bad overwrite was accepted: %+v %v", result, err)
	}
	if snapshot(t, s) != before {
		t.Fatal("failed import left partial mutations")
	}
}

func TestImportCanonicalAliasesAndCreateGuard(t *testing.T) {
	s, server := testStore(t)
	server.Address = "1.1.1.1:53"
	var err error
	server, err = s.SaveServer(server)
	if err != nil {
		t.Fatal(err)
	}
	updated := server
	updated.Name = "alias accepted"
	updated.Address = "1.1.1.1"
	if _, err = s.ImportServers(snapshot(t, s), []model.ServerImportItem{{Row: 2, Key: "1.1.1.1", Action: "overwrite", Server: updated}}); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, s)
	if _, err = s.ImportServers(before, []model.ServerImportItem{{Row: 2, Key: "1.1.1.1", Action: "create", Server: updated}}); err == nil {
		t.Fatal("create unexpectedly overwrote existing ID")
	}
	if snapshot(t, s) != before {
		t.Fatal("rejected create changed storage")
	}
	if _, err = s.ImportServers(before, []model.ServerImportItem{
		{Row: 2, Key: "9.9.9.9", Action: "create", Server: imported("9.9.9.9", "first")},
		{Row: 3, Key: "9.9.9.9", Action: "create", Server: imported("9.9.9.9", "duplicate")},
	}); err == nil {
		t.Fatal("duplicate creates accepted")
	}
	if snapshot(t, s) != before {
		t.Fatal("duplicate create failed to roll back")
	}
}
