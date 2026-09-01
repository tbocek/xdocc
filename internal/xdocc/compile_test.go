package xdocc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// site builds a source tree from a map of path to content and compiles it.
type build struct {
	t     *testing.T
	site  *Site
	src   string
	gen   string
	cache *Cache
}

func newBuild(t *testing.T, files map[string]string) *build {
	t.Helper()
	dir := t.TempDir()
	b := &build{t: t, src: filepath.Join(dir, "src"), gen: filepath.Join(dir, "gen")}
	if err := os.MkdirAll(b.src, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		b.file(name, content)
	}
	// A site provides its own templates, so a fixture is not a site until it
	// has them. These are the plainest ones that still render: a test writes
	// its own where the template is what it is about.
	for name, content := range fixtureTemplates {
		if _, ok := files[TemplateDir+"/"+name]; !ok {
			b.file(TemplateDir+"/"+name, content)
		}
	}
	// These tests are about what xdocc generates, not about how small it can
	// make it: minified markup and a .gz beside every page would only obscure
	// the assertions. A test that is about them says so in its own .xdocc.
	// Minifying and compressing are on by default, and everywhere but in their
	// own tests they would only obscure what is being asserted. Switch them off
	// for the fixture; a test that is about them writes its own .xdocc.
	b.xdocc(plainSite + files[XdoccFile])
	b.cache = OpenCache("")
	return b
}

// fixtureTemplates are the stand-ins newBuild writes for whatever templates a
// test did not bring its own of.
var fixtureTemplates = map[string]string{
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
	TemplateList:      `{% for it in data.Items %}{{ it.Content }}{% endfor %}`,
	TemplateItem:      `{{ data.Content }}`,
	TemplateFile:      `<a href="{{ data.Root }}{{ data.Link }}">{% if data.Name %}{{ data.Name }}{% else %}{{ data.FileName }}{% endif %}</a>`,
	TemplateDirectory: `<a href="{{ data.Root }}{{ data.Link }}">{% if data.Name %}{{ data.Name }}{% else %}{{ data.FileName }}{% endif %}</a>`,
}

// plainSite is the root .xdocc of a fixture that is about content only.
const plainSite = "minify: false\ncompress: false\n"

// xdocc writes the root settings, replacing what newBuild put there.
func (b *build) xdocc(content string) { b.file(XdoccFile, content) }

func (b *build) file(name, content string) {
	b.t.Helper()
	full := filepath.Join(b.src, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		b.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		b.t.Fatal(err)
	}
}

func (b *build) remove(name string) {
	b.t.Helper()
	if err := os.RemoveAll(filepath.Join(b.src, filepath.FromSlash(name))); err != nil {
		b.t.Fatal(err)
	}
}

func (b *build) compile() {
	b.t.Helper()
	site, err := NewSite(b.src, b.gen)
	if err != nil {
		b.t.Fatal(err)
	}
	site.SetCache(b.cache)
	if _, err := site.Compile(); err != nil {
		b.t.Fatal(err)
	}
	b.site = site
}

func (b *build) read(name string) string {
	b.t.Helper()
	data, err := os.ReadFile(filepath.Join(b.gen, filepath.FromSlash(name)))
	if err != nil {
		b.t.Fatalf("reading %s: %v", name, err)
	}
	return strings.TrimSpace(string(data))
}

func (b *build) exists(name string) bool {
	_, err := os.Lstat(filepath.Join(b.gen, filepath.FromSlash(name)))
	return err == nil
}

// want compares a generated file with the expected content, ignoring line
// breaks, which say nothing about correctness here.
func (b *build) want(name, content string) {
	b.t.Helper()
	if got := strings.ReplaceAll(b.read(name), "\n", ""); got != content {
		b.t.Errorf("%s = %q, want %q", name, got, content)
	}
}

// listing prints the url of every item, which is what most tests check.
const listTemplate = `{% for x in data.Items %}[{{ x.URL }}]{% endfor %}`

func TestCompileBasics(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		".templates/item.html": `({{ data.Content }})`,
		"1-intro.md":           "hello **world**",
		"2-about[About us].md": "about",
		"logo.svg":             "<svg/>",
	})
	b.compile()

	b.want("intro.html", "(<p>hello <strong>world</strong></p>)")
	b.want("about.html", "(<p>about</p>)")
	b.want("index.html", "[intro.html][about.html]")
	b.want("logo.svg", "<svg/>")
}

