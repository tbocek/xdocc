package xdocc

import (
	"crypto/sha256"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxLinkDepth stops a .link file that pulls in itself, directly or not.
const maxLinkDepth = 10

// Data is what a template sees.
type Data struct {
	*Item

	Root    string // relative path from the page being rendered to the site root
	Content string // the item, or the listing, as HTML

	Items      []*Data          // the items of a listing
	ItemsByURL map[string]*Data // the same items, keyed by output file name

	GlobalNav  []*Nav
	CurrentNav *Nav
	Breadcrumb []*Nav
}

// NavHTML renders the navigation tree as HTML. Liquid renders {% include %}
// with an empty context, so the recursive tree cannot be a template; it is
// built here and the page template inlines the result.
func (d *Data) NavHTML() string {
	var out strings.Builder
	var walk func(nodes []*Nav)
	walk = func(nodes []*Nav) {
		out.WriteString("<ul>")
		for _, n := range nodes {
			cls := ""
			if n.Active {
				cls = ` class="active"`
			}
			out.WriteString("<li><a href=\"")
			out.WriteString(n.Href)
			out.WriteString("\"")
			out.WriteString(cls)
			out.WriteString(">")
			out.WriteString(n.Name)
			out.WriteString("</a>")
			if n.Children != nil {
				walk(n.Children)
			}
			out.WriteString("</li>")
		}
		out.WriteString("</ul>")
	}
	walk(d.GlobalNav)
	return out.String()
}

// Result counts what one compilation run did to the output tree. Pages and
// Assets add up to the files xdocc is responsible for, and so do Written and
// Unchanged: the first split says what the site is made of, the second what the
// run had to touch.
type Result struct {
	Pages     int // pages rendered from content
	Assets    int // files placed beside them, compressed copies included
	Written   int // output files created or overwritten
	Unchanged int // output files that were already what they should be
	Removed   int // output paths that no longer have a source
	Read      int // source files taken off the disk rather than from a cache
}

// String is the line the command and the watcher print.
func (r Result) String() string {
	out := fmt.Sprintf("%d written, %d unchanged", r.Written, r.Unchanged)
	if r.Removed > 0 {
		out += fmt.Sprintf(", %d removed", r.Removed)
	}
	return fmt.Sprintf("%s (%d pages, %d assets), %d read", out, r.Pages, r.Assets, r.Read)
}

// counts are the tallies of one run. They are apart from Result because the
// workers add to them from several goroutines at once.
type counts struct {
	pages, assets, written, unchanged, removed atomic.Int64
}

// compiler holds the state of one compilation run.
type compiler struct {
	site   *Site
	counts counts
	pool   *pool

	// mu guards the three fields below and the site's memory of the output
	// tree. Everything that costs - reading, minifying, compressing, writing -
	// happens outside it.
	mu       sync.Mutex
	produced map[string]bool

	// claimed is the walk's own record of the output paths it has handed out.
	// Only the walk goroutine touches it, so it needs no lock.
	claimed map[string]bool

	// staleSidecars collects the .gz and .br files found in the source tree,
	// to be said once when the workers are done rather than one line at a time
	// from whichever worker happened to find one.
	staleSidecars []string

	// noSymlink is set once symlinking has failed, so a file system without
	// symlinks costs one failed attempt and not one per file.
	noSymlink bool
}

// result is what the run has done so far.
func (c *compiler) result() Result {
	return Result{
		Pages:     int(c.counts.pages.Load()),
		Assets:    int(c.counts.assets.Load()),
		Written:   int(c.counts.written.Load()),
		Unchanged: int(c.counts.unchanged.Load()),
		Removed:   int(c.counts.removed.Load()),
		Read:      int(c.site.reads.Load()),
	}
}

// claim records an output path on the walk goroutine, before any worker is
// given work for it. Two sources with the same url is a mistake xdocc reports,
// but letting their workers race for the path would make the site depend on
// which one happened to finish last, so the second waits for everything before
// it. It costs nothing on a tree that has no duplicates.
func (c *compiler) claim(rel string) {
	target := filepath.Join(c.site.Gen, filepath.FromSlash(rel))
	if c.claimed[target] {
		// the error this returns is kept by the pool and reported by the wait
		// at the end of the run
		_ = c.pool.wait()
	}
	c.claimed[target] = true
}

// placement reads back what this process last put at an output path.
func (c *compiler) placement(target string) (placement, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.site.placed[target]
	return p, ok
}

// setPlacement records what was just put at an output path.
func (c *compiler) setPlacement(target string, p placement) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.site.placed[target] = p
}

