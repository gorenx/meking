//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package local

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockProjectFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrProjectServiceLocked
	}
	if err != nil {
		return fmt.Errorf("lock Project write service: %w", err)
	}
	return nil
}

func unlockProjectFile(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
		return fmt.Errorf("unlock Project write service: %w", err)
	}
	return nil
}
