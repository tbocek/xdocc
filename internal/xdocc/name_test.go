package xdocc

import (
	"reflect"
	"testing"
	"time"
)

func TestParseNameOrder(t *testing.T) {
	tests := []struct {
		raw      string
		hasOrder bool
		order    int64
		pinned   bool
		url      string
		title    string
		handler  string
		fileName string // output name, transformed where a handler applies
	}{
		// plain content
		{"1-about.md", true, 1, false, "about", "about", HandlerMarkdown, "about.html"},
		{"7-report.markdown", true, 7, false, "report", "report", HandlerMarkdown, "report.html"},
		{"10-x.html", true, 10, false, "x", "x", HandlerHTML, "x.html"},
		{"2-news.link", true, 2, false, "news", "news", HandlerLink, "news.html"},

		// the "-" after the order is mandatory
		{"7.md", false, 0, false, "7.md", "", HandlerMarkdown, "7.md"},
		{"7-.md", false, 0, false, "7-.md", "", HandlerMarkdown, "7-.md"},
		{"label-1.jpg", false, 0, false, "label-1.jpg", "", HandlerAsset, "label-1.jpg"},
		{"logo.svg", false, 0, false, "logo.svg", "", HandlerAsset, "logo.svg"},

		// 0 pins to the front
		{"0-title.md", true, 0, true, "title", "title", HandlerMarkdown, "title.html"},

		// dates
		{"2014-01-01.md", false, 0, false, "2014-01-01.md", "", HandlerMarkdown, "2014-01-01.md"},
		{"2014-01-01-.md", false, 0, false, "2014-01-01-.md", "", HandlerMarkdown, "2014-01-01-.md"},
		{"2014-myurl.md", true, 2014, false, "myurl", "myurl", HandlerMarkdown, "myurl.html"},
		{"2014-01.md", true, 2014, false, "01", "01", HandlerMarkdown, "01.html"},

		// assets with an order keep their extension
		{"1-photo.jpg", true, 1, false, "photo", "photo", HandlerAsset, "photo.jpg"},
		{"1-archive.tar.gz", true, 1, false, "archive", "archive", HandlerAsset, "archive.tar.gz"},

		// directories
		{"3-news[News]nav", true, 3, false, "news", "News", HandlerAsset, "news"},
		{"genai-2024", false, 0, false, "genai-2024", "", HandlerAsset, "genai-2024"},
	}
	for _, tt := range tests {
		n := ParseName(tt.raw)
		if n.HasOrder != tt.hasOrder {
			t.Errorf("%q: HasOrder = %v, want %v", tt.raw, n.HasOrder, tt.hasOrder)
			continue
		}
		if n.HasOrder && !n.HasDate && n.Order != tt.order {
			t.Errorf("%q: Order = %d, want %d", tt.raw, n.Order, tt.order)
		}
		if n.Pinned != tt.pinned {
			t.Errorf("%q: Pinned = %v, want %v", tt.raw, n.Pinned, tt.pinned)
		}
		if n.URL != tt.url {
			t.Errorf("%q: URL = %q, want %q", tt.raw, n.URL, tt.url)
		}
		if n.Title != tt.title {
			t.Errorf("%q: Title = %q, want %q", tt.raw, n.Title, tt.title)
		}
		if n.Handler != tt.handler {
			t.Errorf("%q: Handler = %q, want %q", tt.raw, n.Handler, tt.handler)
		}
		if got := n.FileName(n.Handler != HandlerAsset); got != tt.fileName {
			t.Errorf("%q: FileName = %q, want %q", tt.raw, got, tt.fileName)
		}
	}
}

func TestParseNameDate(t *testing.T) {
	n := ParseName("2025-06-02-winner.md")
	if !n.HasDate {
		t.Fatalf("expected a date")
	}
	want := time.Date(2025, 6, 2, 0, 0, 0, 0, time.Local)
	if !n.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", n.Date, want)
	}
	if n.URL != "winner" {
		t.Errorf("URL = %q", n.URL)
	}

	n = ParseName("2025-06-02_15:30:00-winner.md")
	want = time.Date(2025, 6, 2, 15, 30, 0, 0, time.Local)
	if !n.HasDate || !n.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", n.Date, want)
	}
	if n.URL != "winner" {
		t.Errorf("URL = %q", n.URL)
	}
	if n.Order != want.UnixMilli() {
		t.Errorf("Order = %d, want %d", n.Order, want.UnixMilli())
	}
}

