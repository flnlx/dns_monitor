//go:build windows

package winservice

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Hold the lock for the entire process lifetime. The lock file can remain on
// disk: Windows releases the byte-range lock even after an unexpected exit.
func AcquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	var overlapped windows.Overlapped
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("此数据目录已有运行中的 DNS Monitor（可能是 Windows 服务）: %w", err)
	}
	return f, nil
}
