package xdocc

import (
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
