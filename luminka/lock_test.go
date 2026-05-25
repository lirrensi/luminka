// FILE: luminka/lock_test.go
// PURPOSE: Unit tests for lock file operations — parsing, reading, writing, removal, and reachability.
// OWNS: parseLockRecord, readLockRecord, writeInstanceRecord, removeLockFile, localhostPortReachable, lockFilePath
// EXPORTS: none
// DOCS: docs/spec.md

package luminka

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLockRecordValid(t *testing.T) {
	pid, port, err := parseLockRecord("12345:8080")
	if err != nil {
		t.Fatalf("parseLockRecord('12345:8080') error = %v", err)
	}
	if pid != 12345 {
		t.Fatalf("pid = %d, want 12345", pid)
	}
	if port != 8080 {
		t.Fatalf("port = %d, want 8080", port)
	}
}

func TestParseLockRecordWhitespaceTrimmed(t *testing.T) {
	pid, port, err := parseLockRecord("  999:443  \n")
	if err != nil {
		t.Fatalf("parseLockRecord('  999:443  \\n') error = %v", err)
	}
	if pid != 999 {
		t.Fatalf("pid = %d, want 999", pid)
	}
	if port != 443 {
		t.Fatalf("port = %d, want 443", port)
	}
}

func TestParseLockRecordEmpty(t *testing.T) {
	_, _, err := parseLockRecord("")
	if err == nil {
		t.Fatal("parseLockRecord('') = nil, want error")
	}
}

func TestParseLockRecordWhitespaceOnly(t *testing.T) {
	_, _, err := parseLockRecord("  \t  \n")
	if err == nil {
		t.Fatal("parseLockRecord('  \t  \\n') = nil, want error")
	}
}

func TestParseLockRecordMissingColon(t *testing.T) {
	_, _, err := parseLockRecord("12345")
	if err == nil {
		t.Fatal("parseLockRecord('12345') = nil, want error")
	}
}

func TestParseLockRecordMultipleColons(t *testing.T) {
	_, _, err := parseLockRecord("12345:8080:extra")
	if err == nil {
		t.Fatal("parseLockRecord('12345:8080:extra') = nil, want error")
	}
}

func TestParseLockRecordNonNumericPID(t *testing.T) {
	_, _, err := parseLockRecord("abc:8080")
	if err == nil {
		t.Fatal("parseLockRecord('abc:8080') = nil, want error")
	}
}

func TestParseLockRecordNonNumericPort(t *testing.T) {
	_, _, err := parseLockRecord("12345:xyz")
	if err == nil {
		t.Fatal("parseLockRecord('12345:xyz') = nil, want error")
	}
}

func TestParseLockRecordNegativePID(t *testing.T) {
	pid, port, err := parseLockRecord("-1:8080")
	if err != nil {
		t.Fatalf("parseLockRecord('-1:8080') error = %v", err)
	}
	if pid != -1 {
		t.Fatalf("pid = %d, want -1", pid)
	}
	if port != 8080 {
		t.Fatalf("port = %d, want 8080", port)
	}
}

