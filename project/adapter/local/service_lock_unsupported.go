//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package local

import (
	"errors"
	"os"
)

func lockProjectFile(*os.File) error {
	return errors.New("Project service locking is not supported on this operating system")
}

func unlockProjectFile(*os.File) error { return nil }
