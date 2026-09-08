package xdocc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounce is how long the watcher waits for the file system to settle before
// it recompiles: every event pushes the build further out, so a build happens
// once nothing has changed for this long. Editors write in bursts, and an
// upload of a whole tree over WebDAV is one long burst - rebuilding inside it
// costs a walk of the source per file that arrives and publishes a site that
// is half uploaded.
const debounce = 250 * time.Millisecond

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

	// The first build is the expensive one - nothing is cached and the output
	// tree is whatever was left behind - so it says how long it took, the same
	// way every rebuild after it does.
	start := time.Now()
	if result, err := s.Compile(); err != nil {
		log.Printf("xdocc: %v", err)
	} else {
		log.Printf("xdocc: %s in %s", result, time.Since(start).Round(time.Millisecond))
	}

	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		<-timer.C
	}

	// The rescan is read from the root .xdocc, which the first compile has just
	// read, and it stands for the life of the process. What is being watched and
	// how often it is reread are one fact, so they are one line.
	var rescan <-chan time.Time
	watching := fmt.Sprintf("xdocc: watching %s, building %s after the last change",
		s.Source, debounce)
	if every := s.Rescan(); every > 0 {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		rescan = ticker.C
		watching += fmt.Sprintf(", rereading the whole tree every %s", every)
	}
	log.Print(watching)

	var pending changes
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
			pending.saw(s.Source, event)
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
			pending.sawOther("the rescan came round")
			timer.Reset(debounce)
		case <-timer.C:
			cause := pending.take()
			start := time.Now()
			result, err := s.Compile()
			if err != nil {
				log.Printf("xdocc: %s: %v", cause, err)
				continue
			}
			if result.Written == 0 && result.Removed == 0 {
				continue // nothing to say: a rescan that found nothing
			}
			log.Printf("xdocc: %s in %s, %s", result,
				time.Since(start).Round(time.Millisecond), cause)
		}
	}
}

// changes is what the watcher has seen since the last build, so that a build
// can say what it was for. The log line is the only view a running service
// gives of the path from a file landing to a page changing, and the two
// questions it has to answer are which file did it and how long the file sat
// there - "slow" is either a build that came late or an upload that did.
type changes struct {
	first time.Time // when the first change of this batch arrived
	names []string  // the first few, named; enough to recognise the edit
	more  int       // how many further changes there were
	other string    // a cause that is not a file, e.g. the rescan
}

// saw records one file system event.
func (c *changes) saw(source string, event fsnotify.Event) {
	c.mark()
	name := event.Name
	if rel, err := filepath.Rel(source, name); err == nil {
		name = rel
	}
	if slices.Contains(c.names, name) {
		return
	}
	if len(c.names) < 3 {
		c.names = append(c.names, name)
		return
	}
	c.more++
}

// sawOther records a cause that no file explains.
func (c *changes) sawOther(why string) {
	c.mark()
	c.other = why
}

func (c *changes) mark() {
	if c.first.IsZero() {
		c.first = time.Now()
	}
}

// take describes the batch and clears it. The wait it reports is from the first
// change to now, so it covers the debounce and anything the build queued behind
// - a wait far above the debounce is the interesting case, and it means the
// events kept coming, not that xdocc was idle.
func (c *changes) take() string {
	defer func() { *c = changes{} }()
	if c.first.IsZero() {
		return "for no change anyone reported"
	}
	waited := time.Since(c.first).Round(time.Millisecond)
	switch {
	case len(c.names) == 0:
		return fmt.Sprintf("%s %s ago", c.other, waited)
	case c.more > 0:
		return fmt.Sprintf("for %s and %d more, %s after the first",
			strings.Join(c.names, ", "), c.more, waited)
	default:
		return fmt.Sprintf("for %s, %s after the first",
			strings.Join(c.names, ", "), waited)
	}
}

// classify decides what one file system event costs. A file that was written is
// read again on its own; anything that changes the shape of the tree or the way
// it is rendered - a file appearing or vanishing, a changed .xdocc, a changed
// template - is beyond patching and asks for a full walk.
func (s *Site) classify(watcher *fsnotify.Watcher, event fsnotify.Event) {
	if event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			// A directory nobody is watching is a directory whose files only
			// ever reach the site on the next rescan, which looks like xdocc
			// being slow rather than xdocc being deaf. The usual reason is the
			// kernel's watch limit, so say which directory and why.
			if err := s.addWatches(watcher, event.Name); err != nil {
				log.Printf("xdocc: cannot watch %s, its changes will only be "+
					"seen by the rescan: %v", event.Name, err)
			}
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
