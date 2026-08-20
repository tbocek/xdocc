package xdocc

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// IndexURL is the URL of the page that represents a directory.
const IndexURL = "index"

var (
	patternDateTime = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}_\d{2}:\d{2}:\d{2})`)
	patternDate     = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})`)
	patternNumber   = regexp.MustCompile(`^(\d+)`)
)

// knownExtensions are the extensions that select a handler. Everything else is
// an asset. Matching is case insensitive.
var knownExtensions = map[string]string{
	"md":       HandlerMarkdown,
	"markdown": HandlerMarkdown,
	"html":     HandlerHTML,
	"htm":      HandlerHTML,
	"link":     HandlerLink,
	"bib":      HandlerBib,
}

// Handler names.
const (
	HandlerMarkdown = "markdown"
	HandlerHTML     = "html"
	HandlerLink     = "link"
	HandlerBib      = "bib"
	HandlerAsset    = "asset"
)

// Name is the result of parsing a file or directory name.
//
//	1-about[About us]|layout=wide|nav.md
//	│ │      │        │                │
//	│ │      │        │                └── extension, selects the handler
//	│ │      │        └── properties
//	│ │      └── display name
//	│ └── url
//	└── order
type Name struct {
	Raw   string // the name as it is on disk
	Base  string // Raw without the known extensions
	Props Props

	HasOrder bool      // true if an order prefix was found: this is a content item
	Order    int64     // sort key
	Pinned   bool      // "0-" prefix: always sorts first
	Date     time.Time // set for date and datetime prefixes
	HasDate  bool

	URL      string // output name without extension, "index" for the directory page
	Suffix   string // what followed the url in the mandatory part, e.g. ".jpg"
	Title    string // display name
	HasTitle bool

	Ext     string   // the trailing extension chain as written, e.g. ".link.md"
	Exts    []string // the extensions of Ext, rightmost first, lower case
	Handler string   // handler selected by the rightmost extension
}

// ParseName parses a file or directory name.
func ParseName(raw string) Name {
	n := Name{Raw: raw, Props: Props{}, Handler: HandlerAsset}

	n.Base = n.stripExtensions(raw)

	// The mandatory part is everything before the display name and the
	// properties.
	mandatory := n.Base
	firstPipe := strings.IndexByte(n.Base, '|')
	firstBracket := strings.IndexByte(n.Base, '[')
	switch {
	case firstPipe >= 0 && firstBracket >= 0:
		mandatory = n.Base[:min(firstPipe, firstBracket)]
	case firstBracket >= 0:
		mandatory = n.Base[:firstBracket]
	case firstPipe >= 0:
		mandatory = n.Base[:firstPipe]
	}

	if rest, ok := n.parseOrder(mandatory); ok && n.parseURL(rest) {
		n.HasOrder = true
	} else {
		// Not a content item: the whole name is the URL, and it keeps its
		// extension so it can be copied verbatim. "7-.md" lands here too - it
		// has an order but nothing to publish it under, and a name xdocc cannot
		// read is a name xdocc leaves alone.
		n.Date, n.HasDate, n.Order, n.Pinned = time.Time{}, false, 0, false
		n.Suffix = ""
		n.URL = raw
	}

	n.parseTitle(firstBracket)
	n.parseProps(firstPipe, firstBracket)

	if title, ok := n.Props[PropName]; ok && title != "" {
		n.Title = title
		n.HasTitle = true
	}
	if n.HasTitle {
		// however it was spelled, the display name is a property
		n.Props[PropName] = n.Title
	} else if n.HasOrder {
		n.Title = n.URL
	}
	return n
}

