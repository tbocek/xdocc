package xdocc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The listing template renders the non-directory items and nothing else.
func TestTemplateListRoundTrip(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		"1-a.md":               "a",
		"2-b.md":               "b",
		"3-dir/1-c.md":         "c",
	})
	b.compile()
	b.want("index.html", `<p>a</p><p>b</p><a href="dir/index.html">dir</a>`)
}

// forloop.last: no separator after the final item.
func TestTemplateForloopLast(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}[{{ x.Content }}]{% unless forloop.last %}|{% endunless %}{% endfor %}`,
		"1-a.md":               "a",
		"2-b.md":               "b",
		"3-c.md":               "c",
	})
	b.compile()
	b.want("index.html", "[<p>a</p>]|[<p>b</p>]|[<p>c</p>]")
}

// Content is raw HTML: it is not escaped by the template engine.
func TestTemplateContentIsRawHTML(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.html":             "<b>bold</b>",
	})
	b.compile()
	b.want("a.html", "<b>bold</b>")
}

// A site template that uses the data. prefix and a custom filter.
func TestTemplateCustomFilter(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}{{ x.URL | base }}{% endfor %}`,
		"1-a.md":               "a",
		"2-b.md":               "b",
	})
	b.compile()
	b.want("index.html", "a.htmlb.html")
}

// The page template inlines the navigation tree built in Go.
func TestTemplateNavHTML(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":   `<nav>{{ data.NavHTML }}</nav>`,
		"1-a.md":                 "a",
		"2-news[News]nav/1-b.md": "b",
	})
	b.compile()
	got := b.read("index.html")
	if !strings.Contains(got, "News") {
		t.Errorf("nav does not contain News: %s", got)
	}
}

// Where an item is shown is readable from a template, as a set of three flags.
func TestTemplateShow(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}[{{ x.Show.Page }}{{ x.Show.List }}{{ x.Show.Link }}]{% endfor %}`,
		"1-a.md":               "a",
		"2-b|show=list.md":     "b",
	})
	b.compile()
	b.want("index.html", "[truetruetrue][falsetruefalse]")
}

func TestLayoutPicksItemTemplate(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":         `{{ data.Content }}`,
		".templates/list.html":         `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/item.html":         `[plain]`,
		".templates/item-compact.html": `[compact]`,
		// the layout is inherited, so it reaches the citations below it
		"1-pub|layout=compact/1-a.bib":       "@misc{a, title = {A}}",
		"1-pub|layout=compact/2-sub/1-b.bib": "@misc{b, title = {B}}",
		"2-other/1-c.bib":                    "@misc{c, title = {C}}",
	})
	b.compile()
	// the listing also links the subdirectory, through file.html
	b.want("pub/index.html", `[compact]<a href="../pub/sub/index.html">sub</a>`)
	b.want("pub/sub/index.html", "[compact]")
	b.want("other/index.html", "[plain]")
}

func TestLayoutHereKeepsItemTemplateLocal(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":         `{{ data.Content }}`,
		".templates/list.html":         `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/item.html":         `[plain]`,
		".templates/item-compact.html": `[compact]`,
		// written "here", so the citations one level down are back to item.html
		"1-pub|layouthere=compact/1-a.bib":       "@misc{a, title = {A}}",
		"1-pub|layouthere=compact/2-sub/1-b.bib": "@misc{b, title = {B}}",
	})
	b.compile()
	b.want("pub/index.html", `[plain]<a href="../pub/sub/index.html">sub</a>`)
	b.want("pub/sub/index.html", "[plain]")
}

func TestLayoutPicksPageTemplate(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":      `<plain>{{ data.Content }}`,
		".templates/page-wide.html": `<wide>{{ data.Content }}`,
		".templates/list.html":      `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/item.html":      `{{ data.Content }}`,
		"1-wide|layout=wide/1-a.md": "a",
		"2-other/1-b.md":            "b",
	})
	b.compile()
	b.want("wide/a.html", "<wide><p>a</p>")
	b.want("other/b.html", "<plain><p>b</p>")
}

func TestMissingTemplateIsAnError(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadTemplates(filepath.Join(dir, TemplateDir)); err == nil {
		t.Fatal("a site with no .templates loaded")
	}
	tmpl := filepath.Join(dir, TemplateDir)
	if err := os.MkdirAll(tmpl, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, name := range required {
		if _, err := LoadTemplates(tmpl); err == nil {
			t.Fatalf("loaded with only %d of %d templates", i, len(required))
		}
		if err := os.WriteFile(filepath.Join(tmpl, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadTemplates(tmpl); err != nil {
		t.Fatalf("a site with every template failed: %v", err)
	}
}

// A listing links to an asset and to a subdirectory, and the two are not the
// same thing: one is a download, the other is a section.
func TestDirTemplateSeparateFromFile(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/item.html": `{{ data.Content }}`,
		".templates/file.html": `<file:{{ data.FileName }}>`,
		".templates/dir.html":  `<dir:{{ data.Name }}>`,
		"1-a.md":               "a",
		"2-notes.pdf":          "%PDF",
		"3-sub[Sub]/1-b.md":    "b",
	})
	b.compile()
	b.want("index.html", "<p>a</p><file:2-notes.pdf><dir:Sub>")
}

// dir.html takes a layout variant like the rest of them.
func TestDirTemplateLayoutVariant(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":           `{{ data.Content }}`,
		".templates/list.html":           `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/item.html":           `{{ data.Content }}`,
		".templates/file.html":           `<file>`,
		".templates/dir.html":            `<dir>`,
		".templates/dir-cards.html":      `<card:{{ data.Name }}>`,
		"1-a|layout=cards/1-x[X]/1-c.md": "c",
		"2-b/1-y[Y]/1-d.md":              "d",
	})
	b.compile()
	b.want("a/index.html", "<card:X>")
	b.want("b/index.html", "<dir>")
}
