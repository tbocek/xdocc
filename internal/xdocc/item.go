package xdocc

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Item is one file or directory of the source tree.
type Item struct {
	name Name

	Source string // absolute path in the source tree
	Rel    string // path from the source root, as it is on disk
	IsDir  bool
	Props  Props

	Parent   *Item
	Children []*Item // sorted; directories only

	// settings holds the properties this directory passes down to everything
	// below it.
	settings Props

	Name string    // display name
	URL  string    // path from the site root, e.g. "docs/about.html"
	Dir  string    // directory part of URL, e.g. "docs/", "" at the site root
	Date time.Time // zero unless the name or the front matter carries one
	Nr   int64     // sort key

	FileName string // name on disk
	FileSize int64
	ModTime  time.Time

	body  []byte      // file content without the front matter, read during the walk
	cache *CacheEntry // this item's slot in the cache, nil when not cached
}

// Handler returns the handler that turns this item into HTML, or HandlerAsset.
func (i *Item) Handler() string {
	if i.IsDir {
		return HandlerAsset
	}
	return i.name.Handler
}

// IsTransformed reports whether the item is turned into HTML.
func (i *Item) IsTransformed() bool {
	return !i.IsDir && i.isContent() && i.name.Handler != HandlerAsset
}

// isContent reports whether xdocc takes charge of the item: only then is it
// transformed, listed and given a clean URL. The order prefix says so, and the
// root of the site is always content. Without one, a file is passed through
// untouched, which is what lets a self-contained subtree - a demo, a generated
// report, someone else's web app - be dropped into the source tree.
func (i *Item) isContent() bool {
	return i.name.HasOrder || (i.IsDir && i.Parent == nil)
}

// IsIndex reports whether the item is the page of its own directory. Only an
// item that is turned into HTML can be.
func (i *Item) IsIndex() bool { return i.name.IsIndex() && i.IsTransformed() }

// IsNav reports whether the directory belongs in the navigation tree.
func (i *Item) IsNav() bool {
	v, ok := i.Props.Bool(PropNav)
	return ok && v
}

// Split reports whether the item gets a page of its own. On a directory it
// speaks for the items directly inside it; it is not inherited any deeper, so
// "nosplit" at the root folds the front page together without touching the
// sections below it.
func (i *Item) Split() bool {
	if v, ok := i.Props.Bool(PropSplit); ok {
		return v
	}
	// A .bib is a list of citations, not a document. It belongs in a listing
	// and has nothing to put on a page of its own, so it does not ask for one
	// unless the filename says otherwise.
	return i.name.Handler != HandlerBib
}

// Layout is the free-form layout hint handed to templates.
func (i *Item) Layout() string { return i.Props[PropLayout] }

// Link is where this item is reachable, relative to the site root. It is the
// item's own page when it has one, and the index of its directory when it has
// not, which is what an item that does not split ends up in.
func (i *Item) Link() string {
	if i.IsDir || !i.IsTransformed() {
		return i.URL
	}
	if i.Split() && !i.IsIndex() {
		return i.URL
	}
	return i.Parent.URL
}

// Ext is the extension as written on disk, e.g. ".md".
func (i *Item) Ext() string { return i.name.Ext }

// Depth is the number of directories between the site root and this item.
func (i *Item) Depth() int {
	depth := 0
	for p := i.Parent; p != nil; p = p.Parent {
		depth++
	}
	return depth
}

// Root is the relative path from this item back to the site root, e.g. "../".
func (i *Item) Root() string {
	depth := strings.Count(strings.TrimSuffix(i.Dir, "/"), "/")
	if i.Dir != "" {
		depth++
	}
	return strings.Repeat("../", depth)
}

// Sort returns the sort order that applies to this directory's listing.
func (i *Item) Sort() string {
	switch strings.ToLower(i.Props[PropSort]) {
	case SortAsc:
		return SortAsc
	case SortDesc:
		return SortDesc
	default:
		return SortAuto
	}
}

// ContentItems returns the children that belong in the directory's listing.
func (i *Item) ContentItems() []*Item {
	var items []*Item
	for _, child := range i.Children {
		if child.isContent() && !child.IsIndex() {
			items = append(items, child)
		}
	}
	return items
}

// IndexItem returns the child that replaces the generated listing, if any.
func (i *Item) IndexItem() *Item {
	for _, child := range i.Children {
		if child.IsIndex() {
			return child
		}
	}
	return nil
}

// isDescending resolves the sort order of a listing. "auto" reads dated items
// newest first and numbered items in ascending order.
func isDescending(items []*Item, order string) bool {
	switch order {
	case SortAsc:
		return false
	case SortDesc:
		return true
	}
	for _, item := range items {
		if item.name.HasDate {
			return true
		}
	}
	return false
}

