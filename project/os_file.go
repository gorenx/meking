package project

import (
	"io/fs"
	"os"
	"path/filepath"
)

func writeProjectFile(path string, content []byte, mode fs.FileMode, overwrite bool) (bool, error) {
	if !overwrite {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return false, err
		}
		if err := writeAndSync(file, content); err != nil {
			_ = os.Remove(path)
			return false, err
		}
		return true, nil
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return false, err
	}
	if err := writeAndSync(temporary, content); err != nil {
		return false, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return false, err
	}
	return false, nil
}

func writeAndSync(file *os.File, content []byte) error {
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