func TestCompileOrderAndSort(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		"2-b.md":               "b",
		"1-a.md":               "a",
		"0-pinned.md":          "pin",
		"10-c.md":              "c",
	})
	b.compile()
	b.want("index.html", "[pinned.html][a.html][b.html][c.html]")

	// dates sort newest first
	b2 := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		"2024-01-01-old.md":    "old",
		"2025-06-02-new.md":    "new",
		"0-title.md":           "title",
	})
	b2.compile()
	b2.want("index.html", "[title.html][new.html][old.html]")

	// an explicit sort wins
	b3 := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		".xdocc":               "sort: desc\n",
		"1-a.md":               "a",
		"2-b.md":               "b",
	})
	b3.compile()
	b3.want("index.html", "[b.html][a.html]")
}

func TestCompileAssetsAndVisible(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		"1-photo.jpg":          "binary",
		"untouched.txt":        "as is",
		"7.md":                 "no order prefix",
	})
	b.compile()

	// a content item that has no handler keeps its extension and is listed
	b.want("photo.jpg", "binary")
	// files without an order prefix are copied verbatim, and not listed
	b.want("untouched.txt", "as is")
	// a handled file without an order prefix is not touched at all
	b.want("7.md", "no order prefix")
	if b.exists("7.html") {
		t.Error("7.md was transformed although it has no order prefix")
	}
	b.want("index.html", "[photo.jpg]")
}

func TestCompileIndexItem(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `[{% if data.Content %}{{ data.Content }}{% endif %}]`,
		".templates/list.html": listTemplate,
		".templates/item.html": `{{ data.Content }}`,
		"1-index.md":           "the page itself",
		"2-other.md":           "other",
	})
	b.compile()
	// the index item replaces the generated listing
	b.want("index.html", "[<p>the page itself</p>]")
	b.want("other.html", "[<p>other</p>]")
	if b.exists("index.html") && b.read("index.html") == "" {
		t.Error("index.html is empty")
	}
	// an order with no url after it - "7-.md" - is not a name xdocc reads, so
	// the file is passed through and the listing is generated as usual
	b2 := newBuild(t, map[string]string{
		".templates/page.html": `[{% if data.Content %}{{ data.Content }}{% endif %}]`,
		".templates/list.html": listTemplate,
		"7-.md":                "empty url",
	})
	b2.compile()
	b2.want("7-.md", "empty url")
	b2.want("index.html", "[]")
}

func TestCompileShowPlaces(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":         `{{ data.Content }}`,
		".templates/list.html":         listTemplate,
		".templates/item.html":         `[{{ data.URL }}]`,
		".templates/item-pull.html":    `({{ data.Content }})`,
		"1-pull|layout=pull.link":      "url=a/*\n",
		"1-a[A]/0-x|show=page.md":      "hidden from both",
		"1-a[A]/1-o|show=page-link.md": "links only",
		"1-a[A]/2-y.md":                "visible",
		"2-sib[Sib]/1-c.md":            "sib",
	})
	b.compile()
	// the generated listing skips whatever leaves "list" out
	b.want("a/index.html", "[a/y.html]")
	// a .link pulls in what keeps "link"
	b.want("pull.html", "([a/o.html][a/y.html])")
	// both are structural, not inherited: a sibling dir without them still lists
	b.want("index.html", "[a/index.html][pull.html][sib/index.html]")
	b.want("sib/index.html", "[sib/c.html]")

	// an explicit .link name does not pull an item without "link" either
	b2 := newBuild(t, map[string]string{
		".templates/page.html":      `{{ data.Content }}`,
		".templates/list.html":      listTemplate,
		".templates/item.html":      `[{{ data.URL }}]`,
		".templates/item-pull.html": `({{ data.Content }})`,
		"1-pull|layout=pull.link":   "url=a/x\n",
		"1-a[A]/0-x|show=page.md":   "hidden",
	})
	b2.compile()
	b2.want("pull.html", "()")
}

