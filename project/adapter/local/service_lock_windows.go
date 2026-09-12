//go:build windows

package local

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockProjectFile(file *os.File) error {
	overlapped := new(windows.Overlapped)
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		overlapped,
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrProjectServiceLocked
	}
	if err != nil {
		return fmt.Errorf("lock Project write service: %w", err)
	}
	return nil
}

func unlockProjectFile(file *os.File) error {
	overlapped := new(windows.Overlapped)
	if err := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped); err != nil {
		return fmt.Errorf("unlock Project write service: %w", err)
	}
	return nil
}
