package xdocc

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWorkersSetting(t *testing.T) {
	def := runtime.NumCPU() + 1
	tests := []struct {
		xdocc string
		want  int
	}{
		{plainSite, def},
		{plainSite + "workers: 1\n", 1},
		{plainSite + "workers: 4\n", 4},
		// a value that is not a count falls back rather than stopping the build
		{plainSite + "workers: many\n", def},
		{plainSite + "workers: 0\n", def},
		{plainSite + "workers: -2\n", def},
	}
	for _, tt := range tests {
		b := newBuild(t, map[string]string{
			".templates/page.html": `{{ data.Content }}`,
			"1-a.md":               "a",
		})
		b.xdocc(tt.xdocc)
		b.compile()
		if got := b.site.Workers(); got != tt.want {
			t.Errorf("%q: Workers() = %d, want %d", tt.xdocc, got, tt.want)
		}
	}
}

// However many workers run, the site has to come out the same. Anything else
// would mean a build could not be reproduced.
func TestWorkersDoNotChangeTheOutput(t *testing.T) {
	files := map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		".templates/item.html": `[{{ data.URL }}]`,
		"1-a.md":               "a",
		"2-b.md":               "b",
		"3-dir/1-c.md":         "c",
		"3-dir/2-d.md":         "d",
		"3-dir/style.css":      "body { color : red }",
		"4-e.html":             "<p>  e  </p>",
	}

	tree := func(workers string) map[string]string {
		b := newBuild(t, files)
		// minify and compress on, so the workers have real work to divide
		b.xdocc("compress\nminify\n" + workers)
		b.compile()

		out := map[string]string{}
		err := filepath.WalkDir(b.gen, func(p string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(b.gen, p)
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	one := tree("workers: 1\n")
	many := tree("workers: 16\n")
	if len(one) == 0 {
		t.Fatal("nothing was generated")
	}
	if len(one) != len(many) {
		t.Fatalf("%d files with one worker, %d with sixteen", len(one), len(many))
	}
	for name, want := range one {
		if got, ok := many[name]; !ok {
			t.Errorf("%s is missing when sixteen workers run", name)
		} else if got != want {
			t.Errorf("%s differs between one worker and sixteen", name)
		}
	}
}

// A run says how many source files it took off the disk, which is the part the
// caches cannot spare it.
func TestResultCountsReads(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		"1-a.md":               "a",
		"2-b.md":               "b",
		"logo.svg":             `<svg xmlns="http://www.w3.org/2000/svg"></svg>`,
	})

	site, err := NewSite(b.src, b.gen)
	if err != nil {
		t.Fatal(err)
	}
	site.SetCache(b.cache)

	// The walk has to read both markdown files: the hash that would tell it
	// they are unchanged is a hash of exactly those bytes. The svg is linked,
	// so it is never opened.
	first, err := site.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if first.Read != 2 {
		t.Errorf("first run read %d sources, want 2", first.Read)
	}

	// Nothing was reported changed, so the tree in memory answers for the whole
	// site and not a single file is opened.
	second, err := site.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if second.Read != 0 {
		t.Errorf("an unchanged run read %d sources, want 0", second.Read)
	}
	if second.Written != 0 {
		t.Errorf("an unchanged run wrote %d files, want 0", second.Written)
	}

	// A full walk reads both again: the hash that decides a cache hit is a hash
	// of the bytes, so the bytes have to be there to decide it.
	site.Invalidate()
	full, err := site.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if full.Read != 2 {
		t.Errorf("a full walk read %d sources, want 2", full.Read)
	}
	if full.Written != 0 {
		t.Errorf("a full walk wrote %d files, want 0", full.Written)
	}

	// After a change to one file, only that file is read.
	b.file("1-a.md", "changed")
	site.Touch(filepath.Join(b.src, "1-a.md"))
	third, err := site.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if third.Read != 1 {
		t.Errorf("after one change %d sources were read, want 1", third.Read)
	}
}
