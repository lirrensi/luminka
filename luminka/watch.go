// FILE: luminka/watch.go
// PURPOSE: Watch registered relative paths via fsnotify and emit fs_changed notifications, with a polling fallback.
// OWNS: Watched path registration, fsnotify-based change detection, polling fallback, and watcher lifecycle.
// EXPORTS: Watcher, NewWatcher
// DOCS: docs/spec.md, docs/arch.md, agent_chat/plan_fsnotify_log_recipes_2026-05-08.md

package luminka

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	root     string
	interval time.Duration
	notify   func(string) error

	mu          sync.Mutex
	watched     map[string]struct{}  // all relative paths being watched
	lastSeen    map[string]time.Time // last known mod time for change detection
	polled      map[string]struct{}  // paths watched via polling (fsnotify unavailable)
	watchedDirs map[string]struct{}  // directories registered with fsnotify
	running     bool
	stopped     bool
	stopOnce    sync.Once
	doneOnce    sync.Once
	stopCh      chan struct{}
	doneCh      chan struct{}

	fsw *fsnotify.Watcher
}

func NewWatcher(root string, interval time.Duration, notify func(string) error) *Watcher {
	resolved := root
	if abs, err := filepath.Abs(root); err == nil {
		resolved = abs
	}
	if eval, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = eval
	}
	if interval <= 0 {
		interval = time.Second
	}
	return &Watcher{
		root:        resolved,
		interval:    interval,
		notify:      notify,
		watched:     make(map[string]struct{}),
		lastSeen:    make(map[string]time.Time),
		polled:      make(map[string]struct{}),
		watchedDirs: make(map[string]struct{}),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
}

func (w *Watcher) Add(path string) error {
	if w == nil {
		return nil
	}
	rel, _, err := resolveRelativePath(w.root, path)
	if err != nil {
		return err
	}
	modTime, _, err := currentPathModTime(w.root, rel)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.initMapsLocked()
	w.watched[rel] = struct{}{}
	w.lastSeen[rel] = modTime

	// If fsnotify is running, try to add the directory for this path.
	if w.fsw != nil {
		w.addDirForPathLocked(rel)
	}
	return nil
}

// addDirForPathLocked attempts to add the directory containing rel to the fsnotify watcher.
// If successful, marks the directory as watched. If it fails, marks rel as polled.
// Must be called with mu held.
func (w *Watcher) addDirForPathLocked(rel string) {
	if w.fsw == nil {
		return
	}
	dir := filepath.Dir(rel)
	if _, exists := w.watchedDirs[dir]; exists {
		return
	}
	absDir := filepath.Join(w.root, dir)
	if err := w.fsw.Add(absDir); err != nil {
		// fsnotify cannot watch this directory — mark all watched files in it as polled.
		for p := range w.watched {
			if filepath.Dir(p) == dir {
				w.polled[p] = struct{}{}
			}
		}
		return
	}
	w.watchedDirs[dir] = struct{}{}
}

func (w *Watcher) Remove(path string) error {
	if w == nil {
		return nil
	}
	rel, _, err := resolveRelativePath(w.root, path)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.watched, rel)
	delete(w.lastSeen, rel)
	delete(w.polled, rel)
	return nil
}

func (w *Watcher) Start() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.running || w.stopped {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.initMapsLocked()
	stopCh := w.stopCh

	// Initialize the fsnotify watcher.
	fsw, fswErr := fsnotify.NewWatcher()
	if fswErr == nil {
		w.fsw = fsw
		// Watch the root directory.
		if err := fsw.Add(w.root); err == nil {
			w.watchedDirs["."] = struct{}{}
		}
		// Add directories for already-registered paths.
		for rel := range w.watched {
			w.addDirForPathLocked(rel)
		}
	}

	hasPolled := len(w.polled) > 0
	w.mu.Unlock()

	// Start the fsnotify event-processing goroutine.
	if fswErr == nil && fsw != nil {
		go func() {
			for {
				select {
				case <-stopCh:
					return
				case event, ok := <-fsw.Events:
					if !ok {
						return
					}
					w.handleFSEvent(event)
				case _, ok := <-fsw.Errors:
					if !ok {
						return
					}
					// fsnotify errors are silently ignored (plan says log if logging exists).
				}
			}
		}()
	}

	// Start the polling fallback goroutine. This is the goroutine that manages doneCh.
	// It runs only when there are polled paths (or fsnotify is entirely unavailable).
	needsPolling := hasPolled || fswErr != nil
	go func() {
		defer func() {
			w.mu.Lock()
			w.running = false
			w.mu.Unlock()
			w.doneOnce.Do(func() {
				close(w.doneCh)
			})
		}()

		if !needsPolling {
			// No polling needed — just wait for the stop signal.
			<-stopCh
			return
		}

		interval := w.interval
		if interval <= 0 {
			interval = time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				w.pollOnce()
			}
		}
	}()
}

