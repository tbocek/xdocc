package xdocc

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseBib(t *testing.T) {
	in := `@inproceedings{RBS18,
address = {Waltham, MA, U.S.A},
author = {Rodrigues, Bruno and Bocek, Thomas and Stiller, Burkhard},
booktitle = {Blockchain Technology: Platforms, Tools and Use Cases},
month = {sep},
pages = {163-198},
title = {The Use of Blockchains: Application-Driven Analysis of Applicability},
url = {https://example.org/paper.pdf},
year = {2018}
}`
	entries := parseBib([]byte(in))
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Type != "inproceedings" || e.Key != "RBS18" {
		t.Errorf("type = %q, key = %q", e.Type, e.Key)
	}
	want := map[string]string{
		"address":   "Waltham, MA, U.S.A",
		"author":    "Rodrigues, Bruno and Bocek, Thomas and Stiller, Burkhard",
		"booktitle": "Blockchain Technology: Platforms, Tools and Use Cases",
		"month":     "sep",
		"pages":     "163-198",
		"title":     "The Use of Blockchains: Application-Driven Analysis of Applicability",
		"url":       "https://example.org/paper.pdf",
		"year":      "2018",
	}
	if !reflect.DeepEqual(e.Fields, want) {
		t.Errorf("fields = %v", e.Fields)
	}
}

// bib files are written by hand and by a dozen tools, so the parser has to
// take what it is given.
func TestParseBibShapes(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		entries int
		check   func(*testing.T, []bibEntry)
	}{
		{
			name:    "quoted values",
			in:      `@article{k, title = "A Title", year = "2020"}`,
			entries: 1,
			check: func(t *testing.T, e []bibEntry) {
				if e[0].field("title") != "A Title" {
					t.Errorf("title = %q", e[0].field("title"))
				}
			},
		},
		{
			name:    "bare value",
			in:      `@article{k, year = 2020, month = jan}`,
			entries: 1,
			check: func(t *testing.T, e []bibEntry) {
				if e[0].field("year") != "2020" || e[0].field("month") != "jan" {
					t.Errorf("fields = %v", e[0].Fields)
				}
			},
		},
		{
			name:    "nested braces protect capitals",
			in:      `@misc{k, title = {The {DHT} of {P}astry}}`,
			entries: 1,
			check: func(t *testing.T, e []bibEntry) {
				if got := e[0].field("title"); got != "The DHT of Pastry" {
					t.Errorf("title = %q", got)
				}
			},
		},
		{
			name:    "value over several lines",
			in:      "@misc{k, title = {One\n  two\n  three}}",
			entries: 1,
			check: func(t *testing.T, e []bibEntry) {
				if got := e[0].field("title"); got != "One two three" {
					t.Errorf("title = %q", got)
				}
			},
		},
		{
			name:    "latex escapes",
			in:      `@misc{k, title = {Research \& Development, 50\% of it}, note = {\lbrack Demo\rbrack}}`,
			entries: 1,
			check: func(t *testing.T, e []bibEntry) {
				if got := e[0].field("title"); got != "Research & Development, 50% of it" {
					t.Errorf("title = %q", got)
				}
				if got := e[0].field("note"); got != "[Demo]" {
					t.Errorf("note = %q", got)
				}
			},
		},
		{
			name:    "an unknown command keeps its backslash",
			in:      `@misc{k, author = {Erd\H{o}s}}`,
			entries: 1,
			check: func(t *testing.T, e []bibEntry) {
				if got := e[0].field("author"); got != `Erd\Hos` {
					t.Errorf("author = %q", got)
				}
			},
		},
		{
			name:    "@string and @comment are not citations",
			in:      "@string{ieee = {IEEE}}\n@comment{ignore me}\n@misc{k, year = {2020}}",
			entries: 1,
			check: func(t *testing.T, e []bibEntry) {
				if e[0].Key != "k" {
					t.Errorf("key = %q", e[0].Key)
				}
			},
		},
		{
			name:    "trailing comma and no final newline",
			in:      `@misc{k, year = {2020},}`,
			entries: 1,
			check:   func(t *testing.T, e []bibEntry) {},
		},
		{
			name:    "several entries",
			in:      "@misc{a, year = {1}}\n\n@misc{b, year = {2}}\n@misc{c, year = {3}}",
			entries: 3,
			check: func(t *testing.T, e []bibEntry) {
				if e[0].Key != "a" || e[1].Key != "b" || e[2].Key != "c" {
					t.Errorf("keys = %q %q %q", e[0].Key, e[1].Key, e[2].Key)
				}
			},
		},
		{
			name:    "an entry that was never closed",
			in:      "@misc{a, year = {1}}\n@misc{b, title = {unfinished",
			entries: 2,
			check:   func(t *testing.T, e []bibEntry) {},
		},
		{
			name:    "not a bib file at all",
			in:      "just some text\nwith an @ in it\n",
			entries: 0,
			check:   func(t *testing.T, e []bibEntry) {},
		},
		{
			name:    "empty",
			in:      "",
			entries: 0,
			check:   func(t *testing.T, e []bibEntry) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := parseBib([]byte(tt.in))
			if len(entries) != tt.entries {
				t.Fatalf("got %d entries, want %d: %v", len(entries), tt.entries, entries)
			}
			tt.check(t, entries)
		})
	}
}

