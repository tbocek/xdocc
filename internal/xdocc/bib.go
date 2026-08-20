package xdocc

import (
	"html"
	"strings"
)

// A .bib file is a list of citations, not a document. Every entry becomes one
// <div class="citation">, in the order the file has them, and the file never
// gets a page of its own: it exists to be listed.

// bibEntry is one @type{key, field = {value}, ...} record. Fields holds the
// values as the file wrote them, braces and LaTeX and all, because the braces
// still carry meaning - they are what holds "{Anderson and Sons}" together as
// one author.
type bibEntry struct {
	Type   string
	Key    string
	Fields map[string]string
}

// field returns a field by its lower case name, in the shape a reader wants.
func (e bibEntry) field(name string) string { return cleanBibValue(e.Fields[name]) }

// venue is the publication an entry appeared in, under whichever of the three
// field names the entry happens to use.
func (e bibEntry) venue() string {
	for _, name := range []string{"booktitle", "journal", "howpublished"} {
		if e.Fields[name] != "" {
			return e.field(name)
		}
	}
	return ""
}

// months maps what bib files write to what a reader wants to see.
var months = map[string]string{
	"jan": "January", "feb": "February", "mar": "March", "apr": "April",
	"may": "May", "jun": "June", "jul": "July", "aug": "August",
	"sep": "September", "oct": "October", "nov": "November", "dec": "December",
}

// renderBib turns a .bib file into a citation per entry.
func renderBib(body []byte) string {
	var out strings.Builder
	for _, entry := range parseBib(body) {
		out.WriteString(`<div class="citation">`)
		if authors := bibAuthors(entry.Fields["author"]); authors != "" {
			out.WriteString(html.EscapeString(authors))
			out.WriteString(", ")
		}
		if title := entry.field("title"); title != "" {
			out.WriteString(`"`)
			out.WriteString(html.EscapeString(title))
			out.WriteString(`", `)
		}
		if venue := entry.venue(); venue != "" {
			out.WriteString("<b>")
			out.WriteString(html.EscapeString(venue))
			out.WriteString("</b>; ")
		}
		if month := bibMonth(entry.field("month")); month != "" {
			out.WriteString(html.EscapeString(month))
			out.WriteString(", ")
		}
		// the year is often not a year at all but a patent number or a note
		out.WriteString(html.EscapeString(entry.field("year")))
		out.WriteString(". ")
		if url := entry.field("url"); url != "" {
			out.WriteString(`<a href="`)
			out.WriteString(html.EscapeString(url))
			out.WriteString(`">`)
			out.WriteString(html.EscapeString(url))
			out.WriteString(`</a>`)
		}
		out.WriteString("</div>\n")
	}
	return out.String()
}

// bibMonth expands "aug" to "August" and leaves anything it does not know
// alone, because bib files spell the month in every way there is.
func bibMonth(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	if len(key) > 3 {
		key = key[:3]
	}
	if name, ok := months[key]; ok {
		return name
	}
	return value
}

