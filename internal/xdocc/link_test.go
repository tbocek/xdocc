package xdocc

import "testing"

func linkFiles(link string) map[string]string {
	return map[string]string{
		".templates/page.html":      `{{ data.Content }}`,
		".templates/list.html":      `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/item.html":      `[{{ data.URL }}]`,
		".templates/item-pull.html": `({{ data.Content }})`,
		"1-pull|layout=pull.link":   link,
		"2-news[News]/1-a.md":       "a",
		"2-news[News]/2-b.md":       "b",
		"3-other/1-c.md":            "c",
	}
}

func TestLinkPullDirectoryItems(t *testing.T) {
	b := newBuild(t, linkFiles("url=news/*\n"))
	b.compile()
	b.want("pull.html", "([news/a.html][news/b.html])")
}

func TestLinkLimit(t *testing.T) {
	b := newBuild(t, linkFiles("url=news/*\nlimit=1\n"))
	b.compile()
	b.want("pull.html", "([news/a.html])")
}

func TestLinkPullSingleItem(t *testing.T) {
	b := newBuild(t, linkFiles("url=news/b\n"))
	b.compile()
	b.want("pull.html", "([news/b.html])")
}

func TestLinkPullDirectoryAsListing(t *testing.T) {
	b := newBuild(t, linkFiles("url=news\n"))
	b.compile()
	// a directory is pulled in as its own listing
	b.want("pull.html", "([news/a.html][news/b.html])")
}

func TestLinkSeveralPatterns(t *testing.T) {
	b := newBuild(t, linkFiles("url=news/*\nurl=other/*\n"))
	b.compile()
	b.want("pull.html", "([news/a.html][news/b.html][other/c.html])")
}

func TestLinkRootedAndRelative(t *testing.T) {
	files := linkFiles("url=news/*\n")
	delete(files, "1-pull|layout=pull.link")
	files["3-other/1-pull|layout=pull.link"] = "url=/news/*\n"
	files["3-other/2-up|layout=pull.link"] = "url=../news/*\n"
	b := newBuild(t, files)
	b.compile()
	b.want("other/pull.html", "([news/a.html][news/b.html])")
	b.want("other/up.html", "([news/a.html][news/b.html])")
}

func TestLinkMissingTargetIsEmpty(t *testing.T) {
	b := newBuild(t, linkFiles("url=nowhere/*\n"))
	b.compile()
	b.want("pull.html", "()")
}

func TestLinkItemsByURL(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/item.html": `{{ data.Content }}`,
		".templates/list.html": `{{ (data.ItemsByURL["intro.html"]).Content }}`,
		"1-intro.md":           "the intro",
		"2-other.md":           "other",
	})
	b.compile()
	b.want("index.html", "<p>the intro</p>")
}

// A directory nothing can reach is a source: a .link renders what is in it and
// xdocc writes none of it, not the listing, not the pages, not the files.
func TestLinkToSourceDirWritesNothing(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":         `{{ data.Content }}`,
		".templates/list.html":         `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/item.html":         `{{ data.Content }}`,
		"1-src|show=link/1-b.md":       "B",
		"1-src|show=link/photo.jpg":    "JPEG",
		"1-src|show=link/2-sub/1-c.md": "C",
		"9-p.link":                     "url=src/*\n",
	})
	b.compile()
	// the subdirectory is pulled in as its own listing, and is written no more
	// than its parent is
	b.want("p.html", "<p>B</p><p>C</p>")
	for _, gone := range []string{"src/index.html", "src/index.md", "src/b.html", "src/photo.jpg", "src/sub/index.html"} {
		if b.exists(gone) {
			t.Errorf("a source directory wrote %s", gone)
		}
	}
}

// Turning a published directory into a source one takes its old output with it.
func TestSourceDirCleansUpWhatItPublishedBefore(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/item.html": `{{ data.Content }}`,
		"1-src/1-b.md":         "B",
		"1-src/photo.jpg":      "JPEG",
		"9-p.link":             "url=src/*\n",
	})
	b.compile()
	if !b.exists("src/index.html") || !b.exists("src/photo.jpg") {
		t.Fatal("the first build did not publish the directory")
	}
	b.remove("1-src")
	b.file("1-src|show=link/1-b.md", "B")
	b.file("1-src|show=link/photo.jpg", "JPEG")
	b.compile()
	b.want("p.html", "<p>B</p>")
	for _, gone := range []string{"src/index.html", "src/b.html", "src/photo.jpg"} {
		if b.exists(gone) {
			t.Errorf("%s survived the directory becoming a source", gone)
		}
	}
}