// itemLess compares two items of the same listing. Items pinned with "0-" come
// first whatever the direction, and items without an order sort last, by name.
func itemLess(x, y *Item, descending bool) bool {
	if x.name.Pinned != y.name.Pinned {
		return x.name.Pinned
	}
	if x.name.HasOrder != y.name.HasOrder {
		return x.name.HasOrder
	}
	if !x.name.HasOrder {
		return x.FileName < y.FileName
	}
	if x.Nr != y.Nr {
		if descending {
			return x.Nr > y.Nr
		}
		return x.Nr < y.Nr
	}
	return x.FileName < y.FileName
}

// sortItems orders a listing.
func sortItems(items []*Item, order string) {
	descending := isDescending(items, order)
	sort.SliceStable(items, func(a, b int) bool { return itemLess(items[a], items[b], descending) })
}

// isHiddenName reports the names that never make it into the output.
func isHiddenName(name string) bool {
	return name == "" || strings.HasPrefix(name, ".") ||
		strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".bak")
}

// walk reads a directory and returns it as an item, with its children.
func (s *Site) walk(dir string, parent *Item) (*Item, error) {
	item, err := s.newDir(dir, parent)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if isHiddenName(name) {
			continue
		}
		full := filepath.Join(dir, name)
		if s.isExcluded(full) {
			continue
		}
		var child *Item
		if entry.IsDir() {
			child, err = s.walk(full, item)
		} else {
			child, err = s.newFile(full, item)
		}
		if err != nil {
			return nil, err
		}
		if child == nil {
			continue
		}
		item.Children = append(item.Children, child)
	}
	sortItems(item.Children, item.Sort())
	return item, nil
}

// newDir builds the item for a directory, resolving its properties against the
// chain of .xdocc files above it.
func (s *Site) newDir(dir string, parent *Item) (*Item, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	own, err := LoadXdocc(filepath.Join(dir, XdoccFile))
	if err != nil {
		return nil, err
	}
	item := &Item{
		Source:   dir,
		IsDir:    true,
		Parent:   parent,
		FileName: filepath.Base(dir),
		ModTime:  info.ModTime(),
	}
	if parent == nil {
		item.name = ParseName("")
		item.Rel = "."
	} else {
		item.name = ParseName(item.FileName)
		item.Rel = filepath.Join(parent.Rel, item.FileName)
	}

	// A directory's own .xdocc describes the directory itself; its filename
	// wins over it, and only settings are inherited from above.
	item.Props = Props{}
	item.Props.merge(item.name.Props, false)
	item.Props.merge(own, false)
	if parent != nil {
		item.Props.merge(parent.settings, true)
	}
	item.settings = Props{}
	item.settings.merge(item.Props, true)

	if parent == nil {
		item.Dir, item.URL, item.Name = "", IndexURL+".html", ""
	} else {
		segment := item.name.URL
		if !item.name.HasOrder {
			segment = item.FileName
		}
		item.Dir = parent.Dir + segment + "/"
		item.URL = item.Dir + IndexURL + ".html"
		item.Name = item.name.Title
		if title, ok := item.Props[PropName]; ok && title != "" {
			item.Name = title
		}
	}
	item.Nr = item.name.Order
	item.Date = item.name.Date
	return item, nil
}

// newFile builds the item for a file, resolving its properties against its
// front matter and the .xdocc chain. It returns nil for files that are skipped.
func (s *Site) newFile(file string, parent *Item) (*Item, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	item := &Item{
		Source:   file,
		Rel:      filepath.Join(parent.Rel, filepath.Base(file)),
		Parent:   parent,
		FileName: filepath.Base(file),
		FileSize: info.Size(),
		ModTime:  info.ModTime(),
	}
	item.name = ParseName(item.FileName)

	item.Props = Props{}
	item.Props.merge(item.name.Props, false)

	// Front matter is only read from files that are turned into HTML, and the
	// cache spares us even that when the file has not changed. A file without
	// an order prefix is passed through untouched, so it is never opened.
	if item.name.HasOrder {
		switch item.name.Handler {
		case HandlerMarkdown, HandlerHTML, HandlerLink, HandlerBib:
			rel := filepath.ToSlash(item.Rel)
			s.alive[rel] = true
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, err
			}
			entry, ok := s.cache.lookup(rel, data)
			if !ok {
				front, body, err := SplitFrontmatter(data)
				if err != nil {
					return nil, err
				}
				entry = &CacheEntry{Hash: sha256.Sum256(data), Front: front}
				s.cache.put(rel, entry)
				item.body = body
			}
			item.cache = entry
			item.Props.merge(entry.Front, false)
		}
	}
	item.Props.merge(parent.settings, true)

	item.Name = item.name.Title
	if title, ok := item.Props[PropName]; ok && title != "" {
		item.Name = title
	}
	item.Dir = parent.Dir
	item.URL = parent.Dir + item.name.FileName(item.IsTransformed())
	item.Nr = item.name.Order
	item.Date = item.name.Date
	return item, nil
}

// isExcluded keeps the generated tree, the templates and the cache out of the
// source tree walk.
func (s *Site) isExcluded(p string) bool {
	for _, dir := range s.excluded {
		if p == dir || strings.HasPrefix(p, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
