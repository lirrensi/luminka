// FILE: luminka/watch.go
// PURPOSE: Poll watched relative paths and emit fs_changed notifications.
// OWNS: Watched path registration, change detection, and polling lifecycle.
// EXPORTS: Watcher, NewWatcher
// DOCS: docs/spec.md, docs/arch.md

package luminka

import (
	"path/filepath"
	"sync"
	"time"
)

type Watcher struct {
	root     string
	interval time.Duration
	notify   func(string) error

	mu       sync.Mutex
	watched  map[string]struct{}
	lastSeen map[string]time.Time
	running  bool
	stopped  bool
	stopOnce sync.Once
	doneOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
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
		root:     resolved,
		interval: interval,
		notify:   notify,
		watched:  make(map[string]struct{}),
		lastSeen: make(map[string]time.Time),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
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
	stopCh := w.stopCh
	doneCh := w.doneCh
	interval := w.interval
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			w.running = false
			w.mu.Unlock()
			w.doneOnce.Do(func() {
				close(doneCh)
			})
		}()

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
	w.mu.Unlock()
	w.stopOnce.Do(func() {
		close(stopCh)
	})
	if running {
		<-doneCh
		return
	}
	w.doneOnce.Do(func() {
		close(doneCh)
	})
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
	if w.watched == nil {
		w.watched = make(map[string]struct{})
	}
	if w.lastSeen == nil {
		w.lastSeen = make(map[string]time.Time)
	}
	w.watched[rel] = struct{}{}
	w.lastSeen[rel] = modTime
	return nil
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
	return nil
}

func (w *Watcher) pollOnce() {
	if w == nil {
		return
	}
	w.mu.Lock()
	paths := make([]string, 0, len(w.watched))
	for path := range w.watched {
		paths = append(paths, path)
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
