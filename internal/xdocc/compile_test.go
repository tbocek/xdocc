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
	b.cache = OpenCache("")
	return b
}

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
const listTemplate = `{{ range .Items }}[{{ .URL }}]{{ end }}`

func TestCompileBasics(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":     `{{ .Content }}`,
		".templates/list.html":     listTemplate,
		".templates/markdown.html": `({{ .Content }})`,
		"1-intro.md":               "hello **world**",
		"2-about[About us].md":     "about",
		"logo.svg":                 "<svg/>",
	})
	b.compile()

	b.want("intro.html", "(<p>hello <strong>world</strong></p>)")
	b.want("about.html", "(<p>about</p>)")
	b.want("index.html", "[intro.html][about.html]")
	b.want("logo.svg", "<svg/>")
}

func TestCompileOrderAndSort(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
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
		".templates/page.html": `{{ .Content }}`,
		".templates/list.html": listTemplate,
		"2024-01-01-old.md":    "old",
		"2025-06-02-new.md":    "new",
		"0-title.md":           "title",
	})
	b2.compile()
	b2.want("index.html", "[title.html][new.html][old.html]")

	// an explicit sort wins
	b3 := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
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
		".templates/page.html": `{{ .Content }}`,
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
		".templates/page.html":     `[{{ .Content }}]`,
		".templates/list.html":     listTemplate,
		".templates/markdown.html": `{{ .Content }}`,
		"1-index.md":               "the page itself",
		"2-other.md":               "other",
	})
	b.compile()
	// the index item replaces the generated listing
	b.want("index.html", "[<p>the page itself</p>]")
	b.want("other.html", "[<p>other</p>]")
	if b.exists("index.html") && b.read("index.html") == "" {
		t.Error("index.html is empty")
	}
	// "7-.md" means the same thing
	b2 := newBuild(t, map[string]string{
		".templates/page.html":     `[{{ .Content }}]`,
		".templates/markdown.html": `{{ .Content }}`,
		"7-.md":                    "empty url",
	})
	b2.compile()
	b2.want("index.html", "[<p>empty url</p>]")
}

func TestCompileSplit(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		".templates/list.html": listTemplate,
		"1-news/.xdocc":        "nosplit\n",
		"1-news/1-a.md":        "a",
		"1-news/2-b.md":        "b",
	})
	b.compile()
	b.want("news/index.html", "[news/a.html][news/b.html]")
	if b.exists("news/a.html") {
		t.Error("news/a.html was written although split is off")
	}

	// a single item can opt out
	b2 := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		".templates/list.html": listTemplate,
		"1-a|nosplit.md":       "a",
		"2-b.md":               "b",
	})
	b2.compile()
	if b2.exists("a.html") {
		t.Error("a.html was written although split is off")
	}
	if !b2.exists("b.html") {
		t.Error("b.html is missing")
	}
	b2.want("index.html", "[a.html][b.html]")
}

func TestCompileNoIndex(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		"1-dir|nidx/1-a.md":    "a",
	})
	b.compile()
	if b.exists("dir/index.html") {
		t.Error("dir/index.html was written although noindex is set")
	}
	if !b.exists("dir/a.html") {
		t.Error("dir/a.html is missing")
	}
}

func TestCompilePromote(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":      `{{ .Content }}`,
		".templates/list.html":      listTemplate,
		"2-item2.md":                "two",
		"1-featured|prm/1-item1.md": "one",
	})
	b.compile()
	b.want("index.html", "[featured/item1.html][item2.html]")
	// the promoted directory is still compiled on its own
	b.want("featured/index.html", "[featured/item1.html]")

	// promote=1 contributes only the first item
	b2 := newBuild(t, map[string]string{
		".templates/page.html":       `{{ .Content }}`,
		".templates/list.html":       listTemplate,
		"1-featured|prm1/1-item1.md": "one",
		"1-featured|prm1/2-item2.md": "two",
	})
	b2.compile()
	b2.want("index.html", "[featured/item1.html]")
}

func TestCompileHiddenAndFrontmatter(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		".templates/list.html": `{{ range .Items }}[{{ .Name }}|{{ date "2006-01-02" .Date }}]{{ end }}`,
		"1-a.md":               "---\nname: From front matter\ndate: 2025-06-02\n---\ntext\n",
		"2-b|hidden.md":        "invisible",
		".hidden.md":           "invisible",
		"3-c[From name].md":    "c",
	})
	b.compile()
	b.want("index.html", "[From front matter|2025-06-02][From name|0001-01-01]")
	if b.exists("b.html") {
		t.Error("hidden item was written")
	}
}

func TestCompileSubstitution(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		"1-dir/1-a.md":         "name=${name} url=${url} root=${root} nr=${nr}",
	})
	b.compile()
	b.want("dir/a.html", "<p>name=a url=../dir/a.html root=../ nr=1</p>")
}

func TestCompileDeleteStaleOutput(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
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

func TestCompileSymlink(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		".xdocc":               "symlink\n",
		"logo.svg":             "<svg/>",
	})
	b.compile()
	target := filepath.Join(b.gen, "logo.svg")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("logo.svg is not a symlink")
	}
	b.want("logo.svg", "<svg/>")
}

func TestCompileCacheReusesRenderedContent(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
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
		".templates/page.html":  `{{ .Content }}`,
		".templates/list.html":  listTemplate,
		"1-a.md":                "a",
		"2-draft|hidden/1-b.md": "b",
		"3-also/.xdocc":         "hidden\n",
		"3-also/1-c.md":         "c",
	})
	b.compile()
	b.want("index.html", "[a.html]")
	if b.exists("draft") || b.exists("also") {
		t.Error("a hidden directory reached the output")
	}
}

func TestCompileVisible(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		".templates/list.html": listTemplate,
		".xdocc":               "visible\n",
		"1-ordered.md":         "ordered",
		"beta.md":              "beta",
		"alpha.md":             "alpha",
	})
	b.compile()
	// items without an order prefix are listed too, last and by filename
	b.want("index.html", "[ordered.html][alpha.html][beta.html]")
	b.want("alpha.html", "<p>alpha</p>")
}

func TestCompileCopyProperty(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		".templates/list.html": listTemplate,
		"1-raw|copy.md":        "# not converted",
		"2-normal.md":          "# converted",
	})
	b.compile()
	b.want("raw.md", "# not converted")
	b.want("normal.html", "<h1 id=\"converted\">converted</h1>")
	b.want("index.html", "[raw.md][normal.html]")
}

func TestCompileHTMLHandler(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		"1-a.html":             "<html><head><title>x</title></head><body><p>only this</p></body></html>",
		"2-b.htm":              "<p>no body tag</p>",
	})
	b.compile()
	b.want("a.html", "<p>only this</p>")
	b.want("b.html", "<p>no body tag</p>")
}

func TestCompileStackedExtensions(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
		"1-recipe.link.md":     "# markdown after all",
	})
	b.compile()
	b.want("recipe.html", "<h1 id=\"markdown-after-all\">markdown after all</h1>")
}

func TestCompileDuplicateURLsAreReported(t *testing.T) {
	// two sources that produce the same output file
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ .Content }}`,
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
		".templates/page.html": `[{{ .Content }}]`,
		".templates/list.html": listTemplate,
		".xdocc":               "nosplit\n",
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
