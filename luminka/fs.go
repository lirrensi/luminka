// FILE: luminka/fs.go
// PURPOSE: Provide root-safe text filesystem operations for the capability layer.
// OWNS: Relative path sanitization, text file I/O, directory listing, deletion, and existence checks.
// EXPORTS: FSBridge, NewFSBridge
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	// If symlink resolution leads outside the original path, the bridge still uses
	// the resolved path (it is the canonical location). The sanitize() check prevents
	// path traversal at each operation. Log a warning if the resolved path escapes.
	absOriginal, absErr := filepath.Abs(root)
	if absErr == nil {
		absResolved, _ := filepath.Abs(resolved)
		if !strings.HasPrefix(absResolved, absOriginal) && absResolved != absOriginal {
			fmt.Fprintf(os.Stderr, "[luminka] warning: configured root resolves outside original path: %s → %s\n", root, resolved)
		}
	}
	return &FSBridge{root: resolved}
}

func (fsb *FSBridge) ReadBytes(path string) ([]byte, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func (fsb *FSBridge) WriteBytes(path string, data []byte) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	return os.WriteFile(resolved, data, 0o644)
}

func (fsb *FSBridge) OpenRead(path string) (*os.File, int64, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return nil, 0, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, 0, errors.New("directories cannot be opened for reading")
	}
	return file, info.Size(), nil
}

func (fsb *FSBridge) OpenWrite(path string) (*os.File, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(resolved, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
}

func (fsb *FSBridge) sanitize(path string) (string, error) {
	if fsb == nil {
		return "", errors.New("filesystem bridge is required")
	}
	_, resolved, err := resolveRelativePath(fsb.root, path)
	return resolved, err
}

func (fsb *FSBridge) Read(path string) (string, error) {
	data, err := fsb.ReadBytes(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (fsb *FSBridge) Write(path string, data string) error {
	return fsb.WriteBytes(path, []byte(data))
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
	// Check if the path is a symlink before sanitize resolves it.
	// Sanitize uses EvalSymlinks which follows symlinks to their target,
	// making it impossible to detect them afterward.
	cleanPath, normErr := normalizeRelativePath(path)
	if normErr != nil {
		return normErr
	}
	preResolved := filepath.Join(fsb.root, cleanPath)
	if lstatInfo, lerr := os.Lstat(preResolved); lerr == nil && lstatInfo.Mode()&os.ModeSymlink != 0 {
		// Symlinks are file-like entries regardless of their target
		return os.Remove(preResolved)
	}

	// Not a symlink — use normal sanitize + directory check
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

// --- Directory operations ---

func (fsb *FSBridge) Mkdir(path string, perm os.FileMode) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	return os.Mkdir(resolved, perm)
}

func (fsb *FSBridge) MkdirAll(path string, perm os.FileMode) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(resolved, perm)
}

func (fsb *FSBridge) ReadDir(path string) ([]os.DirEntry, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(resolved)
}

func (fsb *FSBridge) Rmdir(path string) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	return os.Remove(resolved)
}

func (fsb *FSBridge) Mkdtemp(prefix string) (string, error) {
	resolved, err := fsb.sanitize(prefix)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(resolved)
	base := filepath.Base(resolved)
	return os.MkdirTemp(dir, base)
}

// --- Copy, move, delete ---

func (fsb *FSBridge) Rename(oldPath, newPath string) error {
	oldResolved, err := fsb.sanitize(oldPath)
	if err != nil {
		return err
	}
	newResolved, err := fsb.sanitize(newPath)
	if err != nil {
		return err
	}
	return os.Rename(oldResolved, newResolved)
}

func (fsb *FSBridge) CopyFile(src, dst string) error {
	srcResolved, err := fsb.sanitize(src)
	if err != nil {
		return err
	}
	dstResolved, err := fsb.sanitize(dst)
	if err != nil {
		return err
	}
	if srcResolved == dstResolved {
		return nil // no-op: source and destination are the same file
	}
	return copyFileContent(srcResolved, dstResolved)
}

func copyFileContent(srcResolved, dstResolved string) error {
	srcFile, err := os.Open(srcResolved)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	if err := os.MkdirAll(filepath.Dir(dstResolved), 0o755); err != nil {
		return err
	}
	dstFile, err := os.OpenFile(dstResolved, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (fsb *FSBridge) Cp(src, dst string, recursive bool) error {
	srcResolved, err := fsb.sanitize(src)
	if err != nil {
		return err
	}
	dstResolved, err := fsb.sanitize(dst)
	if err != nil {
		return err
	}

	// Circular copy guard: prevent copying a directory into itself
	if strings.HasPrefix(dstResolved+string(filepath.Separator), srcResolved+string(filepath.Separator)) ||
		dstResolved == srcResolved {
		return fmt.Errorf("cannot copy %q to %q: destination is inside source", src, dst)
	}

	srcInfo, err := os.Stat(srcResolved)
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		if !recursive {
			return fmt.Errorf("cannot copy directory %q without recursive flag", src)
		}
		return filepath.WalkDir(srcResolved, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, relErr := filepath.Rel(srcResolved, path)
			if relErr != nil {
				return relErr
			}
			target := filepath.Join(dstResolved, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			// Preserve symlinks instead of following them (security: no data exfiltration)
			if d.Type()&os.ModeSymlink != 0 {
				linkTarget, linkErr := os.Readlink(path)
				if linkErr != nil {
					return linkErr
				}
				return os.Symlink(linkTarget, target)
			}
			if err := copyFileContent(path, target); err != nil {
				// Clean up partially-written destination file on failure
				os.Remove(target)
				return fmt.Errorf("copy %s → %s: %w", path, target, err)
			}
			return nil
		})
	}

	if err := os.MkdirAll(filepath.Dir(dstResolved), 0o755); err != nil {
		return err
	}
	if err := copyFileContent(srcResolved, dstResolved); err != nil {
		// Clean up partially-written destination file on failure
		os.Remove(dstResolved)
		return fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}
	return nil
}

func (fsb *FSBridge) Remove(path string) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	return os.Remove(resolved)
}

func (fsb *FSBridge) RemoveAll(path string) error {
	// Check for symlinks before sanitize resolves them.
	// Sanitize uses EvalSymlinks which follows symlinks, causing path-escape
	// errors for symlinks pointing outside root.
	cleanPath, normErr := normalizeRelativePath(path)
	if normErr != nil {
		return normErr
	}
	preResolved := filepath.Join(fsb.root, cleanPath)
	if lstatInfo, lerr := os.Lstat(preResolved); lerr == nil && lstatInfo.Mode()&os.ModeSymlink != 0 {
		// Remove the symlink entry itself, not the target
		return os.Remove(preResolved)
	}
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(resolved)
}

// --- Metadata ---

func (fsb *FSBridge) Stat(path string) (os.FileInfo, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return nil, err
	}
	return os.Stat(resolved)
}

