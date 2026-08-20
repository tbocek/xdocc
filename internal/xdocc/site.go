package xdocc

import (
	"fmt"
	"os"
	"path/filepath"
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
	s := &Site{Source: source, Gen: gen}
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
	root, err := s.walk(s.Source, nil)
	if err != nil {
		return err
	}
	s.Root = root
	s.cache.forget(s.alive)
	return nil
}

// Symlink reports whether assets are symlinked instead of copied. It is a
// site-wide setting, read from the root .xdocc.
func (s *Site) Symlink() bool {
	if s.Root == nil {
		return false
	}
	v, ok := s.Root.Props.Bool(PropSymlink)
	return ok && v
}

// PostProcessing is the command to run after a successful generation.
func (s *Site) PostProcessing() string {
	if s.Root == nil {
		return ""
	}
	return s.Root.Props[PropPost]
}
