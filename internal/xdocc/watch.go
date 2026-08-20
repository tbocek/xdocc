package xdocc

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounce is how long the watcher waits for the file system to settle before
// it recompiles. Editors write in bursts.
const debounce = 200 * time.Millisecond

// Build compiles the site once and runs the post-processing command.
func (s *Site) Build() (int, error) {
	written, err := s.Compile()
	if err != nil {
		return written, err
	}
	return written, s.postProcess()
}

// postProcess runs the site's post-processing command in the output directory.
func (s *Site) postProcess() error {
	command := s.PostProcessing()
	if command == "" {
		return nil
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = s.Gen
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

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

	if written, err := s.Build(); err != nil {
		log.Printf("xdocc: %v", err)
	} else {
		log.Printf("xdocc: %d files written", written)
	}

	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		<-timer.C
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
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = s.addWatches(watcher, event.Name)
				}
			}
			timer.Reset(debounce)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("xdocc: watch: %v", err)
		case <-timer.C:
			start := time.Now()
			written, err := s.Build()
			if err != nil {
				log.Printf("xdocc: %v", err)
				continue
			}
			log.Printf("xdocc: %d files written in %s", written, time.Since(start).Round(time.Millisecond))
		}
	}
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
