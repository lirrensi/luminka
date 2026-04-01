package luminka

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFSBridgeByteRoundTripOperations(t *testing.T) {
	root := t.TempDir()
	fsb := NewFSBridge(root)
	data := []byte{0x00, 0x01, 0x02, 0xff, 0x10}

	f, err := fsb.OpenWrite(filepath.Join("bytes", "payload.bin"))
	if err != nil {
		t.Fatalf("OpenWrite() error = %v", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		t.Fatalf("Write() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	readFile, size, err := fsb.OpenRead(filepath.Join("bytes", "payload.bin"))
	if err != nil {
		t.Fatalf("OpenRead() error = %v", err)
	}
	defer readFile.Close()
	if size != int64(len(data)) {
		t.Fatalf("OpenRead size = %d, want %d", size, len(data))
	}
	readData, err := io.ReadAll(readFile)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !reflect.DeepEqual(readData, data) {
		t.Fatalf("OpenRead data = %#v, want %#v", readData, data)
	}
}

func TestFSBridgeRoundTripOperations(t *testing.T) {
	root := t.TempDir()
	fsb := NewFSBridge(root)

	if err := fsb.Write(filepath.Join("notes", "todo.txt"), "hello\nworld"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	exists, err := fsb.Exists(filepath.Join("notes", "todo.txt"))
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Fatal("Exists() = false, want true")
	}

	data, err := fsb.Read(filepath.Join("notes", "todo.txt"))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if data != "hello\nworld" {
		t.Fatalf("Read() = %q, want %q", data, "hello\nworld")
	}

	files, err := fsb.List("notes")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !reflect.DeepEqual(files, []string{"todo.txt"}) {
		t.Fatalf("List() = %#v, want %#v", files, []string{"todo.txt"})
	}

	if err := fsb.Delete(filepath.Join("notes", "todo.txt")); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	exists, err = fsb.Exists(filepath.Join("notes", "todo.txt"))
	if err != nil {
		t.Fatalf("Exists() after delete error = %v", err)
	}
	if exists {
		t.Fatal("Exists() after delete = true, want false")
	}
}

func TestNormalizeRelativePathRejectsEscapes(t *testing.T) {
	absolutePath, err := filepath.Abs("tmp")
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "absolute", path: absolutePath},
		{name: "parent traversal", path: filepath.Join("..", "secret.txt")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := normalizeRelativePath(tc.path); err == nil {
				t.Fatalf("normalizeRelativePath(%q) succeeded, want error", tc.path)
			}
		})
	}
}

func TestResolvePathWithinRootRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	linkPath := filepath.Join(root, "escape-link")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Skipf("symlink setup unavailable on this platform: %v", err)
	}

	if _, err := resolvePathWithinRoot(root, filepath.Join("escape-link", "outside.txt")); err == nil {
		t.Fatal("resolvePathWithinRoot() succeeded through escaping symlink, want error")
	}
}

func TestFSBridgeRejectsDirectoryDelete(t *testing.T) {
	root := t.TempDir()
	fsb := NewFSBridge(root)
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := fsb.Delete("notes"); err == nil {
		t.Fatal("Delete() on directory succeeded, want error")
	}
}
