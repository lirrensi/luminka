// FILE: luminka/watch_test.go
// PURPOSE: Unit tests for the fsnotify-based Watcher with polling fallback.
// OWNS: NewWatcher defaults, Add/Remove lifecycle, Start/Stop, pollOnce, handleFSEvent, nil receiver safety
// EXPORTS: none
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestNewWatcherDefaults(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 0, nil)
	if w == nil {
		t.Fatal("NewWatcher() = nil")
	}
	if w.interval != time.Second {
		t.Fatalf("interval = %v, want 1s", w.interval)
	}
	if w.root != root {
		t.Fatalf("root = %q, want %q", w.root, root)
	}
	if w.notify != nil {
		t.Fatal("notify = non-nil, want nil")
	}
	if len(w.watched) != 0 {
		t.Fatalf("watched = %v, want empty", w.watched)
	}
}

func TestNewWatcherResolvesRoot(t *testing.T) {
	root := t.TempDir()
	// Create a symlink to test root resolution
	linkPath := filepath.Join(t.TempDir(), "link-to-root")
	if err := os.Symlink(root, linkPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	w := NewWatcher(linkPath, 0, nil)
	if w.root != root {
		t.Fatalf("resolved root = %q, want %q", w.root, root)
	}
}

func TestWatcherAddRemove(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 0, nil)

	if err := w.Add("test.txt"); err != nil {
		t.Fatalf("Add('test.txt') error = %v", err)
	}
	if _, ok := w.watched["test.txt"]; !ok {
		t.Fatal("watched map missing 'test.txt' after Add")
	}

	// Add the same path again — should be idempotent
	if err := w.Add("test.txt"); err != nil {
		t.Fatalf("Add('test.txt') duplicate error = %v", err)
	}

	if err := w.Remove("test.txt"); err != nil {
		t.Fatalf("Remove('test.txt') error = %v", err)
	}
	if _, ok := w.watched["test.txt"]; ok {
		t.Fatal("watched map still contains 'test.txt' after Remove")
	}

	// Remove again — should be idempotent
	if err := w.Remove("test.txt"); err != nil {
		t.Fatalf("Remove('test.txt') duplicate error = %v", err)
	}
}

func TestWatcherAddNonExistentPath(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 0, nil)
	// currentPathModTime returns nil error for non-existent paths (returns zero time),
	// so Add succeeds — it registers the path and will detect creation later via poll.
	err := w.Add("nonexistent/file.txt")
	if err != nil {
		t.Fatalf("Add('nonexistent/file.txt') error = %v, want nil (will detect creation)", err)
	}
	// On Windows, normalizeRelativePath uses filepath.Clean which converts / to \.
	want := filepath.FromSlash("nonexistent/file.txt")
	if _, ok := w.watched[want]; !ok {
		t.Fatalf("watched map missing %q after Add, want length > 0", want)
	}
}

