package xdocc

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/osteele/liquid"
)

// Template names. A site overrides any of them by dropping a file of the same
// name into .templates.
const (
	TemplatePage     = "page.html"
	TemplateList     = "list.html"
	TemplateMarkdown = "markdown.html"
	TemplateHTML     = "html.html"
	TemplateLink     = "link.html"
	TemplateBib      = "bib.html"
	TemplateFile     = "file.html"
	TemplateNav      = "nav.html"
)

// handlerTemplate maps a handler to the template that renders one of its items.
// A directory has no handler of its own and is rendered by file.html, the same
// as an asset: both are an item a listing links to rather than shows.
var handlerTemplate = map[string]string{
	HandlerMarkdown: TemplateMarkdown,
	HandlerHTML:     TemplateHTML,
	HandlerLink:     TemplateLink,
	HandlerBib:      TemplateBib,
	HandlerAsset:    TemplateFile,
}

// defaultTemplates are the built-in templates, in Liquid. A site overrides any
// of them by dropping a same-named file into .templates.
var defaultTemplates = map[string]string{
	TemplatePage: `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{% if data.Name %}{{ data.Name }}{% else %}{{ data.URL }}{% endif %}</title>
{% if data.MarkdownURL %}<link rel="alternate" type="text/markdown" href="{{ data.MarkdownURL }}">{% endif %}
</head>
<body>
{% if data.GlobalNav %}<nav>{{ data.NavHTML }}</nav>{% endif %}
<main>
{{ data.Content }}
</main>
</body>
</html>
`,
	// nav.html is not a template: the navigation tree is recursive, and Liquid
	// renders {% include %} with an empty context, so the tree is built in Go
	// (see NavHTML) and the page template inlines the result.
	TemplateList:     `{% for it in data.Items %}{{ it.Content }}{% endfor %}`,
	TemplateMarkdown: `{{ data.Content }}`,
	TemplateHTML:     `{{ data.Content }}`,
	TemplateLink:     `{{ data.Content }}`,
	TemplateBib:      `{{ data.Content }}`,
	TemplateFile:     `<a href="{{ data.Root }}{{ data.Link }}">{% if data.Name %}{{ data.Name }}{% else %}{{ data.FileName }}{% endif %}</a>`,
}

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

// Templates is the template set of a site: the built-in defaults, with the
// files of .templates layered on top.
type Templates struct {
	engine  *liquid.Engine
	tmpls   map[string]*liquid.Template
	Dir     string
	ModTime time.Time // newest template file, for the cache
}

// LoadTemplates parses the built-in templates and everything in dir.
func LoadTemplates(dir string) (*Templates, error) {
	eng := liquid.NewEngine()
	for name, fn := range filterFuncs {
		eng.RegisterFilter(name, fn)
	}
	eng.RegisterTemplateStore(&liquidStore{dir: dir})
	t := &Templates{engine: eng, tmpls: map[string]*liquid.Template{}, Dir: dir}

	for name, text := range defaultTemplates {
		tmpl, err := eng.ParseTemplate([]byte(text))
		if err != nil {
			return nil, fmt.Errorf("built-in template %s: %w", name, err)
		}
		t.tmpls[name] = tmpl
	}

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return t, nil
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
