package xdocc

import (
	"sort"
	"strings"
)

// Canonical property names.
const (
	// structural, never inherited
	PropNav  = "nav"
	PropName = "name"
	PropShow = "show"

	// settings, inherited down the tree
	PropLayout = "layout"
	PropSort   = "sort"

	// site settings, root .xdocc only
	PropSymlink  = "symlink"
	PropMinify   = "minify"
	PropCompress = "compress"
	PropRescan   = "rescan"
)

// structural properties describe a single item and are never inherited from a
// parent .xdocc.
var structural = map[string]bool{
	PropNav:  true,
	PropName: true,
	PropShow: true,
}

// alias maps a legacy spelling to a canonical key, optionally forcing a value.
type alias struct {
	key   string
	value string // "" means: keep the value the user wrote
	fixed bool
}

// One spelling per property, plus the short forms the tree already says:
// "l=" for layout, "asc"/"desc" for sort.
var aliases = map[string]alias{
	"l":    {key: PropLayout},
	"asc":  {key: PropSort, value: SortAsc, fixed: true},
	"desc": {key: PropSort, value: SortDesc, fixed: true},
}

// dropped properties are accepted and ignored, so old trees still compile.
// "copy" and "visible" are spelled by leaving the order prefix off or putting
// it on, "promote" is what a .link file does, "hidden" is what a leading dot
// does, "noindex" is what leaving the order prefix off a directory does, and
// post-processing belongs to whatever starts xdocc.
var dropped = map[string]bool{
	"hidden": true, "hide": true, "hid": true,
	"noindex": true, "nidx": true,
	// short spellings that were only ever noise: one word per property is enough
	"n": true, "pag": true, "dsc": true,
	"copy": true, "cp": true,
	"visible": true, "vis": true,
	"promote": true, "prm": true, "prm1": true, "promote1": true,
	"post-processing": true, "pp": true,
	// the three booleans that "show" replaced: split/nosplit/page said whether
	// an item got a page, nolist and linkonly where it appeared
	"split": true, "nosplit": true, "page": true,
	"nolist": true, "linkonly": true,
	"content": true, "cont": true,
	"paging": true, "p": true,
	"crop":        true,
	"link":        true,
	"dir-command": true, "dir-cmd": true,
	"command-odt": true, "cmd-odt": true,
	"command-docx": true, "cmd-docx": true,
	"command-tex": true, "cmd-tex": true,
	"command-rst": true, "cmd-rst": true,
}

// Sort orders.
const (
	SortAuto = "auto"
	SortAsc  = "asc"
	SortDesc = "desc"
)

// Props is a set of properties. A key present with an empty value is a flag and
// counts as true.
type Props map[string]string

// Set normalises key and value and stores them. Unknown keys are kept as-is so
// templates can read custom properties; dropped legacy keys are discarded.
func (p Props) Set(key, value string) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return
	}
	if dropped[key] {
		return
	}
	if a, ok := aliases[key]; ok {
		key = a.key
		if a.fixed {
			value = a.value
		} else if value == "" && a.value != "" {
			value = a.value
		}
	}
	if key == PropSort || key == PropShow {
		value = strings.ToLower(strings.TrimSpace(value))
	}
	p[key] = value
}

// Has reports whether the key is present.
func (p Props) Has(key string) bool {
	_, ok := p[key]
	return ok
}

// Bool reports the truth value of a key. A flag without a value is true; only an
// explicit false/no/0/off is false.
func (p Props) Bool(key string) (value, ok bool) {
	v, ok := p[key]
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "no", "0", "off":
		return false, true
	}
	return true, true
}

// Keys returns the property names in a stable order.
func (p Props) Keys() []string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// merge copies every key of src that is not already set in p.
func (p Props) merge(src Props, inheritedOnly bool) {
	for k, v := range src {
		if inheritedOnly && structural[k] {
			continue
		}
		if _, exists := p[k]; !exists {
			p[k] = v
		}
	}
}