func TestWatcherPollOnceDetectsFileCreation(t *testing.T) {
	root := t.TempDir()
	notified := make(chan string, 1)
	w := NewWatcher(root, 0, func(path string) error {
		select {
		case notified <- path:
		default:
		}
		return nil
	})

	if err := w.Add("newfile.txt"); err != nil {
		t.Fatalf("Add('newfile.txt') error = %v", err)
	}

	// Create the file after adding it
	path := filepath.Join(root, "newfile.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// pollOnce should detect the change
	w.pollOnce()

	select {
	case p := <-notified:
		if p != "newfile.txt" {
			t.Fatalf("notified path = %q, want 'newfile.txt'", p)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for pollOnce notification")
	}
}

func TestWatcherPollOnceDetectsFileModification(t *testing.T) {
	root := t.TempDir()
	notified := make(chan string, 1)
	w := NewWatcher(root, 0, func(path string) error {
		select {
		case notified <- path:
		default:
		}
		return nil
	})

	path := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.Add("existing.txt"); err != nil {
		t.Fatalf("Add('existing.txt') error = %v", err)
	}

	// First poll — no change expected (modTime captured at Add time)
	w.pollOnce()
	select {
	case <-notified:
		t.Fatal("unexpected notification on first poll (no change)")
	case <-time.After(50 * time.Millisecond):
	}

	// Modify the file
	if err := os.WriteFile(path, []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for filesystem mod time granularity
	time.Sleep(10 * time.Millisecond)

	w.pollOnce()
	select {
	case p := <-notified:
		if p != "existing.txt" {
			t.Fatalf("notified path = %q, want 'existing.txt'", p)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for pollOnce notification after modification")
	}
}

func TestWatcherPollOnceIdempotentNoChange(t *testing.T) {
	root := t.TempDir()
	notificationCount := make(chan struct{}, 10)
	w := NewWatcher(root, 0, func(path string) error {
		notificationCount <- struct{}{}
		return nil
	})

	path := filepath.Join(root, "stable.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.Add("stable.txt"); err != nil {
		t.Fatal(err)
	}

	// First poll — no change expected (Add captured current modTime)
	w.pollOnce()
	select {
	case <-notificationCount:
		t.Fatal("pollOnce notified on unchanged file (first poll)")
	case <-time.After(50 * time.Millisecond):
	}

	// Second poll — still no change
	w.pollOnce()
	select {
	case <-notificationCount:
		t.Fatal("pollOnce notified on unchanged file (second poll)")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWatcherPollOnceIgnoresRemovedPath(t *testing.T) {
	root := t.TempDir()
	notificationCount := make(chan struct{}, 10)
	w := NewWatcher(root, 0, func(path string) error {
		notificationCount <- struct{}{}
		return nil
	})

	path := filepath.Join(root, "temp.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.Add("temp.txt"); err != nil {
		t.Fatal(err)
	}

	// First poll — no change expected (Add captured current modTime, file exists before Add)
	w.pollOnce()
	select {
	case <-notificationCount:
		t.Fatal("pollOnce notified on unchanged file")
	case <-time.After(50 * time.Millisecond):
	}

	// Remove the path from the watched set, then modify the file
	w.Remove("temp.txt")
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Poll — should NOT notify because the path was removed
	w.pollOnce()
	select {
	case <-notificationCount:
		t.Fatal("pollOnce notified on removed path")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWatcherStartStopBasic(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 50*time.Millisecond, func(path string) error { return nil })

	w.Start()
	if !w.running {
		t.Fatal("running = false after Start")
	}

	w.Stop()
	if !w.stopped {
		t.Fatal("stopped = false after Stop")
	}
	if w.running {
		t.Fatal("running = true after Stop")
	}

	// Verify doneCh is closed
	select {
	case <-w.doneCh:
		// OK
	case <-time.After(time.Second):
		t.Fatal("doneCh not closed after Stop")
	}
}

func TestWatcherStartTwoCallsIdempotent(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 50*time.Millisecond, nil)
	w.Start()
	w.Start() // second call should be no-op
	w.Stop()
}

func TestWatcherStopTwoCallsIdempotent(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 50*time.Millisecond, nil)
	w.Start()
	w.Stop()
	w.Stop() // second call should be no-op
}

func TestWatcherStopBeforeStart(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 50*time.Millisecond, nil)
	w.Stop() // should not deadlock or panic

	if !w.stopped {
		t.Fatal("stopped = false after Stop without Start")
	}

	// doneCh should still be closed
	select {
	case <-w.doneCh:
	case <-time.After(time.Second):
		t.Fatal("doneCh not closed after Stop without Start")
	}
}

func TestWatcherStartStopWithFileChange(t *testing.T) {
	root := t.TempDir()
	notified := make(chan string, 1)
	w := NewWatcher(root, 50*time.Millisecond, func(path string) error {
		notified <- path
		return nil
	})

	path := filepath.Join(root, "watch-me.txt")
	if err := os.WriteFile(path, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.Add("watch-me.txt"); err != nil {
		t.Fatal(err)
	}

	w.Start()
	defer w.Stop()

	// Modify the file and wait for polling to detect it
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case p := <-notified:
		if p != "watch-me.txt" {
			t.Fatalf("notified path = %q, want 'watch-me.txt'", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for poll notification after file change")
	}
}

func TestWatcherHandleFSEventIgnoredOperations(t *testing.T) {
	root := t.TempDir()
	notified := make(chan string, 1)
	w := NewWatcher(root, 0, func(path string) error {
		notified <- path
		return nil
	})

	path := filepath.Join(root, "test.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.Add("test.txt")

	// Simulate fsnotify events that should be ignored
	events := []fsnotify.Event{
		{Name: path, Op: fsnotify.Chmod},
		{Name: path, Op: fsnotify.Remove},
	}

	for _, ev := range events {
		w.handleFSEvent(ev)
		select {
		case p := <-notified:
			t.Fatalf("handleFSEvent(%v) triggered notification for %q, want ignored", ev.Op, p)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestWatcherHandleFSEventRelevantOperations(t *testing.T) {
	root := t.TempDir()
	notified := make(chan string, 1)
	w := NewWatcher(root, 0, func(path string) error {
		notified <- path
		return nil
	})

	path := filepath.Join(root, "test.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.Add("test.txt")

	// Modify the file so the mod time differs from when Add captured it
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate a Write event (mod time now differs from Add's capture)
	w.handleFSEvent(fsnotify.Event{Name: path, Op: fsnotify.Write})

	select {
	case p := <-notified:
		if p != "test.txt" {
			t.Fatalf("notified path = %q, want 'test.txt'", p)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for handleFSEvent notification")
	}
}

func TestWatcherHandleFSEventDeduplicatesModTime(t *testing.T) {
	root := t.TempDir()
	callCount := 0
	var mu sync.Mutex
	w := NewWatcher(root, 0, func(path string) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	})

	path := filepath.Join(root, "test.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.Add("test.txt")

	// Two events with same underlying mod time
	w.handleFSEvent(fsnotify.Event{Name: path, Op: fsnotify.Write})
	w.handleFSEvent(fsnotify.Event{Name: path, Op: fsnotify.Write})

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	count := callCount
	mu.Unlock()

	if count > 1 {
		t.Fatalf("handleFSEvent called notify %d times, want 1 (duplicate dedup)", count)
	}
}

func TestWatcherHandleFSEventRelativePathNotRoot(t *testing.T) {
	root := t.TempDir()
	notified := make(chan string, 1)
	w := NewWatcher(root, 0, func(path string) error {
		notified <- path
		return nil
	})

	path := filepath.Join(root, "test.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.Add("test.txt")

	// Event for the root directory itself — should be ignored
	w.handleFSEvent(fsnotify.Event{Name: root, Op: fsnotify.Write})
	select {
	case <-notified:
		t.Fatal("handleFSEvent notified for root directory, want ignored")
	case <-time.After(50 * time.Millisecond):
	}

	// Event for a path outside the root — the Rel will start with ".." — should be ignored
	outsidePath := filepath.Join(root, "..", "outside.txt")
	w.handleFSEvent(fsnotify.Event{Name: outsidePath, Op: fsnotify.Write})
	select {
	case <-notified:
		t.Fatal("handleFSEvent notified for outside path, want ignored")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWatcherNilReceiver(t *testing.T) {
	var w *Watcher

	// These should not panic
	w.Add("test.txt")
	w.Remove("test.txt")
	w.Start()
	w.Stop()
	w.pollOnce()
	w.handleFSEvent(fsnotify.Event{})
	NewWatcher("", 0, nil) // ensures the func is accessible
}

func TestWatcherAddUpdatesLastSeenModTime(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 0, nil)

	path := filepath.Join(root, "pre-existing.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	wantModTime := info.ModTime()

	if err := w.Add("pre-existing.txt"); err != nil {
		t.Fatal(err)
	}

	w.mu.Lock()
	got, ok := w.lastSeen["pre-existing.txt"]
	w.mu.Unlock()
	if !ok {
		t.Fatal("lastSeen missing 'pre-existing.txt' after Add")
	}
	if !got.Equal(wantModTime) {
		t.Fatalf("lastSeen = %v, want %v", got, wantModTime)
	}
}

func TestWatcherRemoveClearsPolledMap(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher(root, 0, nil)

	if err := w.Add("test.txt"); err != nil {
		t.Fatal(err)
	}

	// Manually mark as polled
	w.mu.Lock()
	w.polled["test.txt"] = struct{}{}
	w.mu.Unlock()

	w.Remove("test.txt")

	w.mu.Lock()
	_, inWatched := w.watched["test.txt"]
	_, inPolled := w.polled["test.txt"]
	w.mu.Unlock()

	if inWatched {
		t.Fatal("watched map still contains 'test.txt' after Remove")
	}
	if inPolled {
		t.Fatal("polled map still contains 'test.txt' after Remove")
	}
}
