package xdocc

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

// filler is long enough that a compressed copy is worth writing.
const filler = "The quick brown fox jumps over the lazy dog. " +
	"The quick brown fox jumps over the lazy dog. " +
	"The quick brown fox jumps over the lazy dog. " +
	"The quick brown fox jumps over the lazy dog. " +
	"The quick brown fox jumps over the lazy dog. " +
	"The quick brown fox jumps over the lazy dog. " +
	"The quick brown fox jumps over the lazy dog. " +
	"The quick brown fox jumps over the lazy dog.\n"

// Pages are minified on the way out: that is the default.
func TestMinifyPage(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": "<html>\n  <body>\n    {{ data.Content }}\n  </body>\n</html>\n",
		"1-a.md":               "text",
	})
	b.xdocc("") // nothing set: minifying and compressing are the default
	b.compile()
	// whitespace goes, and so do the tags HTML5 lets you leave out
	b.want("a.html", "<p>text")
}

// A text asset is minified too, which means it is written rather than linked:
// the output is no longer the file the source tree holds.
func TestMinifyAssetIsWrittenNotLinked(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"style.css":            "body {\n    color:  #ffffff;\n}\n",
		"photo.jpg":            "not really a photo",
	})
	b.xdocc("") // nothing set: minifying and compressing are the default
	b.compile()
	b.want("style.css", "body{color:#fff}")
	if info, err := os.Lstat(filepath.Join(b.gen, "style.css")); err != nil {
		t.Fatal(err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("style.css is not a regular file: %v", info.Mode())
	}
	// what has no text in it is still linked, not duplicated
	if _, err := os.Readlink(filepath.Join(b.gen, "photo.jpg")); err != nil {
		t.Errorf("photo.jpg is not a symlink: %v", err)
	}
}

// A file the minifier cannot parse is written as it is instead of failing the
// build.
func TestMinifyFallsBackOnBrokenInput(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"broken.svg":           "<svg><unclosed>",
	})
	b.xdocc("") // nothing set: minifying and compressing are the default
	b.compile()
	b.want("broken.svg", "<svg><unclosed>")
}

func TestMinifyOff(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": "<html>{{ data.Content }}</html>\n",
		"1-a.md":               "text",
		"logo.svg":             "<svg>  </svg>",
	})
	b.xdocc("minify: false\n")
	b.compile()
	b.want("a.html", "<html><p>text</p></html>")
}

// Every text output gets a .gz and a .br beside it, which is what a web server
// serving pre-compressed files looks for.
func TestCompressWritesSidecars(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               filler,
		"notes.txt":            filler,
		"photo.jpg":            filler,
	})
	b.xdocc("") // nothing set: minifying and compressing are the default
	b.compile()

	for _, name := range []string{"a.html", "notes.txt"} {
		want := b.read(name)
		if got := gunzip(t, b.raw(name+".gz")); strings.TrimSpace(got) != want {
			t.Errorf("%s.gz = %q, want %q", name, got, want)
		}
		if got := unbrotli(t, b.raw(name+".br")); strings.TrimSpace(got) != want {
			t.Errorf("%s.br = %q, want %q", name, got, want)
		}
	}
	// a jpeg is compressed already; a second copy of it would only cost space
	if b.exists("photo.jpg.gz") || b.exists("photo.jpg.br") {
		t.Error("a jpeg should not be compressed")
	}
}

// A file too small to gain anything gets no compressed copy: the frame alone
// would cost more than it saves.
// A file with no minifier is still pointed at rather than duplicated; only the
// compressed copies are written beside the link.
func TestCompressKeepsTheLink(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"notes.txt":            filler,
	})
	b.xdocc("")
	b.compile()
	if _, err := os.Readlink(filepath.Join(b.gen, "notes.txt")); err != nil {
		t.Errorf("notes.txt should still be a link: %v", err)
	}
	if got := gunzip(t, b.raw("notes.txt.gz")); got != filler {
		t.Errorf("notes.txt.gz = %q", got)
	}
}

