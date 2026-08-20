package xdocc

import "testing"

// navFiles is the tree the navigation tests share: dir1 and dir2 are in the
// navigation, dir2/subdir3 is not, and it has a nav directory below it.
func navFiles(list string) map[string]string {
	return map[string]string{
		".templates/page.html":                  `{{ .Content }}`,
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
	b := newBuild(t, navFiles(`{{ range .GlobalNav }}[{{ .Path }}({{ range .Children }}[{{ .Path }}]{{ end }})]{{ end }}`))
	b.compile()
	const want = "[dir1()][dir2([dir2/subdir1][dir2/subdir2])]"
	b.want("index.html", want)
	b.want("dir2/index.html", want)
	// a directory that is not in the navigation hides its nav children from it
	b.want("dir2/subdir3/index.html", want)
}

func TestLocalNav(t *testing.T) {
	b := newBuild(t, navFiles(`{{ if not .IsGlobalNav }}{{ range .LocalNav }}[{{ .Path }}]{{ end }}{{ end }}`))
	b.compile()
	b.want("index.html", "")
	b.want("dir2/index.html", "")
	b.want("dir2/subdir3/index.html", "[dir2/subdir3/sub]")
}

func TestBreadcrumb(t *testing.T) {
	b := newBuild(t, navFiles(`{{ range .Breadcrumb }}[{{ .Path }}]{{ end }}`))
	b.compile()
	b.want("index.html", "")
	b.want("dir2/subdir3/sub/index.html", "[dir2][dir2/subdir3][dir2/subdir3/sub]")
}

func TestCurrentNav(t *testing.T) {
	b := newBuild(t, navFiles(`{{ with .CurrentNav }}[{{ .Path }}]{{ end }}`))
	b.compile()
	b.want("index.html", "")
	b.want("dir2/subdir3/index.html", "")
	b.want("dir2/subdir1/index.html", "[dir2/subdir1]")
	b.want("dir2/subdir2/index.html", "[dir2/subdir2]")
}

func TestNavActiveAndHref(t *testing.T) {
	b := newBuild(t, navFiles(`{{ range .GlobalNav }}[{{ .Href }} {{ if .Active }}active{{ end }}]{{ end }}`))
	b.compile()
	b.want("index.html", "[dir1/index.html ][dir2/index.html ]")
	b.want("dir2/index.html", "[../dir1/index.html ][../dir2/index.html active]")
	b.want("dir2/subdir1/index.html", "[../../dir1/index.html ][../../dir2/index.html active]")
}

func TestNavNameFromFilename(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":   `{{ .Content }}`,
		".templates/list.html":   `{{ range .GlobalNav }}[{{ .Name }}]{{ end }}`,
		"1-news[News]nav/1-a.md": "a",
		"2-docs|nav/1-a.md":      "a",
	})
	b.compile()
	b.want("index.html", "[News][docs]")
}
