package xdocc

import "testing"

// A build that fails is the only one that ever saw the changes it was given -
// taking them off the list empties it - so unless it hands them back, the file
// that was written stays unpublished until the rescan comes round, which is
// the same symptom as xdocc not watching at all.
func TestFailedBuildDoesNotSwallowTheChange(t *testing.T) {
	b := newBuild(t, map[string]string{"1-a.md": "# A"})
	b.compile()

	// a file the tree does not know yet: patching it in is not possible, so
	// the refresh falls back to a full walk
	b.file("2-new.md", "# New")
	b.site.Touch(b.src + "/2-new.md")

	// the walk cannot finish without the templates
	b.remove(TemplateDir + "/" + TemplateItem)
	if _, err := b.site.Compile(); err == nil {
		t.Fatal("a build without item.html should fail")
	}

	b.file(TemplateDir+"/"+TemplateItem, fixtureTemplates[TemplateItem])
	if _, err := b.site.Compile(); err != nil {
		t.Fatal(err)
	}
	if !b.exists("new.html") {
		t.Error("new.html is missing: the failed build kept the change to itself")
	}
}