func TestCompileShowPage(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		"1-news/.xdocc":        "show=list-link\n",
		"1-news/1-a.md":        "a",
		"1-news/2-b.md":        "b",
	})
	b.compile()
	b.want("news/index.html", "[news/a.html][news/b.html]")
	if b.exists("news/a.html") {
		t.Error("news/a.html was written although the directory leaves page out")
	}

	// a single item can opt out
	b2 := newBuild(t, map[string]string{
		".templates/page.html":  `{{ data.Content }}`,
		".templates/list.html":  listTemplate,
		"1-a|show=list-link.md": "a",
		"2-b.md":                "b",
	})
	b2.compile()
	if b2.exists("a.html") {
		t.Error("a.html was written although the item leaves page out")
	}
	if !b2.exists("b.html") {
		t.Error("b.html is missing")
	}
	b2.want("index.html", "[a.html][b.html]")
}

func TestCompileHiddenAndFrontmatter(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}[{{ x.Name }}|{{ x.Date | date: "2006-01-02" }}]{% endfor %}`,
		"2025-06-02-a.md":      "---\nname: From front matter\n---\ntext\n",
		".hidden.md":           "invisible",
		"1-b.md~":              "invisible",
		"2-c.bak":              "invisible",
		"3-c[From name].md":    "c",
	})
	b.compile()
	b.want("index.html", "[From front matter|2025-06-02][From name|0001-01-01]")
	for _, name := range []string{"b.html", "hidden.html", ".hidden.md", "1-b.md~", "2-c.bak"} {
		if b.exists(name) {
			t.Errorf("%s reached the output", name)
		}
	}
}

func TestCompileSubstitution(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-dir/1-a.md":         "name=${name} url=${url} root=${root} nr=${nr}",
	})
	b.compile()
	b.want("dir/a.html", "<p>name=a url=../dir/a.html root=../ nr=1</p>")
}

func TestCompileDeleteStaleOutput(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "a",
		"2-b.md":               "b",
	})
	b.compile()
	if !b.exists("b.html") {
		t.Fatal("b.html is missing")
	}
	b.remove("2-b.md")
	b.compile()
	if b.exists("b.html") {
		t.Error("b.html survived the removal of its source")
	}
	if !b.exists("a.html") {
		t.Error("a.html disappeared")
	}
}

func TestCompileCacheReusesRenderedContent(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "a",
		"2-b.md":               "b",
	})
	b.compile()
	misses := b.cache.Misses
	if misses != 2 {
		t.Fatalf("first run: %d misses, want 2", misses)
	}
	b.cache.Hits = 0
	b.compile()
	if b.cache.Hits != 2 {
		t.Errorf("second run: %d hits, want 2", b.cache.Hits)
	}
}

func TestCompileHiddenDirectory(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		"1-a.md":               "a",
		".draft/1-b.md":        "b",
		"2-also~/1-c.md":       "c",
	})
	b.compile()
	b.want("index.html", "[a.html]")
	for _, name := range []string{".draft", "draft", "2-also~", "also"} {
		if b.exists(name) {
			t.Errorf("%s reached the output", name)
		}
	}
}

// "visible", "copy" and "promote" are what the order prefix says now. Trees
// that still spell them must keep compiling, with the prefix having the last
// word.
func TestCompileLegacyPropertiesAreIgnored(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":    `{{ data.Content }}`,
		".templates/list.html":    listTemplate,
		".xdocc":                  "visible\n",
		"0-shown|hidden.md":       "still here",
		"1-raw|copy.md":           "# converted after all",
		"2-normal.md":             "# converted",
		"3-featured|prm/1-one.md": "one",
		"4-plain|noindex/1-p.md":  "p",
		"alpha.md":                "alpha",
	})
	b.compile()
	// "copy" no longer holds a file back from its handler
	b.want("raw.html", "<h1 id=\"converted-after-all\">converted after all</h1>")
	b.want("normal.html", "<h1 id=\"converted\">converted</h1>")
	// "visible" no longer pulls in a file without an order prefix
	b.want("alpha.md", "alpha")
	if b.exists("alpha.html") {
		t.Error("alpha.html was written although alpha.md has no order prefix")
	}
	// "hidden" is what a leading dot says now
	b.want("shown.html", "<p>still here</p>")
	// "promote" no longer merges a directory into its parent
	b.want("index.html", "[shown.html][raw.html][normal.html][featured/index.html][plain/index.html]")
	b.want("featured/index.html", "[featured/one.html]")
	// "noindex" no longer holds back a listing, which is what kept the entry
	// above from pointing at a page that was never written
	b.want("plain/index.html", "[plain/p.html]")
}

func TestCompileHTMLHandler(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.html":             "<html><head><title>x</title></head><body><p>only this</p></body></html>",
		"2-b.htm":              "<p>no body tag</p>",
	})
	b.compile()
	b.want("a.html", "<p>only this</p>")
	b.want("b.html", "<p>no body tag</p>")
}

func TestCompileStackedExtensions(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-recipe.link.md":     "# markdown after all",
	})
	b.compile()
	b.want("recipe.html", "<h1 id=\"markdown-after-all\">markdown after all</h1>")
}

func TestCompileDuplicateURLsAreReported(t *testing.T) {
	// two sources that produce the same output file
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "one",
		"2-a.md":               "two",
	})
	b.compile()
	// the last one wins, and the run does not fail
	b.want("a.html", "<p>two</p>")
}

// A subtree without order prefixes is passed through untouched. This is how a
// self-contained thing - a demo, someone else's web app, a generated report -
// is dropped into the source tree.
func TestCompilePassthroughSubtree(t *testing.T) {
	const page = "<html><head><title>Demo</title></head><body>hi</body></html>"
	b := newBuild(t, map[string]string{
		".templates/page.html": `[{% if data.Content %}{{ data.Content }}{% endif %}]`,
		".templates/list.html": listTemplate,
		".xdocc":               "show=list-link\n",
		"1-a.md":               "a",
		"demo/index.html":      page,
		"demo/hash.html":       "<html><body>hash</body></html>",
		"demo/js/app.js":       "run()",
	})
	b.compile()

	// every file keeps its name and its bytes, head and all
	b.want("demo/index.html", page)
	b.want("demo/hash.html", "<html><body>hash</body></html>")
	b.want("demo/js/app.js", "run()")

	// no generated index is added to a directory xdocc does not own
	if b.exists("demo/js/index.html") {
		t.Error("an index was generated for a directory without an order prefix")
	}
	// and the directory does not appear in the listing above it
	b.want("index.html", "[[a.html]]")
}

// A directory that xdocc otherwise passes through still gets the page an
// ordered index item inside it describes. That is how a folder of lecture
// material carries a written introduction without being ordered itself.
func TestCompileIndexItemInPassthroughDir(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":           `[{% if data.Content %}{{ data.Content }}{% endif %}]`,
		".templates/list.html":           listTemplate,
		".templates/item.html":           `{{ data.Content }}`,
		"1-a.md":                         "a",
		"fs25/2025-02-17-index[FS25].md": "# FS25",
		"fs25/slides.pdf":                "%PDF",
	})
	b.compile()
	b.want("fs25/index.html", `[<h1 id="fs25">FS25</h1>]`)
	b.want("fs25/slides.pdf", "%PDF")
	// the directory is still not part of the site's own structure
	b.want("index.html", "[[a.html]]")
}

func TestCompileListTemplateSelection(t *testing.T) {
	// a directory's own layout property picks a list-<value>.html file
	b := newBuild(t, map[string]string{
		".templates/page.html":      `{{ data.Content }}`,
		".templates/list.html":      listTemplate,
		".templates/list-root.html": "[ROOT]",
		".xdocc":                    "show=list-link\nlayout: root\n",
		"1-a.md":                    "a",
		"2-b.md":                    "b",
		"3-c/1-d.md":                "d",
	})
	b.compile()
	b.want("index.html", "[ROOT]")
	b.want("c/index.html", "[c/d.html]")

	// the property can live in a directory's name
	b2 := newBuild(t, map[string]string{
		".templates/page.html":       `{{ data.Content }}`,
		".templates/list.html":       listTemplate,
		".templates/list-other.html": "[OTHER]",
		"1-a.md":                     "a",
		"2-b/1-c.md":                 "c",
		"3-d|layout=other/1-e.md":    "e",
	})
	b2.compile()
	b2.want("b/index.html", "[b/c.html]")
	b2.want("d/index.html", "[OTHER]")

	// an inherited layout does not select a list template: only what a
	// directory sets for itself does
	b3 := newBuild(t, map[string]string{
		".templates/page.html":      `{{ data.Content }}`,
		".templates/list.html":      listTemplate,
		".templates/list-root.html": "[ROOT]",
		".xdocc":                    "layout: root\n",
		"1-a.md":                    "a",
		"3-s/1-t.md":                "t",
	})
	b3.compile()
	b3.want("index.html", "[ROOT]")
	b3.want("s/index.html", "[s/t.html]")

	// a directory's own .xdocc selects for that directory only
	b4 := newBuild(t, map[string]string{
		".templates/page.html":       `{{ data.Content }}`,
		".templates/list.html":       listTemplate,
		".templates/list-other.html": "[OTHER]",
		"1-a.md":                     "a",
		"3-s/.xdocc":                 "layout: other\n",
		"3-s/1-t.md":                 "t",
	})
	b4.compile()
	b4.want("index.html", "[a.html][s/index.html]")
	b4.want("s/index.html", "[OTHER]")

	// a missing file falls back to list.html without an error
	b5 := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		".xdocc":               "layout: nope\n",
		"1-a.md":               "a",
	})
	b5.compile()
	b5.want("index.html", "[a.html]")
}

func TestCompileTemplateArithmetic(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{{ 1 | plus: 2 }}{{ 5 | minus: 3 }}{{ 5 | modulo: 2 }}`,
		"1-a.md":               "a",
	})
	b.compile()
	b.want("index.html", "321")
}