func TestCompressSkipsSmallFiles(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "hi",
	})
	b.xdocc("") // nothing set: minifying and compressing are the default
	b.compile()
	if b.exists("a.html.gz") {
		t.Error("a two-word page should not be compressed")
	}
}

func TestCompressOff(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               filler,
	})
	b.xdocc("compress: false\n")
	b.compile()
	if b.exists("a.html.gz") || b.exists("a.html.br") {
		t.Error("compressing is off, there should be no compressed copies")
	}
}

// A .br left in the source tree next to the file it belongs to is a build
// artefact, not content: xdocc writes that path itself now.
func TestCompressIgnoresSidecarsInTheSource(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"logo.svg":             "<svg>" + filler + "</svg>",
		"logo.svg.br":          "stale",
		"lonely.txt.br":        "kept",
	})
	b.xdocc("") // nothing set: minifying and compressing are the default
	b.compile()
	if got := unbrotli(t, b.raw("logo.svg.br")); !strings.Contains(got, "svg") {
		t.Errorf("logo.svg.br is the stale source copy: %q", got)
	}
	// one without the file it belongs to is just a file, and is passed through
	if !b.exists("lonely.txt.br") {
		t.Error("lonely.txt.br should have been passed through")
	}
}

// The second run of the same process has nothing left to do, and says so
// without reading the output tree back.
func TestResultCountsUnchanged(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               filler,
		"photo.jpg":            "binary",
	})
	b.xdocc("") // nothing set: minifying and compressing are the default
	b.compile()
	first, err := b.site.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if first.Written != 0 || first.Unchanged == 0 {
		t.Errorf("second run: %s, want everything unchanged", first)
	}
	if first.Pages+first.Assets != first.Written+first.Unchanged {
		t.Errorf("the counts do not add up: %s", first)
	}

	b.file("1-a.md", "changed "+filler)
	b.site.Invalidate()
	second, err := b.site.Compile()
	if err != nil {
		t.Fatal(err)
	}
	// the page, the listing that quotes it, and the two compressed copies of
	// each - and nothing else
	if second.Written != 6 {
		t.Errorf("after one change: %s, want 6 written", second)
	}
}

// Touch names the one file that changed, and that is the only one read again.
func TestRefreshRereadsOnlyTheTouchedFile(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "a",
		"2-b.md":               "b",
	})
	b.compile()

	b.file("1-a.md", "a changed")
	b.file("2-b.md", "b changed")
	b.site.Touch(filepath.Join(b.src, "1-a.md"))
	if _, err := b.site.Compile(); err != nil {
		t.Fatal(err)
	}
	b.want("a.html", "<p>a changed</p>")
	b.want("b.html", "<p>b</p>") // never named, never read

	// a full walk catches up with everything
	b.site.Invalidate()
	if _, err := b.site.Compile(); err != nil {
		t.Fatal(err)
	}
	b.want("b.html", "<p>b changed</p>")
}

// A path the tree has never seen cannot be patched in, so it falls back to a
// walk rather than quietly doing nothing.
func TestRefreshFallsBackToAFullWalk(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "a",
	})
	b.compile()
	b.file("2-b.md", "b")
	b.site.Touch(filepath.Join(b.src, "2-b.md"))
	if _, err := b.site.Compile(); err != nil {
		t.Fatal(err)
	}
	b.want("b.html", "<p>b</p>")
}

func TestRescanSetting(t *testing.T) {
	for _, test := range []struct {
		xdocc string
		want  string
	}{
		{"", "10m0s"},
		{"rescan: 30m\n", "30m0s"},
		{"rescan: off\n", "0s"},
		{"rescan: 0\n", "0s"},
		{"rescan: soon\n", "10m0s"}, // not a duration: complain and carry on
	} {
		b := newBuild(t, map[string]string{
			".templates/page.html": `{{ data.Content }}`,
			"1-a.md":               "a",
		})
		b.xdocc(test.xdocc)
		b.compile()
		if got := b.site.Rescan().String(); got != test.want {
			t.Errorf("%q: rescan = %s, want %s", test.xdocc, got, test.want)
		}
	}
}

