package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OSProjectMaterializer applies Project plans to the local filesystem.
type OSProjectMaterializer struct{}

// NewOSProjectMaterializer constructs a local filesystem materializer.
func NewOSProjectMaterializer() OSProjectMaterializer {
	return OSProjectMaterializer{}
}

func (OSProjectMaterializer) Materialize(ctx context.Context, spec ProjectSpec) (ProjectResult, error) {
	root, err := filepath.Abs(spec.Root)
	if err != nil {
		return ProjectResult{}, fmt.Errorf("resolve project root: %w", err)
	}
	root = filepath.Clean(root)
	if err := ensureRootDirectory(root); err != nil {
		return ProjectResult{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return ProjectResult{}, fmt.Errorf("resolve project symlinks: %w", err)
	}

	marker, err := projectPath(root, spec.Marker)
	if err != nil {
		return ProjectResult{}, fmt.Errorf("resolve marker: %w", err)
	}
	if !spec.Force {
		if _, err := os.Lstat(marker); err == nil {
			return ProjectResult{}, fmt.Errorf("%w at %s", ErrProjectAlreadyInitialized, root)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return ProjectResult{}, fmt.Errorf("inspect project marker: %w", err)
		}
	}

	result := ProjectResult{Root: root}
	directories := append([]string(nil), spec.Directories...)
	sort.Strings(directories)
	for _, relative := range directories {
		if err := ctx.Err(); err != nil {
			return ProjectResult{}, err
		}
		if err := ensureProjectDirectory(root, relative); err != nil {
			return ProjectResult{}, fmt.Errorf("create directory %q: %w", relative, err)
		}
	}

	for _, file := range spec.Files {
		if err := ctx.Err(); err != nil {
			return ProjectResult{}, err
		}
		path, err := projectPath(root, file.Path)
		if err != nil {
			return ProjectResult{}, fmt.Errorf("resolve file %q: %w", file.Path, err)
		}
		if err := ensureProjectDirectory(root, filepath.Dir(file.Path)); err != nil {
			return ProjectResult{}, fmt.Errorf("create parent for %q: %w", file.Path, err)
		}

		existed, err := pathExists(path)
		if err != nil {
			return ProjectResult{}, fmt.Errorf("inspect file %q: %w", file.Path, err)
		}
		if existed && (!spec.Force || file.Preserve) {
			result.Preserved = append(result.Preserved, file.Path)
			continue
		}

		created, err := writeProjectFile(
			path,
			file.Content,
			fs.FileMode(file.Mode),
			spec.Force && !file.Preserve,
		)
		if errors.Is(err, fs.ErrExist) && file.Path == spec.Marker {
			return ProjectResult{}, fmt.Errorf("%w at %s", ErrProjectAlreadyInitialized, root)
		}
		if errors.Is(err, fs.ErrExist) {
			result.Preserved = append(result.Preserved, file.Path)
			continue
		}
		if err != nil {
			return ProjectResult{}, fmt.Errorf("write file %q: %w", file.Path, err)
		}
		if existed && !created {
			result.Overwritten = append(result.Overwritten, file.Path)
		} else {
			result.Created = append(result.Created, file.Path)
		}
	}

	sort.Strings(result.Created)
	sort.Strings(result.Overwritten)
	sort.Strings(result.Preserved)
	return result, nil
}

func ensureRootDirectory(root string) error {
	info, err := os.Stat(root)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("project root is not a directory: %s", root)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect project root: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create project root: %w", err)
	}
	return nil
}

func ensureProjectDirectory(root, relative string) error {
	if relative == "." {
		return nil
	}
	resolved, err := projectPath(root, relative)
	if err != nil {
		return err
	}
	within, err := filepath.Rel(root, resolved)
	if err != nil {
		return err
	}
	current := root
	for _, component := range strings.Split(within, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err == nil {
				continue
			} else if !errors.Is(err, fs.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
			if err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("project directory must not be a symbolic link: %s", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("project path is not a directory: %s", current)
		}
	}
	return nil
}

func projectPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("path must be a non-empty relative path")
	}
	cleaned := filepath.Clean(relative)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes project root")
	}
	resolved := filepath.Join(root, cleaned)
	within, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes project root")
	}
	return resolved, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