// The same item can be spelled in several ways; all of them must parse to the
// same thing.
func TestParseNameEquivalentSpellings(t *testing.T) {
	spellings := []string{
		"1-url123[Myname]|tag1=x|tag2=y|tag3=z.txt",
		"1-url123[Myname]tag1=x|tag2=y|tag3=z.txt",
		"1-url123|name=Myname|tag1=x|tag2=y|tag3=z.txt",
	}
	want := Props{"name": "Myname", "tag1": "x", "tag2": "y", "tag3": "z"}
	for _, raw := range spellings {
		n := ParseName(raw)
		if !n.HasOrder || n.Order != 1 {
			t.Errorf("%q: order = %d, %v", raw, n.Order, n.HasOrder)
		}
		if n.URL != "url123" {
			t.Errorf("%q: URL = %q", raw, n.URL)
		}
		if n.Title != "Myname" {
			t.Errorf("%q: Title = %q", raw, n.Title)
		}
		if !reflect.DeepEqual(n.Props, want) {
			t.Errorf("%q: Props = %v, want %v", raw, n.Props, want)
		}
		if got := n.FileName(false); got != "url123.txt" {
			t.Errorf("%q: FileName = %q", raw, got)
		}
	}
}

func TestParseNameProps(t *testing.T) {
	tests := []struct {
		raw   string
		props Props
	}{
		// "l" is no alias any more: it read as layout what the old tree
		// meant as limit, so it passes through like any custom property
		{"1-intro|l=4.md", Props{"l": "4"}},
		{"1-news[News]nav", Props{PropNav: "", PropName: "News"}},
		{"0-title|show=list-link.md", Props{PropShow: "list-link"}},
		{"1-index|show=LINK-PAGE.md", Props{PropShow: "link-page"}},
		{"1-gallery|nav|sort=desc|layout=wide.md", Props{PropNav: "", PropSort: SortDesc, PropLayout: "wide"}},
		{"1-a|desc.md", Props{PropSort: SortDesc}},
		{"1-a|hid|noindex|n=x|dsc.md", Props{}},
		// dropped legacy properties are accepted and ignored
		{"1-a|crop=10|content|p=3.md", Props{}},
		{"1-a|prm1|vis|cp|pp=x.md", Props{}},
		{"1-a|split|nosplit|page|nolist|linkonly.md", Props{}},
		// no properties at all
		{"1-a.md", Props{}},
		// a bracket without properties after it
		{"1-a[Some Name].md", Props{PropName: "Some Name"}},
		// properties are case insensitive
		{"1-a|NAV|Sort=Desc.md", Props{PropNav: "", PropSort: SortDesc}},
	}
	for _, tt := range tests {
		n := ParseName(tt.raw)
		if !reflect.DeepEqual(n.Props, tt.props) {
			t.Errorf("%q: Props = %v, want %v", tt.raw, n.Props, tt.props)
		}
	}
}

func TestParseNameExtensions(t *testing.T) {
	tests := []struct {
		raw     string
		exts    []string
		ext     string
		handler string
	}{
		{"1-a.md", []string{"md"}, ".md", HandlerMarkdown},
		{"1-a.MD", []string{"md"}, ".MD", HandlerMarkdown},
		{"1-a.link.md", []string{"md", "link"}, ".link.md", HandlerMarkdown},
		{"1-a.md.jpg", []string{"jpg"}, ".jpg", HandlerAsset},
		{"1-a", nil, "", HandlerAsset},
		{".xdocc", nil, "", HandlerAsset},
		{"1-a|pp=x.md", []string{"md"}, ".md", HandlerMarkdown},
	}
	for _, tt := range tests {
		n := ParseName(tt.raw)
		if !reflect.DeepEqual(n.Exts, tt.exts) {
			t.Errorf("%q: Exts = %v, want %v", tt.raw, n.Exts, tt.exts)
		}
		if n.Ext != tt.ext {
			t.Errorf("%q: Ext = %q, want %q", tt.raw, n.Ext, tt.ext)
		}
		if n.Handler != tt.handler {
			t.Errorf("%q: Handler = %q, want %q", tt.raw, n.Handler, tt.handler)
		}
	}
}

func TestParseNameIndex(t *testing.T) {
	for _, raw := range []string{"1-index.md", "0-index.md", "2025-06-02-index[FS25].md"} {
		if n := ParseName(raw); !n.IsIndex() {
			t.Errorf("%q: expected an index item, url = %q", raw, n.URL)
		}
	}
	// without an order prefix xdocc does not take charge of the file, so
	// "index.md" is copied and not the generated page of its directory, and
	// "7-.md" has an order but no url, which is not a name xdocc reads
	for _, raw := range []string{"1-about.md", "indexes.md", "1-indexed.md", "index.md", "index.html", "7-.md", "0-.md"} {
		if n := ParseName(raw); n.IsIndex() {
			t.Errorf("%q: unexpected index item", raw)
		}
	}
}