func (b *build) raw(name string) []byte {
	b.t.Helper()
	data, err := os.ReadFile(filepath.Join(b.gen, filepath.FromSlash(name)))
	if err != nil {
		b.t.Fatalf("reading %s: %v", name, err)
	}
	return data
}

func gunzip(t *testing.T, data []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func unbrotli(t *testing.T, data []byte) string {
	t.Helper()
	out, err := io.ReadAll(brotli.NewReader(bytes.NewReader(data)))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// Switching a site setting has to reach the files that were placed under the
// old one, even though none of them changed.
func TestOutputSettingsSwitchMidRun(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"notes.txt":            filler,
	})
	b.xdocc("compress: false\n")
	b.compile()
	if b.exists("notes.txt.gz") {
		t.Fatal("compressing is off")
	}

	b.xdocc("") // and now it is on
	b.site.Invalidate()
	if _, err := b.site.Compile(); err != nil {
		t.Fatal(err)
	}
	if got := gunzip(t, b.raw("notes.txt.gz")); got != filler {
		t.Errorf("notes.txt.gz = %q", got)
	}

	b.xdocc("compress: false\n") // and off again
	b.site.Invalidate()
	if _, err := b.site.Compile(); err != nil {
		t.Fatal(err)
	}
	if b.exists("notes.txt.gz") || b.exists("notes.txt.br") {
		t.Error("the compressed copies should have been cleaned up")
	}
}

// A .gz or .br left in the source tree is worth saying once. Repeating it on
// every rebuild would bury the log, so it is only said again when the file it
// is derived from has changed.
func TestSidecarInTheSourceIsReportedOnce(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"logo.svg":             "<svg>" + filler + "</svg>",
		"logo.svg.br":          "stale",
	})
	b.xdocc("") // minifying and compressing are the default

	site, err := NewSite(b.src, b.gen)
	if err != nil {
		t.Fatal(err)
	}
	site.SetCache(b.cache)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	said := func() int {
		return strings.Count(buf.String(), "logo.svg.br is generated by xdocc now")
	}
	compile := func() {
		t.Helper()
		site.Invalidate() // a full walk, the way a restarted watcher does it
		if _, err := site.Compile(); err != nil {
			t.Fatal(err)
		}
	}

	compile()
	if said() != 1 {
		t.Fatalf("the first run said it %d times, want 1", said())
	}

	compile()
	compile()
	if said() != 1 {
		t.Errorf("said %d times after three runs, want 1: %s", said(), buf.String())
	}

	// the file it is derived from changed, so it is worth mentioning again
	b.file("logo.svg", "<svg>changed"+filler+"</svg>")
	compile()
	if said() != 2 {
		t.Errorf("said %d times after the svg changed, want 2: %s", said(), buf.String())
	}
}

// However many workers find one, what they found is said in one line at the end
// of the run rather than a line at a time from whichever worker got there
// first, and in a fixed order so two runs of the same tree read the same.
func TestSidecarsInTheSourceAreReportedInOneLine(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"c.svg":                "<svg>" + filler + "</svg>",
		"c.svg.br":             "stale",
		"a.css":                "body{}" + filler,
		"a.css.gz":             "stale",
		"b.js":                 "var x=1;" + filler,
		"b.js.br":              "stale",
	})
	b.xdocc("workers: 4\n") // several at once, to scatter the order they are found in

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	b.compile()

	lines := 0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if strings.Contains(line, "generated by xdocc now") {
			lines++
			want := "3 compressed copies are generated by xdocc now, " +
				"the ones in the source tree are ignored: a.css.gz, b.js.br, c.svg.br"
			if !strings.Contains(line, want) {
				t.Errorf("the line reads %q, want it to contain %q", line, want)
			}
		}
	}
	if lines != 1 {
		t.Errorf("%d lines were said about the copies, want 1: %s", lines, buf.String())
	}
}