// symlinking reports whether assets are still being linked rather than copied.
func (c *compiler) symlinking() bool {
	if !c.site.Symlink() {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.noSymlink
}

// symlinkFailed switches the run over to copying, and says so once however many
// workers ran into the same wall.
func (c *compiler) symlinkFailed(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.noSymlink {
		return
	}
	// Windows without the privilege, a FAT stick, an exported share: whatever
	// the reason, this tree has no symlinks and copying is what is left.
	log.Printf("xdocc: cannot symlink (%v), copying instead", err)
	c.noSymlink = true
}

// Compile brings the output tree in line with the source tree and reports what
// that took.
func (s *Site) Compile() (Result, error) {
	s.reads.Store(0)
	if err := s.refresh(); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(s.Gen, 0o755); err != nil {
		return Result{}, err
	}
	// What xdocc remembers of the output tree describes a tree built with the
	// settings that were in force then. Change one and none of it holds.
	if flags := s.outputFlags(); flags != s.placedWith {
		clear(s.placed)
		s.placedWith = flags
	}
	c := &compiler{
		site:     s,
		produced: map[string]bool{},
		claimed:  map[string]bool{},
		pool:     newPool(s.Workers()),
	}
	walkErr := c.compileDir(s.Root)
	// Every output has to be accounted for before cleanup can tell what is
	// stale, so the pool is drained even when the walk gave up early.
	if err := c.pool.wait(); err != nil && walkErr == nil {
		walkErr = err
	}
	c.reportSidecars()
	if walkErr != nil {
		return c.result(), walkErr
	}
	if err := c.cleanup(); err != nil {
		return c.result(), err
	}
	if s.cache != nil {
		if err := s.cache.Save(); err != nil {
			return c.result(), err
		}
	}
	return c.result(), nil
}

// compileDir generates everything a directory is responsible for: its items,
// its assets and its index.
func (c *compiler) compileDir(dir *Item) error {
	var items []*Data
	var index *Data

	for _, child := range dir.Children {
		if child.IsDir {
			if err := c.compileDir(child); err != nil {
				return err
			}
			if child.isContent() && child.Show().List {
				data, err := c.render(child, dir, 0)
				if err != nil {
					return err
				}
				items = append(items, data)
			}
			continue
		}

		if !child.IsTransformed() {
			// Reading the file, minifying it and compressing it twice is the
			// bulk of a build and has nothing to do with the walk, so it goes
			// to a worker and the walk carries on.
			c.claim(child.URL)
			c.pool.do(func() error { return c.copyAsset(child) })
			if !child.isContent() {
				continue
			}
		}
		data, err := c.render(child, dir, 0)
		if err != nil {
			return err
		}
		if child.IsIndex() {
			index = data
			continue
		}
		if child.isContent() && child.Show().List {
			items = append(items, data)
		}
		if child.IsTransformed() && child.Show().Page && dir.Show().Page {
			if err := c.writePage(child, data); err != nil {
				return err
			}
		}
	}

	sortData(items, dir.Sort())

	// An item called "index" is the page of its directory and replaces the
	// generated listing. It is the page itself, not an item next to it, so
	// show=page does not apply to it and it is written even into a directory
	// that xdocc otherwise only passes through.
	if index != nil {
		return c.writePage(dir, index)
	}
	// A directory without an order prefix is passed through: its files were
	// copied above, but xdocc adds no listing of its own to it.
	if !dir.isContent() {
		return nil
	}
	listing, err := c.listing(dir, items)
	if err != nil {
		return err
	}
	return c.writePage(dir, listing)
}

// listing renders the index of a directory.
func (c *compiler) listing(dir *Item, items []*Data) (*Data, error) {
	data := c.newData(dir, dir)
	data.Items = items
	data.ItemsByURL = make(map[string]*Data, len(items))
	for _, item := range items {
		data.ItemsByURL[path.Base(item.URL)] = item
	}
	// a directory picks its list template from its OWN layout property — the
	// one set in its name or its own .xdocc, not an inherited one:
	// "layout: root" renders list-root.html. Only self-set values are read,
	// because layout is inherited and a root .xdocc would otherwise make every
	// section's listing use the root's list template.
	name := TemplateList
	if v := dir.self[PropLayout]; v != "" {
		if alt := "list-" + v + ".html"; c.site.templates.Has(alt) {
			name = alt
		}
	}
	content, err := c.site.templates.Render(name, data)
	if err != nil {
		return nil, err
	}
	data.Content = content
	return data, nil
}

// render turns one item into the HTML that goes into a page. page is the item
// whose page it is rendered for, which decides the relative paths.
func (c *compiler) render(item, page *Item, depth int) (*Data, error) {
	data := c.newData(item, page)
	raw, err := c.content(item, page, depth)
	if err != nil {
		return nil, err
	}
	data.Content = substitute(string(raw), item, data.Root)
	rendered, err := c.site.templates.Render(handlerTemplate[item.Handler()], data)
	if err != nil {
		return nil, err
	}
	data.Content = rendered
	return data, nil
}

// content runs the handler of an item and returns its HTML.
func (c *compiler) content(item, page *Item, depth int) (template.HTML, error) {
	if item.IsDir {
		return "", nil
	}
	switch item.Handler() {
	case HandlerMarkdown:
		return c.site.cached(item, func(body []byte) (template.HTML, error) {
			return renderMarkdown(body)
		})
	case HandlerHTML:
		return c.site.cached(item, func(body []byte) (template.HTML, error) {
			return renderHTML(body), nil
		})
	case HandlerBib:
		return c.site.cached(item, func(body []byte) (template.HTML, error) {
			return template.HTML(renderBib(body)), nil
		})
	case HandlerLink:
		return c.link(item, page, depth)
	default:
		return "", nil
	}
}

// link resolves a .link file and renders what it pulls in.
func (c *compiler) link(item, page *Item, depth int) (template.HTML, error) {
	if depth >= maxLinkDepth {
		return "", fmt.Errorf("%s: link nested too deeply", item.Rel)
	}
	body, err := c.site.body(item)
	if err != nil {
		return "", err
	}
	spec := parseLink(body)
	var pulled []*Item
	for _, pattern := range spec.patterns {
		pulled = append(pulled, c.site.resolveLink(pattern, item.Parent)...)
	}
	// The order the patterns were written in is kept; an explicit sort on the
	// .link file re-sorts everything it pulled in.
	if item.Props.Has(PropSort) {
		sortItems(pulled, item.Sort())
	}
	if spec.limit > 0 && len(pulled) > spec.limit {
		pulled = pulled[:spec.limit]
	}
	var out strings.Builder
	for _, target := range pulled {
		// a .link file pulls in what says it is shown by a link
		if !target.Show().Link {
			continue
		}
		if target.IsDir {
			// a directory is pulled in as its own listing
			var items []*Data
			for _, child := range target.ContentItems() {
				if !child.Show().List {
					continue
				}
				data, err := c.render(child, page, depth+1)
				if err != nil {
					return "", err
				}
				items = append(items, data)
			}
			sortData(items, target.Sort())
			listing, err := c.listing(target, items)
			if err != nil {
				return "", err
			}
			out.WriteString(string(listing.Content))
			continue
		}
		data, err := c.render(target, page, depth+1)
		if err != nil {
			return "", err
		}
		out.WriteString(string(data.Content))
	}
	return template.HTML(out.String()), nil
}

// newData builds the template data for an item rendered on a page.
func (c *compiler) newData(item, page *Item) *Data {
	dir := page
	if !dir.IsDir {
		dir = dir.Parent
	}
	data := &Data{Item: item, Root: dir.Root()}
	data.GlobalNav = navTree(c.site.Root, dir)
	data.CurrentNav = findNav(data.GlobalNav, dir)
	data.Breadcrumb = breadcrumb(dir, dir)
	return data
}

// writePage wraps content in the page template and writes it to the output.
func (c *compiler) writePage(item *Item, data *Data) error {
	page := c.newData(item, item)
	page.Content = data.Content
	page.Items = data.Items
	page.ItemsByURL = data.ItemsByURL
	html, err := c.site.templates.Render(TemplatePage, page)
	if err != nil {
		return err
	}
	out := []byte(substitute(string(html), item, page.Root))
	// The page is rendered; minifying it, compressing it and writing it out is
	// not, and that part depends on nothing else in the tree.
	c.claim(item.URL)
	c.pool.do(func() error {
		if c.site.Minify() {
			out = minifyBytes(item.URL, out)
		}
		return c.place(item.URL, nil, func() ([]byte, error) { return out, nil })
	})
	return nil
}

// placeholder matches ${name} and the percent-encoded spelling a markdown link
// destination turns it into.
var placeholder = regexp.MustCompile(`(?i)\$(?:\{|%7B)(name|date|nr|url|path|root)(?:\}|%7D)`)

// substitute replaces the ${...} placeholders inside rendered content. root is
// the way back to the site root from the page the content ends up on.
func substitute(text string, item *Item, root string) string {
	return placeholder.ReplaceAllStringFunc(text, func(match string) string {
		switch strings.ToLower(placeholder.FindStringSubmatch(match)[1]) {
		case "name":
			return item.Name
		case "date":
			if item.Date.IsZero() {
				return ""
			}
			return item.Date.Format("2006-01-02")
		case "nr":
			return strconv.FormatInt(item.Nr, 10)
		case "url":
			return root + item.Link()
		case "path":
			// the directory the item lives in, so that content can build
			// links below it
			return root + strings.TrimSuffix(item.Dir, "/")
		case "root":
			return root
		}
		return match
	})
}

// place puts one output file into the tree, with its compressed copies beside
// it. src is the single source file the output comes from, when there is one:
// its size and mtime let a later run leave an untouched asset alone without
// reading it. A page is rendered from many files and passes nil.
//
// content is a function because on the quiet path it is never called: nothing
// is read, nothing is minified, nothing is compressed.
func (c *compiler) place(rel string, src *Item, content func() ([]byte, error)) error {
	target := filepath.Join(c.site.Gen, filepath.FromSlash(rel))
	c.produce(target)
	if src == nil {
		c.counts.pages.Add(1)
	} else {
		c.counts.assets.Add(1)
	}

	p, known := c.placement(target)
	fresh := known && src != nil && p.link == "" &&
		p.srcSize == src.FileSize && p.srcMod.Equal(src.ModTime)
	if fresh && (!p.sidecars || c.haveSidecars(target)) {
		c.counts.unchanged.Add(1)
		if p.sidecars {
			return c.sidecars(target, nil, false)
		}
		return nil
	}

	data, err := content()
	if err != nil {
		return err
	}
	sidecars := c.compressible(rel, len(data))
	changed, err := c.store(target, data, src, sidecars)
	if err != nil {
		return err
	}
	if !sidecars {
		return nil
	}
	return c.sidecars(target, data, changed)
}

// compressible reports whether an output of that name and size gets compressed
// copies.
func (c *compiler) compressible(rel string, size int) bool {
	return c.site.Compress() && size >= minCompressSize &&
		compressExt[strings.ToLower(path.Ext(rel))]
}

// sidecars keeps the .gz and the .br next to an output file in step with it.
// Compressing at the highest setting is the one expensive thing in a build, so
// it happens only when the bytes it would compress have moved. data may be nil
// when they have not.
func (c *compiler) sidecars(target string, data []byte, changed bool) error {
	for _, e := range encoders {
		name := target + e.suffix
		c.produce(name)
		c.counts.assets.Add(1)
		if data == nil || (!changed && c.have(name)) {
			c.counts.unchanged.Add(1)
			continue
		}
		encoded, err := e.encode(data)
		if err != nil {
			return err
		}
		if _, err := c.store(name, encoded, nil, false); err != nil {
			return err
		}
	}
	return nil
}

// haveSidecars reports whether both compressed copies of an output are there.
func (c *compiler) haveSidecars(target string) bool {
	for _, e := range encoders {
		if !c.have(target + e.suffix) {
			return false
		}
	}
	return true
}

// have reports whether an output file is already there, remembering the answer.
func (c *compiler) have(target string) bool {
	if _, ok := c.placement(target); ok {
		return true
	}
	_, err := os.Lstat(target)
	return err == nil
}

// store writes data at target unless it is already there, and remembers what it
// put there so that the next run does not have to read it back.
func (c *compiler) store(target string, data []byte, src *Item, sidecars bool) (bool, error) {
	p := placement{hash: sha256.Sum256(data), sidecars: sidecars}
	if src != nil {
		p.srcSize, p.srcMod = src.FileSize, src.ModTime
	}
	prev, known := c.placement(target)
	unchanged := known && prev.link == "" && prev.hash == p.hash
	if !known {
		// First run of this process: the output tree is whatever was left
		// behind, so it has to be read back once. A symlink from an earlier run
		// is not read through - writing into it would rewrite the source.
		if info, err := os.Lstat(target); err == nil && info.Mode().IsRegular() {
			if old, err := os.ReadFile(target); err == nil && sha256.Sum256(old) == p.hash {
				unchanged = true
			}
		}
	}
	if unchanged {
		c.setPlacement(target, p)
		c.counts.unchanged.Add(1)
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, err
	}
	if err := os.RemoveAll(target); err != nil {
		return false, err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return false, err
	}
	c.setPlacement(target, p)
	c.counts.written.Add(1)
	return true, nil
}

// copyAsset puts a file into the output. A file xdocc rewrites - a minified
// stylesheet, a minified SVG - is written, because the output is then no longer
// the file the source holds. Everything else is pointed at instead of
// duplicated, and text still gets its compressed copies beside the link.
func (c *compiler) copyAsset(item *Item) error {
	// A .gz or .br next to the file it belongs to is a build artefact that
	// xdocc now keeps itself. Passing it through would write the same output
	// path twice, once from the source and once from the compressor.
	if c.site.Compress() {
		if base, ok := sidecarBase(item); ok {
			c.noteSidecar(item, base)
			return nil
		}
	}
	if _, ok := minifyType[strings.ToLower(path.Ext(item.URL))]; ok && c.site.Minify() {
		return c.place(item.URL, item, func() ([]byte, error) {
			data, err := c.site.readSource(item.Source)
			if err != nil {
				return nil, err
			}
			return minifyBytes(item.Rel, data), nil
		})
	}

	// The link is the same whether the file behind it changed or not, so
	// whether the compressed copies are still good has to be asked before the
	// link is written and the stamp on it renewed.
	target := filepath.Join(c.site.Gen, filepath.FromSlash(item.URL))
	p, _ := c.placement(target)
	fresh := p.srcSize == item.FileSize && p.srcMod.Equal(item.ModTime)

	if err := c.linkAsset(item, target); err != nil {
		return err
	}
	if !c.compressible(item.URL, int(item.FileSize)) {
		return nil
	}
	var data []byte
	if !fresh || !c.haveSidecars(target) {
		var err error
		if data, err = c.site.readSource(item.Source); err != nil {
			return err
		}
	}
	return c.sidecars(target, data, data != nil)
}

// sidecarBase reports whether the file is the compressed copy of a file next to
// it - something a previous build, xdocc or another, left behind - and returns
// the file it was made from.
func sidecarBase(item *Item) (os.FileInfo, bool) {
	for _, e := range encoders {
		base, ok := strings.CutSuffix(item.FileName, e.suffix)
		if !ok || !compressExt[strings.ToLower(path.Ext(base))] {
			continue
		}
		if info, err := os.Stat(filepath.Join(filepath.Dir(item.Source), base)); err == nil {
			return info, true
		}
	}
	return nil, false
}

// noteSidecar records that a .gz or .br in the source tree is xdocc's to write
// now, for reportSidecars to say when the run is done. The copy is derived from
// the file beside it, so as long as that file has not moved there is nothing
// new to note - repeating it on every rebuild would bury everything else in the
// log. It is noted again when the file it comes from changes, which is when
// someone might act on it, and again on the next start, so a fresh log still
// describes the tree.
func (c *compiler) noteSidecar(item *Item, base os.FileInfo) {
	now := stampOf(base)
	rel := filepath.ToSlash(item.Rel)

	c.mu.Lock()
	defer c.mu.Unlock()
	if was, seen := c.site.reported[rel]; seen && was.same(now) {
		return
	}
	c.site.reported[rel] = now
	c.staleSidecars = append(c.staleSidecars, rel)
}

// reportSidecars says in one line what the workers found. They run in whatever
// order the pool gives them, so a line per file would arrive scattered through
// the build and in no fixed order; this waits until they are all done, sorts
// what they found and says it once. Called after the pool is drained.
func (c *compiler) reportSidecars() {
	found := c.staleSidecars
	if len(found) == 0 {
		return
	}
	sort.Strings(found)
	if len(found) == 1 {
		log.Printf("xdocc: %s is generated by xdocc now, the copy in the source tree is ignored", found[0])
		return
	}
	log.Printf("xdocc: %d compressed copies are generated by xdocc now, the ones in the source tree are ignored: %s",
		len(found), strings.Join(found, ", "))
}

// linkAsset points the output at the source instead of duplicating it, and
// copies where that is not possible.
func (c *compiler) linkAsset(item *Item, target string) error {
	c.produce(target)
	c.counts.assets.Add(1)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if c.symlinking() {
		// a relative link keeps working when the output tree is moved; an
		// absolute one (a different volume on Windows) is left as is
		link := item.Source
		if rel, err := filepath.Rel(filepath.Dir(target), item.Source); err == nil && !filepath.IsAbs(rel) {
			link = path.Join(strings.Split(rel, string(os.PathSeparator))...)
		}
		stamp := placement{link: link, srcSize: item.FileSize, srcMod: item.ModTime}
		if p, ok := c.placement(target); ok && p.link == link {
			c.setPlacement(target, stamp)
			c.counts.unchanged.Add(1)
			return nil
		}
		if old, err := os.Readlink(target); err == nil && old == link {
			c.setPlacement(target, stamp)
			c.counts.unchanged.Add(1)
			return nil
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		if err := os.Symlink(link, target); err == nil {
			c.setPlacement(target, stamp)
			c.counts.written.Add(1)
			return nil
		} else {
			c.symlinkFailed(err)
		}
	}
	if p, ok := c.placement(target); ok && p.link == "" &&
		p.srcSize == item.FileSize && p.srcMod.Equal(item.ModTime) {
		c.counts.unchanged.Add(1)
		return nil
	}
	if info, err := os.Lstat(target); err == nil && info.Mode().IsRegular() &&
		info.Size() == item.FileSize && !info.ModTime().Before(item.ModTime) {
		c.setPlacement(target, placement{srcSize: item.FileSize, srcMod: item.ModTime})
		c.counts.unchanged.Add(1)
		return nil
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := copyFile(item.Source, target); err != nil {
		return err
	}
	c.site.reads.Add(1)
	c.setPlacement(target, placement{srcSize: item.FileSize, srcMod: item.ModTime})
	c.counts.written.Add(1)
	return os.Chtimes(target, time.Now(), item.ModTime)
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// produce records an output file and every directory leading to it, so that
// cleanup knows what to keep.
func (c *compiler) produce(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.produced[target] {
		log.Printf("xdocc: %s is written twice, two sources have the same url",
			strings.TrimPrefix(target, c.site.Gen+string(filepath.Separator)))
	}
	c.produced[target] = true
	for dir := filepath.Dir(target); strings.HasPrefix(dir, c.site.Gen); dir = filepath.Dir(dir) {
		if c.produced[dir] {
			break
		}
		c.produced[dir] = true
	}
}

// cleanup removes output that this run did not produce.
func (c *compiler) cleanup() error {
	var stale []string
	err := filepath.WalkDir(c.site.Gen, func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == c.site.Gen || c.produced[p] {
			return nil
		}
		stale = append(stale, p)
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}
	// deepest first, so directories are empty when they are removed
	sort.Slice(stale, func(a, b int) bool { return len(stale[a]) > len(stale[b]) })
	for _, p := range stale {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
		c.counts.removed.Add(1)
		for target := range c.site.placed {
			if target == p || strings.HasPrefix(target, p+string(filepath.Separator)) {
				delete(c.site.placed, target)
			}
		}
	}
	return nil
}

// sortData orders a listing of already rendered items.
func sortData(items []*Data, order string) {
	underlying := make([]*Item, len(items))
	for i, item := range items {
		underlying[i] = item.Item
	}
	descending := isDescending(underlying, order)
	sort.SliceStable(items, func(a, b int) bool {
		return itemLess(items[a].Item, items[b].Item, descending)
	})
}
