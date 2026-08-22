package xdocc

import "testing"

func TestSettingsInherit(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}[{{ x.Layout }}]{% endfor %}`,
		".xdocc":               "layout: root\n",
		"1-a.md":               "a",
		"2-sub/.xdocc":         "layout: sub\n",
		"2-sub/1-b.md":         "b",
		"2-sub/2-deep/1-c.md":  "c",
		"3-other/1-d.md":       "d",
	})
	b.compile()
	// directories are listed too, with their own resolved settings
	b.want("index.html", "[root][sub][root]")
	b.want("sub/index.html", "[sub][sub]")
	b.want("sub/deep/index.html", "[sub]")
	b.want("other/index.html", "[root]")
}

func TestSettingsPrecedence(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}[{{ x.Name }}/{{ x.Layout }}]{% endfor %}`,
		".xdocc":               "layout: root\n",
		// the filename wins over front matter, which wins over .xdocc
		"1-a|layout=name.md": "---\nlayout: front\n---\na\n",
		"2-b.md":             "---\nlayout: front\nname: From front matter\n---\nb\n",
		"3-c.md":             "c",
	})
	b.compile()
	b.want("index.html", "[a/name][From front matter/front][c/root]")
}

func TestStructuralPropertiesDoNotInherit(t *testing.T) {
	// nav, name and split describe one item and must not leak into the items
	// below it
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.GlobalNav %}[{{ x.Path }}({% for c in x.Children %}[{{ c.Path }}]{% endfor %})]{% endfor %}`,
		"1-dir/.xdocc":         "nav\nname: A directory\nnosplit\npromote\n",
		"1-dir/1-a.md":         "a",
		"1-dir/2-sub/1-b.md":   "b",
	})
	b.compile()
	// the .xdocc marks its own directory, not the one below it
	b.want("dir/sub/index.html", "[dir()]")
	if b.exists("dir/a.html") {
		t.Error("nosplit was ignored: dir/a.html exists")
	}
	if !b.exists("dir/sub/b.html") {
		t.Error("nosplit leaked into dir/sub")
	}
}

func TestDirectoryNameFromXdocc(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.GlobalNav %}[{{ x.Name }}]{% endfor %}`,
		"1-dir/.xdocc":         "nav\nname: A directory\n",
		"1-dir/1-a.md":         "a",
	})
	b.compile()
	b.want("index.html", "[A directory]")
}

func TestFilenameBeatsXdoccOnTheDirectoryItself(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":        `{{ data.Content }}`,
		".templates/list.html":        `{% for x in data.GlobalNav %}[{{ x.Name }}]{% endfor %}`,
		"1-dir[From name]|nav/.xdocc": "name: From xdocc\n",
		"1-dir[From name]|nav/1-a.md": "a",
	})
	b.compile()
	b.want("index.html", "[From name]")
}

func TestSymlinkIsSiteWide(t *testing.T) {
	// symlinking is the default and only the root .xdocc has a say in it
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		"1-dir/.xdocc":         "symlink: false\n",
		"1-dir/logo.svg":       "<svg/>",
	})
	b.compile()
	if !b.site.Symlink() {
		t.Error("symlink was switched off from a directory below the root")
	}

	// the root .xdocc does have a say
	b2 := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".xdocc":               "symlink: false\n",
		"1-dir/logo.svg":       "<svg/>",
	})
	b2.compile()
	if b2.site.Symlink() {
		t.Error("symlink: false in the root .xdocc was ignored")
	}
}

// nosplit says how one directory presents its own items. It must not reach the
// sections below it: the front page of a site is often one long page while its
// sections are not.
func TestSplitIsNotInherited(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": listTemplate,
		".xdocc":               "nosplit\n",
		"1-intro.md":           "intro",
		"2-news/1-first.md":    "first",
	})
	b.compile()

	// the root folds its items into the front page
	if b.exists("intro.html") {
		t.Error("intro.html was written although the root says nosplit")
	}
	// the section below it does not
	b.want("news/first.html", "<p>first</p>")
	b.want("news/index.html", "[news/first.html]")
}