// bibAuthors turns the "and" separated author list of a bib file into "First
// Last, First Last". It takes the value as the file wrote it: the braces have
// to be split on before they are thrown away.
func bibAuthors(value string) string {
	if value == "" {
		return ""
	}
	var names []string
	for _, name := range splitAuthors(value) {
		name = cleanBibValue(name)
		if name == "" {
			continue
		}
		if last, first, ok := strings.Cut(name, ","); ok {
			// "Niya, Sina Rafati" is one person written back to front
			name = strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// splitAuthors splits on the word "and", but not on an "and" that is part of a
// name protected by braces.
func splitAuthors(value string) []string {
	var (
		parts []string
		start int
		depth int
	)
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case 'a', 'A':
			if depth != 0 || !isWordBoundary(value, i-1) {
				continue
			}
			if i+3 > len(value) || !strings.EqualFold(value[i:i+3], "and") {
				continue
			}
			if !isWordBoundary(value, i+3) {
				continue
			}
			parts = append(parts, value[start:i])
			start = i + 3
			i += 2
		}
	}
	return append(parts, value[start:])
}

// isWordBoundary reports whether the byte at i is absent or a space.
func isWordBoundary(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	return s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r'
}

// parseBib reads the entries of a .bib file. It is deliberately forgiving:
// a file that a stricter parser would reject still yields the entries that are
// well formed, because a citation list is content, not code.
func parseBib(data []byte) []bibEntry {
	var entries []bibEntry
	s := string(data)
	for i := 0; i < len(s); {
		at := strings.IndexByte(s[i:], '@')
		if at < 0 {
			break
		}
		i += at + 1
		entry, next := parseBibEntry(s, i)
		i = next
		if entry != nil {
			entries = append(entries, *entry)
		}
	}
	return entries
}

// parseBibEntry reads one entry, starting just after its "@". It returns the
// entry and the offset to continue at.
func parseBibEntry(s string, i int) (*bibEntry, int) {
	start := i
	for i < len(s) && s[i] != '{' && s[i] != '(' && !isSpaceByte(s[i]) {
		i++
	}
	kind := strings.ToLower(strings.TrimSpace(s[start:i]))
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != '{' {
		return nil, i
	}
	i++ // past the "{"

	// @string, @preamble and @comment are not citations
	if kind == "string" || kind == "preamble" || kind == "comment" {
		return nil, skipBibBraces(s, i)
	}

	entry := bibEntry{Type: kind, Fields: map[string]string{}}
	key := i
	for i < len(s) && s[i] != ',' && s[i] != '}' {
		i++
	}
	entry.Key = strings.TrimSpace(s[key:i])

	for i < len(s) && s[i] != '}' {
		i++ // past the "," that ended the key or the previous field
		for i < len(s) && isSpaceByte(s[i]) {
			i++
		}
		if i >= len(s) || s[i] == '}' {
			break
		}
		name := i
		for i < len(s) && s[i] != '=' && s[i] != ',' && s[i] != '}' {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			// a stray token without a value: skip it and carry on
			continue
		}
		field := strings.ToLower(strings.TrimSpace(s[name:i]))
		i++ // past the "="
		var value string
		value, i = parseBibValue(s, i)
		if field != "" {
			entry.Fields[field] = value
		}
		for i < len(s) && isSpaceByte(s[i]) {
			i++
		}
	}
	if i < len(s) {
		i++ // past the closing "}"
	}
	return &entry, i
}

// parseBibValue reads one field value: {braced}, "quoted" or bare.
func parseBibValue(s string, i int) (string, int) {
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	if i >= len(s) {
		return "", i
	}
	switch s[i] {
	case '{':
		depth, start := 0, i
		for i < len(s) {
			switch s[i] {
			case '{':
				depth++
			case '}':
				depth--
			case '\\':
				i++ // an escaped brace does not count
			}
			i++
			if depth == 0 {
				return strings.TrimSpace(s[start+1 : i-1]), i
			}
		}
		return strings.TrimSpace(s[start+1:]), i
	case '"':
		start := i
		for i++; i < len(s); i++ {
			if s[i] == '\\' {
				i++
				continue
			}
			if s[i] == '"' {
				return strings.TrimSpace(s[start+1 : i]), i + 1
			}
		}
		return strings.TrimSpace(s[start+1:]), i
	default:
		start := i
		for i < len(s) && s[i] != ',' && s[i] != '}' {
			i++
		}
		return strings.TrimSpace(s[start:i]), i
	}
}

// skipBibBraces returns the offset just past the "}" that closes the group
// opened before i.
func skipBibBraces(s string, i int) int {
	depth := 1
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i + 1
			}
		}
	}
	return i
}

// bibCommands are the LaTeX spellings common enough in a bibliography to be
// worth undoing. A command that is not here keeps its backslash: guessing at
// it would lose more than it gains.
var bibCommands = map[string]string{
	"lbrack": "[", "rbrack": "]",
	"ldots": "\u2026", "dots": "\u2026",
	"textendash": "\u2013", "textemdash": "\u2014",
	"LaTeX": "LaTeX", "TeX": "TeX",
}

// cleanBibValue strips the braces that protect capitalisation, undoes the
// escapes a reader should not see, and folds the line breaks a bib file uses
// to stay readable.
func cleanBibValue(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		switch c := value[i]; c {
		case '{', '}':
			// braces protect capitalisation, they are not content
		case '\\':
			if i+1 >= len(value) {
				out.WriteByte(c)
				continue
			}
			if next := value[i+1]; !isASCIILetter(next) {
				// \&, \_, \%, \$, \#, \{ ... : the character itself
				out.WriteByte(next)
				i++
				continue
			}
			name := i + 1
			for i++; i < len(value) && isASCIILetter(value[i]); i++ {
			}
			word := value[name:i]
			replacement, known := bibCommands[word]
			if !known {
				// pass it through rather than guess at it
				out.WriteByte('\\')
				out.WriteString(word)
				i--
				continue
			}
			out.WriteString(replacement)
			// a LaTeX control word swallows the space after it, which is why
			// "\\lbrack Demo" is "[Demo" and not "[ Demo"
			for i < len(value) && (value[i] == ' ' || value[i] == '\t') {
				i++
			}
			i--
		default:
			out.WriteByte(c)
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

// isASCIILetter reports whether b can be part of a LaTeX command name.
func isASCIILetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// isSpaceByte reports whether b is one of the bytes a bib file uses to lay
// itself out.
func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