// Symlinking is the default, so a tree whose weight is in its files costs
// almost nothing to generate. Only the root .xdocc turns it off.
func TestCompileSymlinkIsTheDefault(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "a",
		"logo.svg":             "<svg/>",
		"deep/nested/logo.png": "x",
	})
	b.compile()
	link, err := os.Readlink(filepath.Join(b.gen, "logo.svg"))
	if err != nil {
		t.Fatalf("logo.svg is not a symlink: %v", err)
	}
	// the link is relative, so the output tree survives being moved
	if filepath.IsAbs(link) {
		t.Errorf("logo.svg points at the absolute path %q", link)
	}
	if want, _ := filepath.Rel(b.gen, filepath.Join(b.src, "logo.svg")); link != want {
		t.Errorf("logo.svg points at %q, want %q", link, want)
	}
	// it still reads as the file it points at
	b.want("logo.svg", "<svg/>")

	// a nested asset is relative to its own place in the output tree
	link, err = os.Readlink(filepath.Join(b.gen, "deep/nested/logo.png"))
	if err != nil {
		t.Fatalf("deep/nested/logo.png is not a symlink: %v", err)
	}
	if want, _ := filepath.Rel(filepath.Join(b.gen, "deep/nested"), filepath.Join(b.src, "deep/nested/logo.png")); link != want {
		t.Errorf("deep/nested/logo.png points at %q, want %q", link, want)
	}
	b.want("deep/nested/logo.png", "x")
	// a generated page is a real file, never a link
	if info, err := os.Lstat(filepath.Join(b.gen, "a.html")); err != nil {
		t.Fatal(err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("a.html is not a regular file: %v", info.Mode())
	}
}

func TestCompileSymlinkOff(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".xdocc":               "symlink: false\n",
		"logo.svg":             "<svg/>",
	})
	b.compile()
	info, err := os.Lstat(filepath.Join(b.gen, "logo.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("logo.svg is not a copy: %v", info.Mode())
	}
	b.want("logo.svg", "<svg/>")
}

// Switching the setting has to replace what the previous run left behind,
// in both directions.
func TestCompileSymlinkSwitchesBothWays(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".xdocc":               "symlink: false\n",
		"logo.svg":             "<svg/>",
	})
	b.compile()
	b.xdocc(plainSite + "symlink\n")
	b.compile()
	if _, err := os.Readlink(filepath.Join(b.gen, "logo.svg")); err != nil {
		t.Fatalf("the copy was not replaced by a symlink: %v", err)
	}
	b.xdocc(plainSite + "symlink: false\n")
	b.compile()
	info, err := os.Lstat(filepath.Join(b.gen, "logo.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("the symlink was not replaced by a copy: %v", info.Mode())
	}
	b.want("logo.svg", "<svg/>")
}
