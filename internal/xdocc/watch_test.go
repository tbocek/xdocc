package xdocc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// waitFor polls until the condition holds, so the watcher tests do not depend
// on how fast the file system reports a change.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestWatchRecompilesOnChange(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "first",
	})
	site, err := NewSite(b.src, b.gen)
	if err != nil {
		t.Fatal(err)
	}
	site.SetCache(OpenCache(""))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- site.Watch(ctx) }()

	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(b.gen, name))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}

	waitFor(t, "the first build", func() bool { return read("a.html") == "<p>first</p>" })

	b.file("1-a.md", "second")
	waitFor(t, "the change to be picked up", func() bool { return read("a.html") == "<p>second</p>" })

	// a new file in a new directory is picked up as well
	b.file("2-dir/1-b.md", "in a new directory")
	waitFor(t, "the new directory", func() bool { return read("dir/b.html") != "" })

	// and removing a source removes its page
	b.remove("1-a.md")
	waitFor(t, "the removal", func() bool { return read("a.html") == "" })

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWatchPicksUpTemplateChanges(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `[{% if data.Content %}{{ data.Content }}{% endif %}]`,
		"1-a.md":               "text",
	})
	site, err := NewSite(b.src, b.gen)
	if err != nil {
		t.Fatal(err)
	}
	site.SetCache(OpenCache(""))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go site.Watch(ctx)

	read := func() string {
		data, _ := os.ReadFile(filepath.Join(b.gen, "a.html"))
		return strings.ReplaceAll(strings.TrimSpace(string(data)), "\n", "")
	}
	waitFor(t, "the first build", func() bool { return read() == "[<p>text</p>]" })
	b.file(".templates/page.html", `({{ data.Content }})`)
	waitFor(t, "the template change", func() bool { return read() == "(<p>text</p>)" })
}

func TestCachePersistsBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "a",
		"2-b.md":               "b",
	})
	cachePath := filepath.Join(dir, "cache.gob")

	run := func() *Cache {
		cache := OpenCache(cachePath)
		site, err := NewSite(b.src, b.gen)
		if err != nil {
			t.Fatal(err)
		}
		site.SetCache(cache)
		if _, err := site.Compile(); err != nil {
			t.Fatal(err)
		}
		return cache
	}

	if cache := run(); cache.Hits != 0 {
		t.Errorf("first run: %d hits, want 0", cache.Hits)
	}
	if cache := run(); cache.Hits != 2 || cache.Misses != 0 {
		t.Errorf("second run: %d hits, %d misses, want 2 and 0", cache.Hits, cache.Misses)
	}

	// touching one file only invalidates that one
	b.file("1-a.md", "changed")
	cache := run()
	if cache.Hits != 1 || cache.Misses != 1 {
		t.Errorf("third run: %d hits, %d misses, want 1 and 1", cache.Hits, cache.Misses)
	}
	if got := b.read("a.html"); !strings.Contains(got, "changed") {
		t.Errorf("a.html = %q", got)
	}
}

// What one file system event costs: a written file is read again on its own,
// anything that changes the shape of the tree asks for a full walk.
func TestClassifyEvents(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-a.md":               "a",
	})
	b.compile()
	site := b.site

	write := func(name string) fsnotify.Event {
		return fsnotify.Event{Name: filepath.Join(b.src, filepath.FromSlash(name)), Op: fsnotify.Write}
	}
	dirty := func() ([]string, bool) { return site.takeDirty() }

	site.classify(nil, write("1-a.md"))
	if paths, full := dirty(); full || len(paths) != 1 {
		t.Errorf("a written file: %v, full=%v, want just that file", paths, full)
	}

	site.classify(nil, write(XdoccFile))
	if _, full := dirty(); !full {
		t.Error("a changed .xdocc should ask for a full walk")
	}

	site.classify(nil, write(TemplateDir+"/page.html"))
	if _, full := dirty(); !full {
		t.Error("a changed template should ask for a full walk")
	}

	site.classify(nil, fsnotify.Event{Name: filepath.Join(b.src, "2-gone.md"), Op: fsnotify.Remove})
	if _, full := dirty(); !full {
		t.Error("a removed file should ask for a full walk")
	}

	// what a full walk asks for cannot be undone by a single file after it
	site.Invalidate()
	site.classify(nil, write("1-a.md"))
	if _, full := dirty(); !full {
		t.Error("a pending full walk should survive a later write")
	}
}
