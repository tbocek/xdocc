package xdocc

import (
	"reflect"
	"testing"
)

func TestLoadXdoccBareWords(t *testing.T) {
	tests := []struct {
		in    string
		props Props
	}{
		{"nosplit\n", Props{PropSplit: "false"}},
		{"# a comment\n\nnosplit\nsymlink\n", Props{PropSplit: "false", PropSymlink: ""}},
		{"split: false\nlayout: wide\n", Props{PropSplit: "false", PropLayout: "wide"}},
		{"symlink: true\n", Props{PropSymlink: ""}},
		{"symlink: false\n", Props{PropSymlink: "false"}},
		{"nav|nosplit\n", Props{PropNav: "", PropSplit: "false"}},
		{"layout=wide\n", Props{PropLayout: "wide"}},
		{"post-processing: rsync -a . host:/var/www\n", Props{PropPost: "rsync -a . host:/var/www"}},
		{"", Props{}},
	}
	for _, tt := range tests {
		props, err := parseYAMLProps([]byte(tt.in))
		if err != nil {
			t.Errorf("%q: %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(props, tt.props) {
			t.Errorf("%q: got %v, want %v", tt.in, props, tt.props)
		}
	}
}

func TestSplitFrontmatter(t *testing.T) {
	in := "---\nname: Challenge Task Winner FS25\ndate: 2025-06-02\nlayout: wide\n---\n#### Hello\ntext\n"
	props, body, err := SplitFrontmatter([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := Props{PropName: "Challenge Task Winner FS25", PropDate: "2025-06-02", PropLayout: "wide"}
	if !reflect.DeepEqual(props, want) {
		t.Errorf("props = %v, want %v", props, want)
	}
	if string(body) != "#### Hello\ntext\n" {
		t.Errorf("body = %q", body)
	}
	if _, ok := ParseDate(props[PropDate]); !ok {
		t.Errorf("date not parsed: %q", props[PropDate])
	}
}

func TestSplitFrontmatterAbsentOrBroken(t *testing.T) {
	for _, in := range []string{
		"# just markdown\n",
		"text\n---\nname: x\n---\n",  // not on the first line
		"---\nname: x\nno closing\n", // no closing fence
		"",
	} {
		props, body, err := SplitFrontmatter([]byte(in))
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if len(props) != 0 {
			t.Errorf("%q: unexpected props %v", in, props)
		}
		if string(body) != in {
			t.Errorf("%q: body = %q", in, body)
		}
	}
}

func TestSplitFrontmatterLongFence(t *testing.T) {
	props, body, err := SplitFrontmatter([]byte("-----\nname: x\n-----\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	if props[PropName] != "x" || string(body) != "body\n" {
		t.Errorf("props = %v, body = %q", props, body)
	}
}
