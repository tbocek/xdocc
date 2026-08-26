package xdocc

import (
	"bytes"
	"log"
	"os"
	"testing"
)

func TestParseShow(t *testing.T) {
	tests := []struct {
		props   Props
		handler string
		want    Show
	}{
		// nothing said: shown everywhere
		{Props{}, HandlerMarkdown, Show{true, true, true}},
		// a .bib is a list of citations, so it asks for no page of its own
		{Props{}, HandlerBib, Show{false, true, true}},
		{Props{PropShow: "page-list-link"}, HandlerBib, Show{true, true, true}},

		// the places are a set: the order they are written in does not matter
		{Props{PropShow: "page-link"}, HandlerMarkdown, Show{true, false, true}},
		{Props{PropShow: "link-page"}, HandlerMarkdown, Show{true, false, true}},
		{Props{PropShow: "list-link"}, HandlerMarkdown, Show{false, true, true}},
		{Props{PropShow: "page"}, HandlerMarkdown, Show{true, false, false}},
		{Props{PropShow: "list"}, HandlerMarkdown, Show{false, true, false}},
		{Props{PropShow: "link"}, HandlerMarkdown, Show{false, false, true}},
		// saying the same place twice is still that place
		{Props{PropShow: "page-page"}, HandlerMarkdown, Show{true, false, false}},
		// a stray separator is not a place
		{Props{PropShow: "page--link"}, HandlerMarkdown, Show{true, false, true}},

		// a bare flag says nothing, so it changes nothing
		{Props{PropShow: ""}, HandlerMarkdown, Show{true, true, true}},
		// and a typo shows the item rather than losing it
		{Props{PropShow: "pgae-link"}, HandlerMarkdown, Show{true, true, true}},
		{Props{PropShow: "nolist"}, HandlerBib, Show{false, true, true}},
	}
	for _, tt := range tests {
		if got := parseShow(tt.props, tt.handler, "1-a.md"); got != tt.want {
			t.Errorf("parseShow(%v, %s) = %+v, want %+v", tt.props, tt.handler, got, tt.want)
		}
	}
}

// A place nobody knows is worth saying out loud: silently showing the item
// everywhere would leave a typo in a filename undiscovered.
func TestParseShowWarnsAboutAnUnknownPlace(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	parseShow(Props{PropShow: "page-lsit"}, HandlerMarkdown, "1-a.md")
	if got := buf.String(); !bytes.Contains(buf.Bytes(), []byte("lsit")) {
		t.Errorf("no warning about the unknown place: %q", got)
	}

	buf.Reset()
	parseShow(Props{PropShow: "page-list"}, HandlerMarkdown, "1-a.md")
	if buf.Len() != 0 {
		t.Errorf("a valid value warned: %q", buf.String())
	}
}

// A directory's own filename wins over the .xdocc inside it, here about the
// pages of the items it holds.
func TestShowFromFilenameBeatsXdocc(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":             `{{ data.Content }}`,
		".templates/list.html":             listTemplate,
		"1-dir|show=link-list-page/.xdocc": "show=list-link\n",
		"1-dir|show=link-list-page/1-a.md": "a",
	})
	b.compile()
	b.want("dir/a.html", "<p>a</p>")
	b.want("dir/index.html", "[dir/a.html]")
}

// What a directory says about pages speaks for the items inside it, and an
// item cannot ask for a page the directory does not hand out.
func TestShowPageOnADirectorySpeaksForItsItems(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html":   `{{ data.Content }}`,
		".templates/list.html":   listTemplate,
		"1-dir/.xdocc":           "show=list-link\n",
		"1-dir/1-a.md":           "a",
		"1-dir/2-b|show=page.md": "b",
	})
	b.compile()

	if b.exists("dir/a.html") {
		t.Error("dir/a.html was written although the directory leaves page out")
	}
	if b.exists("dir/b.html") {
		t.Error("dir/b.html was written although the directory leaves page out")
	}
	// b still keeps itself out of the listing
	b.want("dir/index.html", "[dir/a.html]")
}
