package xdocc

import "testing"

// navFiles is the tree the navigation tests share: dir1 and dir2 are in the
// navigation, dir2/subdir3 is not, and it has a nav directory below it.
func navFiles(list string) map[string]string {
	return map[string]string{
		".templates/page.html":                  `{{ data.Content }}`,
		".templates/list.html":                  list,
		"1-test.md":                             "a text file",
		"1-dir1|nav/1-test.md":                  "one",
		"2-dir2|nav/1-test.md":                  "two",
		"2-dir2|nav/2-subdir1|nav/1-test.md":    "three",
		"2-dir2|nav/3-subdir2|nav/1-test.md":    "four",
		"2-dir2|nav/4-subdir3/1-test.md":        "five",
		"2-dir2|nav/4-subdir3/1-sub|nav/1-t.md": "six",
	}
}

func TestGlobalNav(t *testing.T) {
	b := newBuild(t, navFiles(`{% for x in data.GlobalNav %}[{{ x.Path }}({% for c in x.Children %}[{{ c.Path }}]{% endfor %})]{% endfor %}`))
	b.compile()
	const want = "[dir1()][dir2([dir2/subdir1][dir2/subdir2])]"
	b.want("index.html", want)
	b.want("dir2/index.html", want)
	// a directory that is not in the navigation hides its nav children from it
	b.want("dir2/subdir3/index.html", want)
}

func TestBreadcrumb(t *testing.T) {
	b := newBuild(t, navFiles(`{% for x in data.Breadcrumb %}[{{ x.Path }}]{% endfor %}`))
	b.compile()
	b.want("index.html", "")
	b.want("dir2/subdir3/sub/index.html", "[dir2][dir2/subdir3][dir2/subdir3/sub]")
}

// CurrentNav is also how a page reaches the navigation below itself: its
// Children are the nav directories directly inside it.
func TestCurrentNav(t *testing.T) {
	b := newBuild(t, navFiles(`{% if data.CurrentNav %}[{{ data.CurrentNav.Path }}({% for c in data.CurrentNav.Children %}{{ c.Path }} {% endfor %})]{% endif %}`))
	b.compile()
	// the site root has no entry of its own, and neither has a directory
	// outside the navigation
	b.want("index.html", "")
	b.want("dir2/subdir3/index.html", "")
	b.want("dir2/index.html", "[dir2(dir2/subdir1 dir2/subdir2 )]")
	b.want("dir2/subdir1/index.html", "[dir2/subdir1()]")
}

func TestNavActiveAndHref(t *testing.T) {
	b := newBuild(t, navFiles(`{% for x in data.GlobalNav %}[{{ x.Href }} {% if x.Active %}active{% endif %}]{% endfor %}`))
	b.compile()
	b.want("index.html", "[dir1/index.html ][dir2/index.html ]")
	b.want("dir2/index.html", "[../dir1/index.html ][../dir2/index.html active]")
	b.want("dir2/subdir1/index.html", "[../../dir1/index.html ][../../dir2/index.html active]")
}

func TestNavNameFromFilename(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":   `{{ data.Content }}`,
		".templates/list.html":   `{% for x in data.GlobalNav %}[{{ x.Name }}]{% endfor %}`,
		"1-news[News]nav/1-a.md": "a",
		"2-docs|nav/1-a.md":      "a",
	})
	b.compile()
	b.want("index.html", "[News][docs]")
}
