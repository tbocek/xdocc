package xdocc

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounce is how long the watcher waits for the file system to settle before
// it recompiles. Editors write in bursts.
const debounce = 200 * time.Millisecond

// Watch compiles the site and then recompiles it whenever the source changes.
// It returns when the context is cancelled.
func (s *Site) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	if err := s.addWatches(watcher, s.Source); err != nil {
		return err
	}

	if result, err := s.Compile(); err != nil {
		log.Printf("xdocc: %v", err)
	} else {
		log.Printf("xdocc: %s", result)
	}

	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		<-timer.C
	}

	// The rescan is read from the root .xdocc, which the first compile has just
	// read, and it stands for the life of the process.
	var rescan <-chan time.Time
	if every := s.Rescan(); every > 0 {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		rescan = ticker.C
		log.Printf("xdocc: rescanning the whole tree every %s", every)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if isHiddenName(filepath.Base(event.Name)) &&
				filepath.Base(event.Name) != XdoccFile &&
				filepath.Dir(event.Name) != filepath.Join(s.Source, TemplateDir) {
				continue
			}
			if s.isExcluded(event.Name) {
				continue
			}
			s.classify(watcher, event)
			timer.Reset(debounce)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				// The kernel queue ran over, so what changed is no longer
				// knowable. Everything is suspect; read the tree again.
				log.Printf("xdocc: the watch queue overflowed, rereading the whole tree")
				s.Invalidate()
				timer.Reset(debounce)
				continue
			}
			log.Printf("xdocc: watch: %v", err)
		case <-rescan:
			s.Invalidate()
			timer.Reset(debounce)
		case <-timer.C:
			start := time.Now()
			result, err := s.Compile()
			if err != nil {
				log.Printf("xdocc: %v", err)
				continue
			}
			if result.Written == 0 && result.Removed == 0 {
				continue // nothing to say: a rescan that found nothing
			}
			log.Printf("xdocc: %s in %s", result, time.Since(start).Round(time.Millisecond))
		}
	}
}

// classify decides what one file system event costs. A file that was written is
// read again on its own; anything that changes the shape of the tree or the way
// it is rendered - a file appearing or vanishing, a changed .xdocc, a changed
// template - is beyond patching and asks for a full walk.
func (s *Site) classify(watcher *fsnotify.Watcher, event fsnotify.Event) {
	if event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			_ = s.addWatches(watcher, event.Name)
		}
		s.Invalidate()
		return
	}
	dir := filepath.Dir(event.Name)
	if filepath.Base(event.Name) == XdoccFile || dir == filepath.Join(s.Source, TemplateDir) {
		s.Invalidate()
		return
	}
	s.Touch(event.Name)
}

// addWatches watches dir and everything below it, output directory excluded.
func (s *Site) addWatches(watcher *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if s.isExcluded(p) {
			return filepath.SkipDir
		}
		base := filepath.Base(p)
		if p != dir && strings.HasPrefix(base, ".") && base != TemplateDir {
			return filepath.SkipDir
		}
		return watcher.Add(p)
	})
}
