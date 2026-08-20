package xdocc

import (
	"sort"
	"strconv"
	"strings"
)

// Canonical property names.
const (
	// structural, never inherited
	PropNav     = "nav"
	PropName    = "name"
	PropNoIndex = "noindex"
	PropPromote = "promote"
	PropSplit   = "split"

	// settings, inherited down the tree
	PropLayout  = "layout"
	PropSort    = "sort"
	PropHidden  = "hidden"
	PropVisible = "visible"
	PropCopy    = "copy"

	// site settings, root .xdocc only
	PropSymlink = "symlink"
	PropPost    = "post-processing"

	// set by frontmatter only
	PropDate = "date"
)

// structural properties describe a single item and are never inherited from a
// parent .xdocc.
var structural = map[string]bool{
	PropNav:     true,
	PropName:    true,
	PropNoIndex: true,
	PropPromote: true,
	PropSplit:   true,
}

// site properties are only read from the root .xdocc.
var siteOnly = map[string]bool{
	PropSymlink: true,
	PropPost:    true,
}

// alias maps a legacy spelling to a canonical key, optionally forcing a value.
type alias struct {
	key   string
	value string // "" means: keep the value the user wrote
	fixed bool
}

var aliases = map[string]alias{
	"n":        {key: PropName},
	"l":        {key: PropLayout},
	"hide":     {key: PropHidden},
	"hid":      {key: PropHidden},
	"vis":      {key: PropVisible},
	"cp":       {key: PropCopy},
	"nidx":     {key: PropNoIndex},
	"prm":      {key: PropPromote},
	"prm1":     {key: PropPromote, value: "1", fixed: true},
	"promote1": {key: PropPromote, value: "1", fixed: true},
	"nosplit":  {key: PropSplit, value: "false", fixed: true},
	"page":     {key: PropSplit, value: "false", fixed: true},
	"pag":      {key: PropSplit, value: "false", fixed: true},
	"asc":      {key: PropSort, value: SortAsc, fixed: true},
	"desc":     {key: PropSort, value: SortDesc, fixed: true},
	"dsc":      {key: PropSort, value: SortDesc, fixed: true},
	"pp":       {key: PropPost},
}

// dropped properties are accepted and ignored, so old trees still compile.
var dropped = map[string]bool{
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
	if key == PropSort {
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

// Int reads an integer property.
func (p Props) Int(key string) (int, bool) {
	v, ok := p[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
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
