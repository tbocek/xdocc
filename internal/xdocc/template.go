package xdocc

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
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

var defaultTemplates = map[string]string{
	TemplatePage: `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{ if .Name }}{{ .Name }}{{ else }}{{ .URL }}{{ end }}</title>
</head>
<body>
{{ with .GlobalNav }}<nav>{{ template "nav.html" . }}</nav>{{ end }}
<main>
{{ .Content }}
</main>
</body>
</html>
`,
	TemplateNav: `<ul>
{{ range . }}<li><a href="{{ .Href }}"{{ if .Active }} class="active"{{ end }}>{{ .Name }}</a>
{{ with .Children }}{{ template "nav.html" . }}{{ end }}</li>
{{ end }}</ul>
`,
	TemplateList:     `{{ range .Items }}{{ .Content }}{{ end }}`,
	TemplateMarkdown: `{{ .Content }}`,
	TemplateHTML:     `{{ .Content }}`,
	TemplateLink:     `{{ .Content }}`,
	TemplateBib:      `{{ .Content }}`,
	TemplateFile:     `<a href="{{ .Root }}{{ .Link }}">{{ if .Name }}{{ .Name }}{{ else }}{{ .FileName }}{{ end }}</a>`,
}

var templateFuncs = template.FuncMap{
	"base":       path.Base,
	"dir":        path.Dir,
	"date":       func(layout string, t time.Time) string { return t.Format(layout) },
	"html":       func(s string) template.HTML { return template.HTML(s) },
	"join":       strings.Join,
	"hasPrefix":  strings.HasPrefix,
	"hasSuffix":  strings.HasSuffix,
	"trimSuffix": strings.TrimSuffix,
	"trimPrefix": strings.TrimPrefix,
	"lower":      strings.ToLower,
	"upper":      strings.ToUpper,
	"replace":    strings.ReplaceAll,
}

// Templates is the template set of a site: the built-in defaults, with the
// files of .templates layered on top.
type Templates struct {
	tmpl    *template.Template
	Dir     string
	ModTime time.Time // newest template file, for the cache
}

// LoadTemplates parses the built-in templates and everything in dir.
func LoadTemplates(dir string) (*Templates, error) {
	set := template.New("xdocc").Funcs(templateFuncs)
	for name, text := range defaultTemplates {
		if _, err := set.New(name).Parse(text); err != nil {
			return nil, fmt.Errorf("built-in template %s: %w", name, err)
		}
	}
	t := &Templates{tmpl: set, Dir: dir}

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
		if _, err := set.New(entry.Name()).Parse(string(text)); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if info, err := entry.Info(); err == nil && info.ModTime().After(t.ModTime) {
			t.ModTime = info.ModTime()
		}
	}
	return t, nil
}

// Render executes a template and returns its output.
func (t *Templates) Render(name string, data any) (template.HTML, error) {
	var buf bytes.Buffer
	if err := t.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return template.HTML(buf.String()), nil
}

// Has reports whether a template of that name exists.
func (t *Templates) Has(name string) bool { return t.tmpl.Lookup(name) != nil }
