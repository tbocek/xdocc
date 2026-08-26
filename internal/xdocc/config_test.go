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
		{"show=list-link\n", Props{PropShow: "list-link"}},
		{"# a comment\n\nshow=list-link\nsymlink\n", Props{PropShow: "list-link", PropSymlink: ""}},
		{"show: list-link\nlayout: wide\n", Props{PropShow: "list-link", PropLayout: "wide"}},
		{"symlink: true\n", Props{PropSymlink: ""}},
		{"symlink: false\n", Props{PropSymlink: "false"}},
		{"nav|show=list-link\n", Props{PropNav: "", PropShow: "list-link"}},
		{"layout=wide\n", Props{PropLayout: "wide"}},
		// dropped legacy keys are accepted and ignored
		{"post-processing: rsync -a . host:/var/www\n", Props{}},
		{"promote\nvisible\ncopy\n", Props{}},
		{"nosplit\nnolist\nlinkonly\n", Props{}},
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
	in := "---\nname: Challenge Task Winner FS25\nauthor: Someone\nlayout: wide\n---\n#### Hello\ntext\n"
	props, body, err := SplitFrontmatter([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	// keys xdocc does not know are kept, so templates can read them
	want := Props{PropName: "Challenge Task Winner FS25", "author": "Someone", PropLayout: "wide"}
	if !reflect.DeepEqual(props, want) {
		t.Errorf("props = %v, want %v", props, want)
	}
	if string(body) != "#### Hello\ntext\n" {
		t.Errorf("body = %q", body)
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