func (w *Watcher) handleFSEvent(event fsnotify.Event) {
	if w == nil {
		return
	}
	// Determine the relative path from the root.
	// filepath.Rel returns OS-native paths (backslashes on Windows), which matches
	// the output of resolveRelativePath / normalizeRelativePath used for watched map keys.
	relPath, err := filepath.Rel(w.root, event.Name)
	if err != nil {
		return
	}
	if relPath == "." || relPath == ".." {
		return
	}

	// React to writes, creates, and renames (handles atomic-save rename-to-target patterns).
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
		return
	}

	w.mu.Lock()
	_, isWatched := w.watched[relPath]
	_, isPolled := w.polled[relPath]
	w.mu.Unlock()

	if !isWatched || isPolled {
		return
	}

	// Verify the modification time actually changed.
	modTime, exists, err := currentPathModTime(w.root, relPath)
	if err != nil || !exists {
		return
	}

	w.mu.Lock()
	last, hasLast := w.lastSeen[relPath]
	changed := !hasLast || !last.Equal(modTime)
	if changed {
		w.lastSeen[relPath] = modTime
	}
	w.mu.Unlock()

	if changed && w.notify != nil {
		_ = w.notify(relPath)
	}
}

func (w *Watcher) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.stopped {
		running := w.running
		doneCh := w.doneCh
		w.mu.Unlock()
		if running {
			<-doneCh
		}
		return
	}
	w.stopped = true
	running := w.running
	stopCh := w.stopCh
	doneCh := w.doneCh
	fsw := w.fsw
	w.mu.Unlock()

	w.stopOnce.Do(func() {
		close(stopCh)
	})

	// Close the fsnotify watcher to unblock the event goroutine.
	if fsw != nil {
		_ = fsw.Close()
	}

	if running {
		<-doneCh
		return
	}
	w.doneOnce.Do(func() {
		close(doneCh)
	})
}

func (w *Watcher) pollOnce() {
	if w == nil {
		return
	}
	w.mu.Lock()
	// Determine which paths to poll.
	var paths []string
	if w.fsw != nil {
		// fsnotify is working — only poll fallback paths.
		for p := range w.polled {
			paths = append(paths, p)
		}
	} else {
		// fsnotify unavailable — poll all watched paths.
		for p := range w.watched {
			paths = append(paths, p)
		}
	}
	w.mu.Unlock()

	for _, path := range paths {
		modTime, _, err := currentPathModTime(w.root, path)
		if err != nil {
			continue
		}

		w.mu.Lock()
		_, stillWatched := w.watched[path]
		last := w.lastSeen[path]
		changed := stillWatched && !last.Equal(modTime)
		if changed {
			w.lastSeen[path] = modTime
		}
		w.mu.Unlock()

		if changed && w.notify != nil {
			_ = w.notify(path)
		}
	}
}

// initMapsLocked ensures the internal maps are initialized. Must be called with mu held.
func (w *Watcher) initMapsLocked() {
	if w.watched == nil {
		w.watched = make(map[string]struct{})
	}
	if w.lastSeen == nil {
		w.lastSeen = make(map[string]time.Time)
	}
	if w.polled == nil {
		w.polled = make(map[string]struct{})
	}
	if w.watchedDirs == nil {
		w.watchedDirs = make(map[string]struct{})
	}
}