// stripExtensions removes the trailing extension chain. The rightmost extension
// selects the handler; further extensions are only stripped while they are known,
// so "1-a.link.md" yields exts [md link] but "1-a.tar.gz" stops at ".gz".
func (n *Name) stripExtensions(name string) string {
	for {
		dot := strings.LastIndexByte(name, '.')
		if dot <= 0 { // no dot, or a dotfile like ".xdocc"
			return name
		}
		ext := name[dot+1:]
		if ext == "" || strings.ContainsAny(ext, "|[]/ ") {
			return name
		}
		lower := strings.ToLower(ext)
		handler, known := knownExtensions[lower]
		if len(n.Exts) == 0 {
			if known {
				n.Handler = handler
			}
		} else if !known {
			// only the rightmost extension may be unknown
			return name
		}
		n.Exts = append(n.Exts, lower)
		n.Ext = name[dot:] + n.Ext
		name = name[:dot]
		if !known {
			return name
		}
	}
}

// parseOrder reads the order prefix and the mandatory "-" that follows it. It
// returns the remainder of the mandatory part.
func (n *Name) parseOrder(mandatory string) (string, bool) {
	var raw string
	switch {
	case patternDateTime.MatchString(mandatory):
		raw = patternDateTime.FindStringSubmatch(mandatory)[1]
		t, err := time.ParseInLocation("2006-01-02_15:04:05", raw, time.Local)
		if err != nil {
			return "", false
		}
		n.Date, n.HasDate = t, true
		n.Order = t.UnixMilli()
	case patternDate.MatchString(mandatory):
		raw = patternDate.FindStringSubmatch(mandatory)[1]
		t, err := time.ParseInLocation("2006-01-02", raw, time.Local)
		if err != nil {
			return "", false
		}
		n.Date, n.HasDate = t, true
		n.Order = t.UnixMilli()
	case patternNumber.MatchString(mandatory):
		raw = patternNumber.FindStringSubmatch(mandatory)[1]
		nr, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return "", false
		}
		n.Order = nr
		n.Pinned = nr == 0
	default:
		return "", false
	}
	// the "-" after the order is mandatory
	rest := mandatory[len(raw):]
	if !strings.HasPrefix(rest, "-") {
		return "", false
	}
	return rest[1:], true
}

// parseURL reads the url, which ends at the first "." or "/". It reports
// whether there was one: an order prefix with no url after it is not a name
// xdocc takes charge of.
func (n *Name) parseURL(rest string) bool {
	if i := strings.IndexAny(rest, "./"); i >= 0 {
		n.Suffix = rest[i:]
		rest = rest[:i]
	}
	if rest == "" {
		return false
	}
	n.URL = rest
	return true
}

// parseTitle reads the display name from "[...]", using the first "[" and the
// last "]".
func (n *Name) parseTitle(firstBracket int) {
	if firstBracket < 0 {
		return
	}
	lastBracket := strings.LastIndexByte(n.Base, ']')
	if lastBracket <= firstBracket {
		return
	}
	n.Title, n.HasTitle = n.Base[firstBracket+1:lastBracket], true
}

// parseProps reads the "|key=value" list. Properties start right after the "]"
// if there is one, otherwise at the first "|".
func (n *Name) parseProps(firstPipe, firstBracket int) {
	start := -1
	if firstBracket >= 0 {
		if lastBracket := strings.LastIndexByte(n.Base, ']'); lastBracket > firstBracket {
			start = lastBracket + 1
		}
	}
	if start < 0 {
		if firstPipe < 0 {
			return
		}
		start = firstPipe + 1
	}
	for _, token := range strings.Split(n.Base[start:], "|") {
		if token = strings.TrimSpace(token); token == "" {
			continue
		}
		key, value, _ := strings.Cut(token, "=")
		n.Props.Set(key, value)
	}
}

// FileName returns the name this item gets in the output tree. Items that are
// transformed lose their extension and become HTML; everything else keeps the
// name it has on disk.
func (n Name) FileName(transformed bool) string {
	switch {
	case !n.HasOrder:
		return n.Raw
	case transformed:
		return n.URL + ".html"
	default:
		return n.URL + n.Suffix + n.Ext
	}
}

// IsIndex reports whether this item is the page of its directory rather than a
// page next to it. "1-index.md" and "7-.md" are. A plain "index.md" has no order
// prefix, so xdocc does not touch it at all - it is copied, and the web server
// serves it as the directory page by itself.
func (n Name) IsIndex() bool {
	return n.HasOrder && n.URL == IndexURL
}
