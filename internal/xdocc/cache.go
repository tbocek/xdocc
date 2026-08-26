package xdocc

import (
	"crypto/sha256"
	"encoding/gob"
	"html/template"
	"os"
	"path/filepath"
)

// cacheVersion invalidates the whole cache when the way content is rendered
// changes.
const cacheVersion = 2

// CacheEntry is what is remembered about one source file: its front matter and
// the HTML its handler produced. Neither depends on templates or on .xdocc, so
// only a change of the file itself invalidates it.
type CacheEntry struct {
	Hash    [32]byte // of the file as it is on disk, front matter included
	Front   Props
	Content string
	HasBody bool
}

// Cache remembers rendered content between runs, so that a change to one file
// does not cost a full rebuild. A cache without a path lives in memory only,
// which is what the watcher uses.
type Cache struct {
	Version int
	Entries map[string]*CacheEntry

	path   string
	Hits   int
	Misses int
}

// OpenCache reads a cache file. A missing or unreadable file yields an empty
// cache; an empty path keeps the cache in memory.
func OpenCache(path string) *Cache {
	cache := &Cache{Version: cacheVersion, Entries: map[string]*CacheEntry{}, path: path}
	if path == "" {
		return cache
	}
	file, err := os.Open(path)
	if err != nil {
		return cache
	}
	defer file.Close()
	stored := &Cache{}
	if err := gob.NewDecoder(file).Decode(stored); err != nil {
		return cache
	}
	if stored.Version != cacheVersion || stored.Entries == nil {
		return cache
	}
	cache.Entries = stored.Entries
	return cache
}

// Save writes the cache to disk. It is a no-op for an in-memory cache.
func (c *Cache) Save() error {
	if c == nil || c.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(c.path)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(file).Encode(c); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// Clear throws everything away.
func (c *Cache) Clear() {
	if c == nil {
		return
	}
	c.Entries = map[string]*CacheEntry{}
	c.Hits, c.Misses = 0, 0
}

// lookup returns the entry for a file if the file is byte for byte the one that
// was rendered. Timestamps are deliberately not used: a touch, a checkout or an
// rsync moves them without changing the file, and a coarse file system can hide
// a change behind an unchanged timestamp. Hashing what has to be read anyway is
// cheap next to rendering it.
func (c *Cache) lookup(rel string, data []byte) (*CacheEntry, bool) {
	if c == nil {
		return nil, false
	}
	entry, ok := c.Entries[rel]
	if !ok || entry.Hash != sha256.Sum256(data) {
		c.Misses++
		return nil, false
	}
	c.Hits++
	return entry, true
}

func (c *Cache) put(rel string, entry *CacheEntry) {
	if c == nil {
		return
	}
	c.Entries[rel] = entry
}

// forget drops the entries of files that are no longer in the source tree.
func (c *Cache) forget(alive map[string]bool) {
	if c == nil {
		return
	}
	for rel := range c.Entries {
		if !alive[rel] {
			delete(c.Entries, rel)
		}
	}
}

// cached runs a handler on an item, reusing the result of the previous run when
// the file has not changed.
func (s *Site) cached(item *Item, convert func([]byte) (template.HTML, error)) (template.HTML, error) {
	if item.cache != nil && item.cache.HasBody {
		return template.HTML(item.cache.Content), nil
	}
	body, err := s.body(item)
	if err != nil {
		return "", err
	}
	content, err := convert(body)
	if err != nil {
		return "", err
	}
	if item.cache != nil {
		item.cache.Content = string(content)
		item.cache.HasBody = true
	}
	return content, nil
}

// body returns the content of a file without its front matter, reading it if
// the walk did not have to.
func (s *Site) body(item *Item) ([]byte, error) {
	if item.body != nil {
		return item.body, nil
	}
	data, err := s.readSource(item.Source)
	if err != nil {
		return nil, err
	}
	_, body, err := SplitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	item.body = body
	return body, nil
}
