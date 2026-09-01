package xdocc

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mdBuild is a fixture whose templates put a wrapper around everything, so that
// a markdown copy can be told apart from the HTML page at a glance.
func mdBuild(t *testing.T, files map[string]string) *build {
	t.Helper()
	base := map[string]string{
		".templates/page.html":     `<html>{{ data.Content }}</html>`,
		".templates/markdown.html": `<div>{{ data.Content }}</div>`,
		".templates/list.html":     `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
	}
	for name, content := range files {
		base[name] = content
	}
	return newBuild(t, base)
}

// The markdown copy of a page written in markdown is the source that was
// written: no templates, no wrappers, no navigation.
func TestMarkdownTwinIsTheSource(t *testing.T) {
	b := mdBuild(t, map[string]string{
		"1-a.md": "# Title\n\nhello **world**\n",
	})
	b.compile()

	b.want("a.html", "<html><div><h1 id=\"title\">Title</h1><p>hello <strong>world</strong></p></div></html>")
	if got, want := b.read("a.md"), "# Title\n\nhello **world**"; got != want {
		t.Errorf("a.md = %q, want %q", got, want)
	}
}

// A page is a page whether it is an item or the listing of a directory, and
// both get a copy: "about.html" gets "about.md", an index gets "index.md".
func TestMarkdownTwinOfAListing(t *testing.T) {
	b := mdBuild(t, map[string]string{
		"1-a.md": "first",
		"2-b.md": "second",
	})
	b.compile()

	// the items in the order the listing has them, one block each
	if got, want := b.read("index.md"), "first\n\nsecond"; got != want {
		t.Errorf("index.md = %q, want %q", got, want)
	}
	if got, want := b.read("a.md"), "first"; got != want {
		t.Errorf("a.md = %q, want %q", got, want)
	}
}

// The page points at its own copy, so an agent that reads the HTML can find the
// markdown without knowing the convention.
func TestMarkdownTwinIsLinkedFromThePage(t *testing.T) {
	b := newBuild(t, map[string]string{"1-a.md": "text"})
	b.compile()

	page := b.read("a.html")
	if want := `<link rel="alternate" type="text/markdown" href="a.md">`; !strings.Contains(page, want) {
		t.Errorf("a.html does not link its markdown copy:\n%s", page)
	}
	index := b.read("index.html")
	if want := `href="index.md"`; !strings.Contains(index, want) {
		t.Errorf("index.html does not link its markdown copy:\n%s", index)
	}
}

// "markdown: false" turns the copies off, and a build that had them removes the
// ones it wrote before.
func TestMarkdownOff(t *testing.T) {
	b := mdBuild(t, map[string]string{"1-a.md": "text"})
	b.compile()
	if !b.exists("a.md") {
		t.Fatal("a.md was not written")
	}

	b.xdocc(plainSite + "markdown: false\n")
	b.site.Invalidate()
	if _, err := b.site.Compile(); err != nil {
		t.Fatal(err)
	}
	if b.exists("a.md") || b.exists("index.md") {
		t.Error("the markdown copies are still there after markdown: false")
	}
	// and the page no longer points at a file that is not there
	if strings.Contains(b.read("a.html"), "text/markdown") {
		t.Error("a.html still links a markdown copy")
	}
}

// There is no markdown behind an .html file, so its HTML goes in as it is -
// markdown carries inline HTML, and guessing at the markdown it came from would
// change what the page says.
func TestMarkdownOfAnHTMLItem(t *testing.T) {
	b := mdBuild(t, map[string]string{
		".templates/html.html": `<section>{{ data.Content }}</section>`,
		"1-a.html":             "<html><body><p>raw</p></body></html>",
	})
	b.compile()
	if got, want := b.read("a.md"), "<p>raw</p>"; got != want {
		t.Errorf("a.md = %q, want %q", got, want)
	}
}

// A .bib file is a list of citations, and it is a list in the copy too.
func TestMarkdownOfABib(t *testing.T) {
	b := mdBuild(t, map[string]string{
		"1-pub[2024].bib": `@misc{k,
	author = {Bocek, Thomas},
	title = {Exploring the Use of GenAI},
	booktitle = {Some Venue},
	month = {aug},
	url = {https://example.org/p.pdf},
	year = {2024}
}`,
	})
	b.compile()
	want := `- Thomas Bocek, "Exploring the Use of GenAI", **Some Venue**; August, 2024. <https://example.org/p.pdf>`
	if got := b.read("index.md"); got != want {
		t.Errorf("index.md = %q, want %q", got, want)
	}
}

// A .link file pulls content into a page, and it pulls the same content into
// the page's copy.
func TestMarkdownFollowsALink(t *testing.T) {
	b := mdBuild(t, map[string]string{
		".templates/link.html": `{{ data.Content }}`,
		"1-pull.link":          "url=news/*\n",
		"2-news[News]/1-a.md":  "first",
		"2-news[News]/2-b.md":  "second",
	})
	b.compile()
	if got, want := b.read("pull.md"), "first\n\nsecond"; got != want {
		t.Errorf("pull.md = %q, want %q", got, want)
	}
}

// A directory and a file xdocc only passes through are a link in a listing and
// not content of it, which is what file.html makes of them and what the copy
// makes of them.
func TestMarkdownLinksWhatIsNotTransformed(t *testing.T) {
	b := mdBuild(t, map[string]string{
		"1-notes[The notes].txt": "plain",
		"2-sub[Sub]/1-a.md":      "inner",
	})
	b.compile()
	want := "[The notes](notes.txt)\n\n[Sub](sub/index.html)"
	if got := b.read("index.md"); got != want {
		t.Errorf("index.md = %q, want %q", got, want)
	}
}

// The placeholders are substituted in the copy the same way they are in the
// page, so both say the same thing.
func TestMarkdownSubstitutesPlaceholders(t *testing.T) {
	b := mdBuild(t, map[string]string{
		"1-a[About us].md": "written by ${name}\n",
	})
	b.compile()
	if got, want := b.read("a.md"), "written by About us"; got != want {
		t.Errorf("a.md = %q, want %q", got, want)
	}
}

// A source file already writes to the copy's path, so the copy stays away
// rather than replacing it.
func TestMarkdownKeepsOffAPassedThroughFile(t *testing.T) {
	var out strings.Builder
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)

	b := mdBuild(t, map[string]string{
		"1-a.md":   "the page",
		"index.md": "a file of its own, passed through",
	})
	b.compile()

	if got, want := b.read("index.md"), "a file of its own, passed through"; got != want {
		t.Errorf("index.md = %q, want %q", got, want)
	}
	if !strings.Contains(out.String(), "index.html gets no markdown copy") {
		t.Errorf("nothing was said about the clash: %q", out.String())
	}
	// the page next to it is unaffected
	if got, want := b.read("a.md"), "the page"; got != want {
		t.Errorf("a.md = %q, want %q", got, want)
	}
}

// A page with nothing to say in markdown gets no copy at all, so that a server
// has a missing file to fall back on rather than a blank answer.
func TestMarkdownSkipsAnEmptyPage(t *testing.T) {
	b := mdBuild(t, map[string]string{"photo.jpg": "binary"})
	b.compile()
	if b.exists("index.md") {
		t.Errorf("index.md = %q, want no file", b.read("index.md"))
	}
}

// The copy is text, so it gets the compressed copies every text output gets.
func TestMarkdownIsCompressed(t *testing.T) {
	b := newBuild(t, map[string]string{"1-a.md": filler})
	b.xdocc("") // nothing set: minifying and compressing are the default
	b.compile()

	if got := gunzip(t, b.raw("a.md.gz")); strings.TrimSpace(got) != b.read("a.md") {
		t.Errorf("a.md.gz = %q, want %q", got, b.read("a.md"))
	}
	if !b.exists("a.md.br") {
		t.Error("a.md.br was not written")
	}
	// and it is not minified: markdown is whitespace
	if !strings.HasPrefix(b.read("a.md"), "The quick brown fox") {
		t.Errorf("a.md = %q", b.read("a.md"))
	}
}

// Both renditions of a file are cached, so the markdown costs no second trip to
// the disk. Without that the walk would read every file to hash it and the
// markdown would then read every one of them again.
func TestMarkdownIsCachedWithTheHTML(t *testing.T) {
	b := mdBuild(t, map[string]string{
		"1-a.md": "first",
		"2-b.md": "second",
	})
	b.compile() // fills the cache

	// a fresh run over the tree, with the cache the first one left behind
	site, err := NewSite(b.src, b.gen)
	if err != nil {
		t.Fatal(err)
	}
	site.SetCache(b.cache)
	result, err := site.Compile()
	if err != nil {
		t.Fatal(err)
	}
	// the two sources, read once each by the walk that hashes them, and not a
	// third read between them
	if result.Read != 2 {
		t.Errorf("%s, want 2 read", result)
	}
}

// A changed file changes both of its renditions.
func TestMarkdownFollowsAChangedFile(t *testing.T) {
	b := mdBuild(t, map[string]string{
		"1-a.md": "first",
		"2-b.md": "second",
	})
	b.compile()

	b.file("1-a.md", "changed")
	b.site.Touch(filepath.Join(b.src, "1-a.md"))
	if _, err := b.site.Compile(); err != nil {
		t.Fatal(err)
	}
	if got, want := b.read("a.md"), "changed"; got != want {
		t.Errorf("a.md = %q, want %q", got, want)
	}
	if got, want := b.read("index.md"), "changed\n\nsecond"; got != want {
		t.Errorf("index.md = %q, want %q", got, want)
	}
}
