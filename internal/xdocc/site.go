package xdocc

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Site is one source tree and the output tree it generates.
type Site struct {
	Source string // absolute path of the source tree
	Gen    string // absolute path of the output tree

	Root      *Item // the source tree, after Load
	templates *Templates
	excluded  []string
	cache     *Cache
	alive     map[string]bool

	// byRel finds an item again by its path from the source root, which is how
	// a change reported by the watcher is turned back into a tree node.
	byRel map[string]*Item

	// placed is what the last run put into the output tree, keyed by output
	// path. It is the other half of the cache: the cache spares the reading and
	// rendering of sources, this spares the reading back of results.
	placed map[string]placement

	// placedWith are the settings the output tree was built with. They decide
	// what an output file even is - a link or a copy, minified or not, with
	// compressed copies or without - so a change to any of them makes the
	// memory above worthless.
	placedWith outputFlags

	// reads counts the source files this run took off the disk, as opposed to
	// the ones the cache or the tree in memory answered for. It is an atomic
	// because the files are read by whichever worker got to them.
	reads atomic.Int64

	// mu guards the two fields below, which the watcher writes and the compiler
	// reads. They are the only state a second goroutine touches.
	mu       sync.Mutex
	dirty    map[string]bool
	fullLoad bool
}

// SetCache attaches a cache, so that unchanged files are not rendered again.
func (s *Site) SetCache(cache *Cache) { s.cache = cache }

// Cache returns the cache in use, if any.
func (s *Site) Cache() *Cache { return s.cache }

// NewSite prepares a site. It does not read the source tree yet; call Load.
func NewSite(source, gen string) (*Site, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source: %s is not a directory", source)
	}
	if gen != "" {
		if gen, err = filepath.Abs(gen); err != nil {
			return nil, err
		}
	}
	s := &Site{
		Source: source,
		Gen:    gen,
		placed: map[string]placement{},
		dirty:  map[string]bool{},
	}
	if gen != "" {
		s.excluded = append(s.excluded, gen)
	}
	return s, nil
}

// Load reads the source tree and the templates.
func (s *Site) Load() error {
	templates, err := LoadTemplates(filepath.Join(s.Source, TemplateDir))
	if err != nil {
		return err
	}
	s.templates = templates
	s.alive = map[string]bool{}
	s.byRel = map[string]*Item{}
	root, err := s.walk(s.Source, nil)
	if err != nil {
		return err
	}
	s.Root = root
	s.cache.forget(s.alive)
	return nil
}

// readSource reads a source file and counts it, so that a run can say how much
// of the tree it actually had to touch.
func (s *Site) readSource(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		s.reads.Add(1)
	}
	return data, err
}

// Touch records that one source file was written, so that the next compile
// re-reads that file instead of walking the tree. Safe to call from another
// goroutine - the watcher does, and a future WebDAV server would too.
func (s *Site) Touch(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirty[path] = true
}

// Invalidate makes the next compile walk the whole source tree again. It is the
// answer to everything the tree cache cannot absorb on its own: a file that
// appeared or vanished, a changed .xdocc or template, an overflowing watch
// queue, and the periodic rescan that catches whatever slipped past all three.
func (s *Site) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fullLoad = true
	clear(s.dirty)
}

// takeDirty hands the pending changes to the compiler and clears them.
func (s *Site) takeDirty() (paths []string, full bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	full = s.fullLoad
	for p := range s.dirty {
		paths = append(paths, p)
	}
	s.fullLoad = false
	clear(s.dirty)
	return paths, full
}

// refresh brings the item tree in line with the source tree. The first run
// walks everything; after that only the files the watcher named are read again,
// which is what keeps a rebuild off the disk on a big tree. Anything that
// cannot be patched in place falls back to a full walk.
func (s *Site) refresh() error {
	paths, full := s.takeDirty()
	if s.Root == nil || full {
		return s.Load()
	}
	reload := false
	for _, p := range paths {
		ok, err := s.reread(p)
		if err != nil {
			return err
		}
		if !ok {
			reload = true
		}
	}
	if reload {
		return s.Load()
	}
	return nil
}

// reread replaces one file item with a freshly read one. It reports false when
// the change is not one it can patch in, and the caller should walk instead.
func (s *Site) reread(source string) (bool, error) {
	rel, err := filepath.Rel(s.Source, source)
	if err != nil {
		return false, nil
	}
	item, ok := s.byRel[filepath.ToSlash(rel)]
	if !ok || item.IsDir || item.Parent == nil {
		return false, nil
	}
	if _, err := os.Stat(source); err != nil {
		return false, nil
	}
	fresh, err := s.newFile(source, item.Parent)
	if err != nil {
		return false, err
	}
	if fresh == nil {
		return false, nil
	}
	// The item is patched in place, so every pointer the tree already holds to
	// it - its parent's child list, a .link that resolved to it - stays good.
	parent := item.Parent
	*item = *fresh
	s.remember(item)
	sortItems(parent.Children, parent.Sort())
	return true, nil
}

// outputFlags are the settings that decide the shape of the output tree.
type outputFlags struct{ symlink, minify, compress bool }

func (s *Site) outputFlags() outputFlags {
	return outputFlags{symlink: s.Symlink(), minify: s.Minify(), compress: s.Compress()}
}

// Minify reports whether generated pages and the text files beside them are
// minified on the way out. On by default; "minify: false" in the root .xdocc
// turns it off. Site-wide, read from the root .xdocc only.
func (s *Site) Minify() bool {
	if s.Root == nil {
		return true
	}
	v, ok := s.Root.Props.Bool(PropMinify)
	return !ok || v
}

// Compress reports whether a .gz and a .br are written next to every text file
// in the output, which is what a web server serving pre-compressed files wants.
// On by default; "compress: false" in the root .xdocc turns it off. Site-wide,
// read from the root .xdocc only.
func (s *Site) Compress() bool {
	if s.Root == nil {
		return true
	}
	v, ok := s.Root.Props.Bool(PropCompress)
	return !ok || v
}

// defaultRescan is how long the watcher trusts the change notifications before
// it walks the whole tree once to be sure.
const defaultRescan = 10 * time.Minute

// Rescan is how often the watcher rereads the whole source tree even though
// nothing was reported. File system notifications are best effort: a network
// share, a container bind mount or a burst that overran the kernel queue can
// swallow one, and then a page stays stale until someone touches it again. The
// rescan is the backstop, and it costs nothing when nothing changed. Set
// "rescan: 30m" or "rescan: off" in the root .xdocc; read once at startup.
func (s *Site) Rescan() time.Duration {
	if s.Root == nil {
		return defaultRescan
	}
	raw, ok := s.Root.Props[PropRescan]
	if !ok {
		return defaultRescan
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "off", "no", "false", "never", "0":
		return 0
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		log.Printf("xdocc: %s: %q is not a duration like \"10m\", using %s", PropRescan, raw, defaultRescan)
		return defaultRescan
	}
	return d
}

// Symlink reports whether assets are symlinked into the output instead of
// copied. Symlinking is the default: a site whose weight is in its files - a
// lecture folder full of video, a directory of PDFs - is then generated in
// milliseconds and takes no second copy of the disk. A site that hands its
// output to something which cannot follow a link out of the output tree turns
// it off with "symlink: false" in the root .xdocc. It is a site-wide setting,
// read from the root .xdocc only.
//
// Where the file system has no symlinks the build falls back to copying by
// itself, so this is a preference and not a promise.
func (s *Site) Symlink() bool {
	if s.Root == nil {
		return true
	}
	v, ok := s.Root.Props.Bool(PropSymlink)
	return !ok || v
}
