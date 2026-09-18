//go:build windows

package winservice

import (
	"path/filepath"
	"testing"
)

func TestSingleInstanceLockReleasedOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	first, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AcquireLock(path)
	if err == nil {
		second.Close()
		first.Close()
		t.Fatal("same data directory accepted two instances")
	}
	first.Close()
	third, err := AcquireLock(path)
	if err != nil {
		t.Fatal("lock did not release", err)
	}
	third.Close()
}
