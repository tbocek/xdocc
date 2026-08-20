package xdocc

import "testing"

func linkFiles(link string) map[string]string {
	return map[string]string{
		".templates/page.html":     `{{ .Content }}`,
		".templates/list.html":     `{{ range .Items }}{{ .Content }}{{ end }}`,
		".templates/markdown.html": `[{{ .URL }}]`,
		".templates/link.html":     `({{ .Content }})`,
		"1-pull.link":              link,
		"2-news[News]/1-a.md":      "a",
		"2-news[News]/2-b.md":      "b",
		"3-other/1-c.md":           "c",
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
	delete(files, "1-pull.link")
	files["3-other/1-pull.link"] = "url=/news/*\n"
	files["3-other/2-up.link"] = "url=../news/*\n"
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
		".templates/page.html":     `{{ .Content }}`,
		".templates/markdown.html": `{{ .Content }}`,
		".templates/list.html":     `{{ (index .ItemsByURL "intro.html").Content }}`,
		"1-intro.md":               "the intro",
		"2-other.md":               "other",
	})
	b.compile()
	b.want("index.html", "<p>the intro</p>")
}
