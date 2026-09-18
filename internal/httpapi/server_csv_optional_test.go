package httpapi

import (
	"dnsmonitor/internal/model"
	"encoding/json"
	"testing"
)

func TestCSVOverwritePreservesOmittedColumns(t *testing.T) {
	s, h, key := testAPI(t)
	token := loginToken(t, h, key)
	original, e := s.Store.SaveServer(model.Server{Name: "Before", Provider: "Preserve supplier", Address: "udp://1.1.1.1", Protocol: "UDP", Enabled: false, Trusted: true, Notes: "Preserve notes"})
	if e != nil {
		t.Fatal(e)
	}
	source := "name,address\nRenamed,1.1.1.1:53\n"
	preview := csvPreview(t, h, token, source)
	if preview.Rows[0].Server.Provider != original.Provider || !preview.Rows[0].Server.Trusted || preview.Rows[0].Server.Enabled {
		t.Fatal("preview lost omitted fields", preview.Rows[0])
	}
	body, _ := json.Marshal(importRequest{CSV: source, Snapshot: preview.Snapshot, Decisions: []importDecision{{Row: 1, Action: "overwrite"}}})
	if res := call(h, "POST", "/api/servers/import", string(body), token); res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	updated, e := s.Store.GetServer(original.ID)
	if e != nil || updated.Name != "Renamed" || updated.Provider != original.Provider || updated.Notes != original.Notes || updated.Enabled || !updated.Trusted {
		t.Fatal(updated, e)
	}
}