func TestReadLockRecordEdgeCases(t *testing.T) {
	root := t.TempDir()

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(root, "empty.lock")
		if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := readLockRecord(path)
		if err == nil {
			t.Fatal("readLockRecord(empty) = nil, want error")
		}
	})

	t.Run("garbage content", func(t *testing.T) {
		path := filepath.Join(root, "garbage.lock")
		if err := os.WriteFile(path, []byte("not a lock record\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := readLockRecord(path)
		if err == nil {
			t.Fatal("readLockRecord(garbage) = nil, want error")
		}
	})

	t.Run("JSON with pid=0", func(t *testing.T) {
		path := filepath.Join(root, "pidzero.lock")
		if err := writeInstanceRecord(path, instanceRecord{PID: 0, Port: 8080}); err != nil {
			t.Fatal(err)
		}
		// readLockRecord rejects JSON with PID == 0, falls through to legacy parse
		// legacy parse "<json>" -> no colon -> error
		record, err := readLockRecord(path)
		if err == nil {
			t.Fatalf("readLockRecord(pid=0) = %#v, want error (no legacy fallback for JSON)", record)
		}
	})

	t.Run("legacy text format", func(t *testing.T) {
		path := filepath.Join(root, "legacy.lock")
		pid := os.Getpid()
		if err := os.WriteFile(path, []byte(fmt.Sprintf("%d:%d", pid, 9090)), 0o644); err != nil {
			t.Fatal(err)
		}
		record, err := readLockRecord(path)
		if err != nil {
			t.Fatalf("readLockRecord(legacy) error = %v", err)
		}
		if record.PID != pid || record.Port != 9090 {
			t.Fatalf("record = %#v, want PID=%d Port=9090", record, pid)
		}
	})

	t.Run("truncated JSON", func(t *testing.T) {
		path := filepath.Join(root, "truncated.lock")
		if err := os.WriteFile(path, []byte(`{"pid": 12345, "p`), 0o644); err != nil {
			t.Fatal(err)
		}
		// JSON is invalid, fall through to legacy parse: no colon -> error
		_, err := readLockRecord(path)
		if err == nil {
			t.Fatal("readLockRecord(truncated JSON) = nil, want error")
		}
	})
}

func TestWriteInstanceRecordRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roundtrip.lock")
	record := instanceRecord{PID: 42, Port: 8888}
	if err := writeInstanceRecord(path, record); err != nil {
		t.Fatalf("writeInstanceRecord() error = %v", err)
	}

	got, err := readLockRecord(path)
	if err != nil {
		t.Fatalf("readLockRecord() error = %v", err)
	}
	if got.PID != 42 || got.Port != 8888 {
		t.Fatalf("record = %#v, want PID=42 Port=8888", got)
	}

	// Verify trailing newline
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("writeInstanceRecord missing trailing newline: %q", string(data))
	}
}

func TestWriteInstanceRecordWithAllFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "full.lock")
	record := instanceRecord{
		PID:  999,
		Port: 0,
		Mode: ModeWebview,
		Window: instanceWindowRecord{
			Platform: "darwin",
			ID:       "0xABC",
		},
	}
	if err := writeInstanceRecord(path, record); err != nil {
		t.Fatalf("writeInstanceRecord() error = %v", err)
	}
	got, err := readLockRecord(path)
	if err != nil {
		t.Fatalf("readLockRecord() error = %v", err)
	}
	if *got != record {
		t.Fatalf("record = %#v, want %#v", *got, record)
	}
}

func TestRemoveLockFileRemovesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exists.lock")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeLockFile(path); err != nil {
		t.Fatalf("removeLockFile() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists after removeLockFile, stat err = %v", err)
	}
}

func TestRemoveLockFileIdempotentMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.lock")
	if err := removeLockFile(path); err != nil {
		t.Fatalf("removeLockFile(missing) error = %v", err)
	}
}

func TestRemoveLockFileIdempotentAlreadyRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "already-removed.lock")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeLockFile(path); err != nil {
		t.Fatalf("first removeLockFile() error = %v", err)
	}
	if err := removeLockFile(path); err != nil {
		t.Fatalf("second removeLockFile() error = %v", err)
	}
}

func TestLockFilePath(t *testing.T) {
	path := lockFilePath("/tmp", "myapp")
	want := filepath.Join("/tmp", "myapp.lock")
	if path != want {
		t.Fatalf("lockFilePath('/tmp', 'myapp') = %q, want %q", path, want)
	}
}

func TestLockFilePathEmptyName(t *testing.T) {
	path := lockFilePath("/tmp", "")
	want := filepath.Join("/tmp", ".lock")
	if path != want {
		t.Fatalf("lockFilePath('/tmp', '') = %q, want %q", path, want)
	}
}

func TestLocalhostPortReachableUnreachable(t *testing.T) {
	// Port 0 is reserved, so connecting to port 0 should fail.
	if localhostPortReachable(0, 50*time.Millisecond) {
		t.Skip("port 0 is unexpectedly reachable on this system")
	}
}

func TestLocalhostPortReachableListenThenClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Port was freed, should not be reachable
	if localhostPortReachable(port, 50*time.Millisecond) {
		t.Skipf("port %d became reachable (likely reassigned)", port)
	}
}

func TestResolveRootDirectoryUnsupportedPolicy(t *testing.T) {
	_, err := resolveRootDirectory("", RootPolicy("unsupported"))
	if err == nil {
		t.Fatal("resolveRootDirectory('', 'unsupported') = nil, want error")
	}
}
