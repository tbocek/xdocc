package xdocc

import (
	"path/filepath"
	"strings"
	"testing"
)

// warm returns a build whose compiles all go through the same cache file, each
// time reopened from disk and handed to a fresh site. That is what a second run
// of the binary, or a restart of the service, does.
func warm(t *testing.T, files map[string]string) (*build, func()) {
	t.Helper()
	b := newBuild(t, files)
	path := filepath.Join(t.TempDir(), "cache.gob")
	return b, func() {
		t.Helper()
		b.cache = OpenCache(path)
		b.compile()
	}
}

// contentListing shows what each listed item renders to, so a stale listing is
// visible and not just a stale link.
const contentListing = `{% for x in data.Items %}[{{ x.URL }}:{{ x.Content }}]{% endfor %}`

// A .link pulls items out of another directory, so a page can go stale because
// of a file it does not contain. Nothing about that is cached.
func TestCacheInvalidatesLinkSource(t *testing.T) {
	b, run := warm(t, map[string]string{
		".templates/page.html":     `{{ data.Content }}`,
		".templates/list.html":     contentListing,
		".templates/markdown.html": `{{ data.Content }}`,
		".templates/link.html":     `{{ data.Content }}`,
		"1-pull.link":              "url=news/*\n",
		"2-news/1-a.md":            "one",
	})
	const dir = `[news/index.html:<a href="news/index.html">news</a>]`
	run()
	b.want("index.html", "[pull.html:<p>one</p>]"+dir)

	// the pulled file changes: the page that pulls it has to follow
	b.file("2-news/1-a.md", "one changed")
	run()
	b.want("index.html", "[pull.html:<p>one changed</p>]"+dir)
	b.want("news/a.html", "<p>one changed</p>")

	// a new file matching the pattern joins the pulled listing
	b.file("2-news/2-b.md", "two")
	run()
	b.want("index.html", "[pull.html:<p>one changed</p><p>two</p>]"+dir)

	// and leaves it again with the file, which also drops out of the cache
	b.remove("2-news/2-b.md")
	run()
	b.want("index.html", "[pull.html:<p>one changed</p>]"+dir)
	if b.exists("news/b.html") {
		t.Error("news/b.html survived the removal of its source")
	}
	if _, ok := b.cache.Entries["2-news/2-b.md"]; ok {
		t.Error("the cache still holds an entry for the removed file")
	}
}

func TestCacheInvalidatesOnXdoccChange(t *testing.T) {
	b, run := warm(t, map[string]string{
		".templates/page.html":     `{{ data.Content }}`,
		".templates/list.html":     `{% for x in data.Items %}[{{ x.URL }}]{% endfor %}`,
		".templates/markdown.html": `{{ data.Content }}`,
		"1-a.md":                   "one",
		"2-b.md":                   "two",
	})
	run()
	b.want("index.html", "[a.html][b.html]")

	// .xdocc is not a source file, but it decides what the listing looks like
	b.file(".xdocc", "sort: desc\n")
	run()
	b.want("index.html", "[b.html][a.html]")

	// removing one item, again without touching the markdown that stays
	b.remove("2-b.md")
	run()
	b.want("index.html", "[a.html]")
}

func TestCacheInvalidatesOnFrontmatterChange(t *testing.T) {
	b, run := warm(t, map[string]string{
		".templates/page.html":     `{{ data.Content }}`,
		".templates/list.html":     `{% for x in data.Items %}[{{ x.Name }}]{% endfor %}`,
		".templates/markdown.html": `{{ data.Content }}`,
		"1-a.md":                   "---\nname: First\n---\none\n",
	})
	run()
	b.want("index.html", "[First]")

	b.file("1-a.md", "---\nname: Renamed\n---\none\n")
	run()
	b.want("index.html", "[Renamed]")
}

func TestCacheInvalidatesOnTemplateChange(t *testing.T) {
	b, run := warm(t, map[string]string{
		".templates/page.html":     `{{ data.Content }}`,
		".templates/list.html":     `{% for x in data.Items %}[{{ x.URL }}]{% endfor %}`,
		".templates/markdown.html": `{{ data.Content }}`,
		"1-a.md":                   "one",
	})
	run()
	b.want("index.html", "[a.html]")
	b.want("a.html", "<p>one</p>")

	// templates are never cached, so a change to one reaches every page
	b.file(".templates/list.html", `{% for x in data.Items %}({{ x.URL }}){% endfor %}`)
	b.file(".templates/markdown.html", `<div>{{ data.Content }}</div>`)
	run()
	b.want("index.html", "(a.html)")
	b.want("a.html", "<div><p>one</p></div>")
}

func TestCacheRenameRemovesOldPage(t *testing.T) {
	b, run := warm(t, map[string]string{
		".templates/page.html":     `{{ data.Content }}`,
		".templates/list.html":     `{% for x in data.Items %}[{{ x.URL }}]{% endfor %}`,
		".templates/markdown.html": `{{ data.Content }}`,
		"1-a.md":                   "one",
	})
	run()
	b.want("index.html", "[a.html]")

	b.remove("1-a.md")
	b.file("1-renamed.md", "one")
	run()
	b.want("index.html", "[renamed.html]")
	if b.exists("a.html") {
		t.Error("a.html survived the rename")
	}
}

func TestCacheIgnoresTimestamps(t *testing.T) {
	b, run := warm(t, map[string]string{
		".templates/page.html":     `{{ data.Content }}`,
		".templates/markdown.html": `{{ data.Content }}`,
		"1-a.md":                   "one",
	})
	run()

	// rewriting a file with the same content, as a checkout or an rsync does,
	// moves its timestamp but must not cost a rendering
	b.file("1-a.md", "one")
	run()
	if b.cache.Hits != 1 || b.cache.Misses != 0 {
		t.Errorf("after a touch: %d hits, %d misses, want 1 and 0", b.cache.Hits, b.cache.Misses)
	}

	// a change of the same length, which a timestamp of low resolution could
	// hide, is still seen
	b.file("1-a.md", "two")
	run()
	if b.cache.Hits != 0 || b.cache.Misses != 1 {
		t.Errorf("after an edit: %d hits, %d misses, want 0 and 1", b.cache.Hits, b.cache.Misses)
	}
	if got := b.read("a.html"); !strings.Contains(got, "two") {
		t.Errorf("a.html = %q", got)
	}
}