func TestBibAuthors(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Bocek, Thomas", "Thomas Bocek"},
		{"Thomas Bocek", "Thomas Bocek"},
		{"Niya, Sina Rafati", "Sina Rafati Niya"},
		{"Rodrigues, Bruno and Bocek, Thomas", "Bruno Rodrigues, Thomas Bocek"},
		{"Krzysztof Gogol and Christian Killer", "Krzysztof Gogol, Christian Killer"},
		// "and" inside a name, and a name that is one protected block
		{"{Anderson and Sons} and Bocek, Thomas", "Anderson and Sons, Thomas Bocek"},
		{"Alexander Hand", "Alexander Hand"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := bibAuthors(tt.in); got != tt.want {
			t.Errorf("bibAuthors(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBibMonth(t *testing.T) {
	tests := []struct{ in, want string }{
		{"aug", "August"}, {"sep", "September"}, {"January", "January"},
		{"JUN", "June"}, {"", ""}, {"early spring", "early spring"},
	}
	for _, tt := range tests {
		if got := bibMonth(tt.in); got != tt.want {
			t.Errorf("bibMonth(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderBib(t *testing.T) {
	in := `@techreport{RSB22,
	author = {Rodrigues, Bruno and Bocek, Thomas},
	howpublished = {IFI-TecReport No. 2022.05},
	month = {jun},
	title = {FlatFeeStack},
	url = {https://example.org/tr.pdf},
	year = {2022}
}
@misc{BO2024,
	author = {Bocek, Thomas},
	month = {aug},
	title = {Exploring the Use of GenAI},
	url = {genai-2024},
	year = {2024}
}
@misc{plain,
	author = {Krzysztof Gogol and Claudio Tessone},
	title = {SoK: Decentralized Finance},
	year = {2024}
}`
	want := `<div class="citation">Bruno Rodrigues, Thomas Bocek, "FlatFeeStack", ` +
		`<b>IFI-TecReport No. 2022.05</b>; June, 2022. ` +
		`<a href="https://example.org/tr.pdf">https://example.org/tr.pdf</a></div>
<div class="citation">Thomas Bocek, "Exploring the Use of GenAI", August, 2024. ` +
		`<a href="genai-2024">genai-2024</a></div>
<div class="citation">Krzysztof Gogol, Claudio Tessone, "SoK: Decentralized Finance", 2024. </div>
`
	if got := renderBib([]byte(in)); got != want {
		t.Errorf("renderBib:\n got %s\nwant %s", got, want)
	}
}

// The venue is whichever of the three field names the entry happens to use.
func TestRenderBibVenue(t *testing.T) {
	tests := []struct{ in, want string }{
		{`@a{k, booktitle = {Proceedings}, year = {1}}`, "<b>Proceedings</b>; "},
		{`@a{k, journal = {IEEE Software}, year = {1}}`, "<b>IEEE Software</b>; "},
		{`@a{k, howpublished = {A Report}, year = {1}}`, "<b>A Report</b>; "},
		{`@a{k, publisher = {Springer}, year = {1}}`, "1. "},
	}
	for _, tt := range tests {
		if got := renderBib([]byte(tt.in)); !strings.Contains(got, tt.want) {
			t.Errorf("renderBib(%q) = %q, want it to contain %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderBibEscapesHTML(t *testing.T) {
	in := `@misc{k, title = {A <script> and an \&}, url = {https://x/?a=1&b=2}, year = {2020}}`
	got := renderBib([]byte(in))
	if strings.Contains(got, "<script>") {
		t.Errorf("the title was not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt; and an &amp;") {
		t.Errorf("escaping is wrong: %s", got)
	}
	if !strings.Contains(got, `href="https://x/?a=1&amp;b=2"`) {
		t.Errorf("the url was not escaped: %s", got)
	}
}

// A .bib is listed with its citations in it, and gets no page of its own: the
// publication list of a site is many files under one url.
func TestCompileBib(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}<h1>{{ x.Name }}</h1>{{ x.Content }}{% endfor %}`,
		"2023-pub[2023].bib":   `@misc{a, author = {Bocek, Thomas}, title = {Older}, year = {2023}}`,
		"2024-pub[2024].bib":   `@misc{b, author = {Bocek, Thomas}, title = {Newer}, year = {2024}}`,
	})
	b.compile()
	b.want("index.html", `<h1>2023</h1><div class="citation">Thomas Bocek, "Older", 2023. </div>`+
		`<h1>2024</h1><div class="citation">Thomas Bocek, "Newer", 2024. </div>`)

	// no page of its own, and no raw copy under the colliding url either
	if b.exists("pub.html") {
		t.Error("pub.html was written although a .bib is not a document")
	}
	if b.exists("pub.bib") {
		t.Error("pub.bib was copied although the .bib was rendered")
	}
}

// A filename that asks for a page anyway gets one.
func TestCompileBibSplitOverride(t *testing.T) {
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		"1-refs|split.bib":     `@misc{a, title = {One}, year = {2020}}`,
	})
	b.compile()
	b.want("refs.html", `<div class="citation">"One", 2020. </div>`)
}

// A .bib without an order prefix is a file like any other: copied, not read.
func TestCompileBibWithoutOrderPrefix(t *testing.T) {
	const raw = `@misc{a, title = {One}, year = {2020}}`
	b := newBuild(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}[{{ x.URL }}]{% endfor %}`,
		"refs.bib":             raw,
	})
	b.compile()
	b.want("refs.bib", raw)
	b.want("index.html", "")
}

func TestCompileBibIsCached(t *testing.T) {
	b, run := warm(t, map[string]string{
		".templates/page.html": `{{ data.Content }}`,
		".templates/list.html": `{% for x in data.Items %}{{ x.Content }}{% endfor %}`,
		".templates/bib.html":  `{{ data.Content }}`,
		"1-refs.bib":           `@misc{a, title = {One}, year = {2020}}`,
	})
	run()
	b.want("index.html", `<div class="citation">"One", 2020. </div>`)

	run()
	if b.cache.Hits != 1 || b.cache.Misses != 0 {
		t.Errorf("second run: %d hits, %d misses, want 1 and 0", b.cache.Hits, b.cache.Misses)
	}

	b.file("1-refs.bib", `@misc{a, title = {Two}, year = {2021}}`)
	run()
	b.want("index.html", `<div class="citation">"Two", 2021. </div>`)
}

// The real thing: the publication list of dsl.i.ost.ch, if it is checked out.
func TestRenderBibAgainstTheRealFiles(t *testing.T) {
	// read the directory rather than glob it: "[Publications]" is a character
	// class to filepath.Glob and would match nothing here
	dir := filepath.Join("..", "..", "old", "site", "3-pub[Publications]nav")
	dirents, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("old/site is not checked out")
	}
	var names []string
	for _, dirent := range dirents {
		if strings.HasSuffix(dirent.Name(), ".bib") {
			names = append(names, filepath.Join(dir, dirent.Name()))
		}
	}
	if len(names) == 0 {
		t.Skip("old/site has no .bib files")
	}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		entries := parseBib(data)
		if len(entries) == 0 {
			t.Errorf("%s: no entries", filepath.Base(name))
			continue
		}
		for _, e := range entries {
			if e.field("title") == "" {
				t.Errorf("%s: %s has no title", filepath.Base(name), e.Key)
			}
			if e.field("year") == "" {
				t.Errorf("%s: %s has no year", filepath.Base(name), e.Key)
			}
		}
		out := renderBib(data)
		if n := strings.Count(out, `<div class="citation">`); n != len(entries) {
			t.Errorf("%s: %d citations for %d entries", filepath.Base(name), n, len(entries))
		}
		if strings.Contains(out, "{") || strings.Contains(out, "}") {
			t.Errorf("%s: braces survived into the output", filepath.Base(name))
		}
	}
}
