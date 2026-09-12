package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func existingDirectory(path, label string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%s root is required", label)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s root: %w", label, err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect %s root: %w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s root is not a directory", label)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve %s root links: %w", label, err)
	}
	return resolved, nil
}

func readRegularProjectFile(root, relative string) ([]byte, error) {
	path, err := projectPath(root, relative)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("project file %q must be a regular file", relative)
	}
	return os.ReadFile(path)
}

func readProjectText(root, relative string) (string, error) {
	path, err := projectPath(root, relative)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
