package store

import (
	"fmt"
	"testing"
	"time"

	"dnsmonitor/internal/model"
)

func TestExportReleasesConnectionAndHasInsertionSnapshot(t *testing.T) {
	s, v := testStore(t)
	at := time.Now().UnixMilli()
	mustSave(t, s, round(v.ID, at, 1000, true, true, 201, 10, "clean"))
	done := make(chan error, 1)
	go func() {
		count := 0
		err := s.WalkResults(v.ID, at, func(_ model.ProbeResult) error {
			count++
			if count == 1 {
				if _, err := s.GetConfig(); err != nil {
					return err
				}
				// A concurrent insert with the same timestamp must not appear on page two.
				return s.SaveRound(round(v.ID, at, 1000, true, true, 1, 20, "clean"))
			}
			return nil
		})
		if err == nil && count != 201 {
			err = fmt.Errorf("export snapshot wanted 201 rows, got %d", count)
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		// Release a broken implementation before cleanup, so this regression never hangs the suite.
		s.db.SetMaxOpenConns(2)
		<-done
		t.Fatal("export callback could not access database; rows held the sole connection")
	}
}