func (fsb *FSBridge) Lstat(path string) (os.FileInfo, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return nil, err
	}
	return os.Lstat(resolved)
}

func (fsb *FSBridge) Access(path string, mode os.FileMode) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	_, err = os.Stat(resolved)
	return err
}

// --- Mutation ---

func (fsb *FSBridge) Chmod(path string, mode os.FileMode) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	return os.Chmod(resolved, mode)
}

func (fsb *FSBridge) Utimes(path string, atime, mtime time.Time) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	return os.Chtimes(resolved, atime, mtime)
}

func (fsb *FSBridge) Truncate(path string, size int64) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	return os.Truncate(resolved, size)
}

func (fsb *FSBridge) Symlink(target, linkPath string) error {
	// The symlink target is stored as-is (may be relative or absolute).
	// Only the link path must be within the app root.
	linkResolved, err := fsb.sanitize(linkPath)
	if err != nil {
		return err
	}
	return os.Symlink(target, linkResolved)
}

func (fsb *FSBridge) Readlink(path string) (string, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return "", err
	}
	return os.Readlink(resolved)
}

func (fsb *FSBridge) Realpath(path string) (string, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(resolved)
}

func (fsb *FSBridge) Link(oldPath, newPath string) error {
	oldResolved, err := fsb.sanitize(oldPath)
	if err != nil {
		return err
	}
	newResolved, err := fsb.sanitize(newPath)
	if err != nil {
		return err
	}
	return os.Link(oldResolved, newResolved)
}

func (fsb *FSBridge) AppendFile(path string, data []byte) error {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(resolved, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// --- Open handle ---

func (fsb *FSBridge) Open(path string, flags int, perm os.FileMode) (*os.File, error) {
	resolved, err := fsb.sanitize(path)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(resolved, flags, perm)
}

// normalizeRelativePath converts a relative path to a clean internal form.
// Convention: "." becomes "" (empty string = root directory).
// Callers must handle the empty-string case when joining with the root path.
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
