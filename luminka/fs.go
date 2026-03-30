// FILE: luminka/fs.go
// PURPOSE: Provide root-safe text filesystem operations for the capability layer.
// OWNS: Relative path sanitization, text file I/O, directory listing, deletion, and existence checks.
// EXPORTS: FSBridge, NewFSBridge
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FSBridge struct {
	root string
}

func NewFSBridge(root string) *FSBridge {
	resolved := root
	if abs, err := filepath.Abs(root); err == nil {
		resolved = abs
	}
	if eval, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = eval
	}
	return &FSBridge{root: resolved}
}

func (fsb *FSBridge) sanitize(path string) (string, error) {
	if fsb == nil {
		return "", errors.New("filesystem bridge is required")
	}
	_, resolved, err := resolveRelativePath(fsb.root, path)
	return resolved, err
}

func (fsb *FSBridge) Read(path string) (string, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (fsb *FSBridge) Write(path string, data string) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	return os.WriteFile(resolved, []byte(data), 0o644)
}

func (fsb *FSBridge) List(path string) ([]string, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		out = append(out, name)
	}
	return out, nil
}

func (fsb *FSBridge) Delete(path string) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("directories cannot be deleted by fs_delete")
	}
	return os.Remove(resolved)
}

func (fsb *FSBridge) Exists(path string) (bool, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(resolved)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func normalizeRelativePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", errors.New("absolute paths are not allowed")
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes root")
	}
	return clean, nil
}

func resolveRelativePath(root, path string) (string, string, error) {
	rel, err := normalizeRelativePath(path)
	if err != nil {
		return "", "", err
	}
	resolved, err := resolvePathWithinRoot(root, rel)
	if err != nil {
		return "", "", err
	}
	return rel, resolved, nil
}

func resolvePathWithinRoot(root, rel string) (string, error) {
	if root == "" {
		return "", errors.New("filesystem root is required")
	}
	root = filepath.Clean(root)
	if rel == "" {
		return ensureWithinRoot(root, root)
	}

	target := filepath.Join(root, rel)
	suffix := ""
	for {
		_, statErr := os.Lstat(target)
		if statErr == nil {
			resolved, err := filepath.EvalSymlinks(target)
			if err != nil {
				return "", err
			}
			if suffix != "" {
				resolved = filepath.Join(resolved, suffix)
			}
			return ensureWithinRoot(root, resolved)
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(target)
		if parent == target {
			return "", errors.New("path escapes root")
		}
		base := filepath.Base(target)
		if suffix == "" {
			suffix = base
		} else {
			suffix = filepath.Join(base, suffix)
		}
		target = parent
	}
}

func ensureWithinRoot(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes root")
	}
	return path, nil
}

func currentPathModTime(root, path string) (time.Time, bool, error) {
	resolved, err := resolvePathWithinRoot(root, path)
	if err != nil {
		return time.Time{}, false, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	return info.ModTime(), true, nil
}
