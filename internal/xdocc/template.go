package xdocc

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/osteele/liquid"
)

// Template names. A site provides all of them in .templates; xdocc has no
// built-in copies, so what a page looks like is written down in the site.
const (
	TemplatePage      = "page.html"
	TemplateList      = "list.html"
	TemplateItem      = "item.html"
	TemplateFile      = "file.html"
	TemplateDirectory = "dir.html"
)

// retired are the templates xdocc used to pick by file type. There is one
// template for an item now and the layout chooses between variants of it, so a
// site that still has these is one rename away from what it meant.
var retired = []string{"markdown.html", "html.html", "link.html", "bib.html"}

// required are the templates every site has to provide. There are no built-in
// copies to fall back on: a missing one is a broken site, not a site that
// silently renders like some other site.
//
// nav.html is not among them and is not a template: the navigation tree is
// recursive, and Liquid renders {% include %} with an empty context, so the
// tree is built in Go (see NavHTML) and page.html inlines the result.
var required = []string{TemplatePage, TemplateList, TemplateItem, TemplateFile, TemplateDirectory}

// filterFuncs are the xdocc-specific Liquid filters, registered on top of the
// standard set. Liquid has no arithmetic in {{ }}, so the numeric helpers a
// template wants come from filters (plus, minus, modulo, divided_by) or these.
var filterFuncs = map[string]any{
	"base":       path.Base,
	"dir":        path.Dir,
	"date":       func(t time.Time, layout string) string { return t.Format(layout) },
	"join":       strings.Join,
	"hasPrefix":  func(s string, p string) bool { return strings.HasPrefix(s, p) },
	"hasSuffix":  func(s string, sfx string) bool { return strings.HasSuffix(s, sfx) },
	"trimSuffix": func(s string, sfx string) string { return strings.TrimSuffix(s, sfx) },
	"trimPrefix": func(s string, p string) string { return strings.TrimPrefix(s, p) },
	"lower":      strings.ToLower,
	"upper":      strings.ToUpper,
	"replace":    func(s string, old, new string) string { return strings.ReplaceAll(s, old, new) },
}

// Templates is the template set of a site: the files of its .templates dir.
type Templates struct {
	engine  *liquid.Engine
	tmpls   map[string]*liquid.Template
	Dir     string
	ModTime time.Time // newest template file, for the cache
}

// LoadTemplates parses everything in dir, and fails unless the site provides
// every required template.
func LoadTemplates(dir string) (*Templates, error) {
	eng := liquid.NewEngine()
	for name, fn := range filterFuncs {
		eng.RegisterFilter(name, fn)
	}
	eng.RegisterTemplateStore(&liquidStore{dir: dir})
	t := &Templates{engine: eng, tmpls: map[string]*liquid.Template{}, Dir: dir}

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: no template directory: a site provides %s",
			dir, strings.Join(required, ", "))
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || isHiddenName(entry.Name()) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".html" && ext != ".htm" && ext != ".tmpl" {
			continue
		}
		file := filepath.Join(dir, entry.Name())
		text, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		tmpl, err := eng.ParseTemplate(text)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		t.tmpls[entry.Name()] = tmpl
		if info, err := entry.Info(); err == nil && info.ModTime().After(t.ModTime) {
			t.ModTime = info.ModTime()
		}
	}
	for _, name := range retired {
		if _, ok := t.tmpls[name]; ok {
			log.Printf("xdocc: %s is not used any more: every item renders with %s, "+
				"and a layout picks a variant of it - rename this to item-<layout>.html "+
				"and set layout=<layout> where it should apply",
				filepath.Join(dir, name), TemplateItem)
		}
	}
	var missing []string
	for _, name := range required {
		if !t.Has(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s: missing template %s", dir, strings.Join(missing, ", "))
	}
	return t, nil
}

// Render executes a template and returns its output.
func (t *Templates) Render(name string, data any) (string, error) {
	tmpl, ok := t.tmpls[name]
	if !ok {
		return "", fmt.Errorf("template %s: not found", name)
	}
	out, err := tmpl.Render(liquid.Bindings{"data": data})
	if err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return string(out), nil
}

// Has reports whether a template of that name exists.
func (t *Templates) Has(name string) bool { _, ok := t.tmpls[name]; return ok }

// liquidStore resolves {% include %} to a file in the site's .templates dir.
// The engine renders includes with an empty context, so xdocc templates do not
// use them; the store is here so a stray include fails against the site dir
// rather than the working directory.
type liquidStore struct{ dir string }

func (s *liquidStore) ReadTemplate(filename string) ([]byte, error) {
	if filepath.IsAbs(filename) {
		return nil, fmt.Errorf("absolute include %s", filename)
	}
	return os.ReadFile(filepath.Join(s.dir, filename))
}
