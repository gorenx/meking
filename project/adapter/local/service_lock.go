package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const projectServiceLockFile = ".project-service.lock"

var ErrProjectServiceLocked = errors.New("Project already has an active write service")

// ProjectServiceLock is the process-held exclusive write-service lease for
// one local Project root. The empty lock file remains after release;
// exclusivity belongs to the open file handle and is released by the operating
// system if the process exits.
type ProjectServiceLock struct {
	mutex    sync.Mutex
	file     *os.File
	released bool
}

func AcquireProjectServiceLock(root string) (*ProjectServiceLock, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("acquire Project service lock: root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Project service lock root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("inspect Project service lock root: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("acquire Project service lock: root is not a directory")
	}
	file, err := openProjectServiceLockFile(
		filepath.Join(filepath.Clean(absolute), projectServiceLockFile),
	)
	if err != nil {
		return nil, fmt.Errorf("open Project service lock: %w", err)
	}
	if err := lockProjectFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	releaseOnFailure := true
	defer func() {
		if releaseOnFailure {
			_ = unlockProjectFile(file)
			_ = file.Close()
		}
	}()
	releaseOnFailure = false
	return &ProjectServiceLock{file: file}, nil
}

func openProjectServiceLockFile(path string) (*os.File, error) {
	for {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
			if errors.Is(createErr, os.ErrExist) {
				continue
			}
			if createErr != nil {
				return nil, fmt.Errorf("create Project service lock: %w", createErr)
			}
			return file, nil
		}
		if err != nil {
			return nil, fmt.Errorf("inspect Project service lock: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("open Project service lock: lock path is not a regular file")
		}
		file, openErr := os.OpenFile(path, os.O_RDWR, 0)
		if errors.Is(openErr, os.ErrNotExist) {
			continue
		}
		if openErr != nil {
			return nil, fmt.Errorf("open Project service lock: %w", openErr)
		}
		opened, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("inspect opened Project service lock: %w", statErr)
		}
		if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
			_ = file.Close()
			return nil, errors.New("open Project service lock: lock path changed while opening")
		}
		return file, nil
	}
}

func (lock *ProjectServiceLock) Close() error {
	if lock == nil {
		return nil
	}
	lock.mutex.Lock()
	defer lock.mutex.Unlock()
	if lock.released {
		return nil
	}
	lock.released = true
	file := lock.file
	lock.file = nil
	if file == nil {
		return nil
	}
	unlockErr := unlockProjectFile(file)
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}
