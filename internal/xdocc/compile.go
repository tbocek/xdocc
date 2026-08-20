package xdocc

import (
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
	"time"
)

// maxLinkDepth stops a .link file that pulls in itself, directly or not.
const maxLinkDepth = 10

// Data is what a template sees.
type Data struct {
	*Item

	Root    string        // relative path from the page being rendered to the site root
	Content template.HTML // the item, or the listing, as HTML

	Items      []*Data          // the items of a listing
	ItemsByURL map[string]*Data // the same items, keyed by output file name

	GlobalNav   []*Nav
	LocalNav    []*Nav
	CurrentNav  *Nav
	Breadcrumb  []*Nav
	IsGlobalNav bool
}

// compiler holds the state of one compilation run.
type compiler struct {
	site     *Site
	produced map[string]bool
	written  int
}

// Compile reads the source tree and generates the output tree.
func (s *Site) Compile() (int, error) {
	if err := s.Load(); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(s.Gen, 0o755); err != nil {
		return 0, err
	}
	c := &compiler{site: s, produced: map[string]bool{}}
	if err := c.compileDir(s.Root); err != nil {
		return c.written, err
	}
	if err := c.cleanup(); err != nil {
		return c.written, err
	}
	if s.cache != nil {
		if err := s.cache.Save(); err != nil {
			return c.written, err
		}
	}
	return c.written, nil
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
			if child.IsPromoted() {
				promoted, err := c.promotedItems(child, dir, 0)
				if err != nil {
					return err
				}
				items = append(items, promoted...)
				continue
			}
			if child.IsContent() {
				data, err := c.render(child, dir, 0)
				if err != nil {
					return err
				}
				items = append(items, data)
			}
			continue
		}

		if !child.IsTransformed() {
			if err := c.copyAsset(child); err != nil {
				return err
			}
			if !child.IsContent() {
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
		if child.IsContent() {
			items = append(items, data)
		}
		if child.IsTransformed() && child.Split() && dir.Split() {
			if err := c.writePage(child, data); err != nil {
				return err
			}
		}
	}

	sortData(items, dir.Sort())

	// An item called "index" is the page of its directory and replaces the
	// generated listing. It is the page itself, not an item next to it, so
	// split does not apply and it is written even into a directory that xdocc
	// otherwise only passes through.
	if index != nil {
		return c.writePage(dir, index)
	}
	// A directory without an order prefix is passed through: its files were
	// copied above, but xdocc adds no listing of its own to it.
	if !dir.IsContent() || dir.NoIndex() {
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
	content, err := c.site.templates.Render(TemplateList, data)
	if err != nil {
		return nil, err
	}
	data.Content = content
	return data, nil
}

// promotedItems collects the items a promoted directory contributes to its
// parent listing.
func (c *compiler) promotedItems(dir, page *Item, depth int) ([]*Data, error) {
	if depth > maxLinkDepth {
		return nil, nil
	}
	var items []*Data
	for _, child := range dir.Children {
		if child.IsDir && child.IsPromoted() {
			nested, err := c.promotedItems(child, page, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, nested...)
			continue
		}
		if !child.IsContent() || child.IsIndex() {
			continue
		}
		data, err := c.render(child, page, 0)
		if err != nil {
			return nil, err
		}
		items = append(items, data)
	}
	sortData(items, dir.Sort())
	if limit := dir.PromoteLimit(); limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// render turns one item into the HTML that goes into a page. page is the item
// whose page it is rendered for, which decides the relative paths.
func (c *compiler) render(item, page *Item, depth int) (*Data, error) {
	data := c.newData(item, page)
	raw, err := c.content(item, page, depth)
	if err != nil {
		return nil, err
	}
	data.Content = template.HTML(substitute(string(raw), item, data.Root))
	name := TemplateDirectory
	if !item.IsDir {
		name = handlerTemplate[item.Handler()]
	}
	rendered, err := c.site.templates.Render(name, data)
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
		if target.IsDir {
			// a directory is pulled in as its own listing
			var items []*Data
			for _, child := range target.ContentItems() {
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
	data.LocalNav = navTree(dir, dir)
	data.CurrentNav = findNav(data.GlobalNav, dir)
	data.Breadcrumb = breadcrumb(dir, dir)
	data.IsGlobalNav = isInGlobalNav(dir)
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
	return c.write(item.URL, []byte(substitute(string(html), item, page.Root)))
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

// write puts a generated file into the output tree, skipping the write when the
// content is unchanged so that mtimes stay useful.
func (c *compiler) write(rel string, content []byte) error {
	target := filepath.Join(c.site.Gen, filepath.FromSlash(rel))
	c.produce(target)
	if old, err := os.ReadFile(target); err == nil && string(old) == string(content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return err
	}
	c.written++
	return nil
}

// copyAsset copies a file to the output, or symlinks it when the site says so.
func (c *compiler) copyAsset(item *Item) error {
	target := filepath.Join(c.site.Gen, filepath.FromSlash(item.URL))
	c.produce(target)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if c.site.Symlink() {
		if link, err := os.Readlink(target); err == nil && link == item.Source {
			return nil
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		c.written++
		return os.Symlink(item.Source, target)
	}
	if info, err := os.Lstat(target); err == nil && info.Mode().IsRegular() &&
		info.Size() == item.FileSize && !info.ModTime().Before(item.ModTime) {
		return nil
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := copyFile(item.Source, target); err != nil {
		return err
	}
	c.written++
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
