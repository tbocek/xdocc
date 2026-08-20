package xdocc

import (
	"bytes"
	"html/template"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.Footnote, extension.DefinitionList),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// renderMarkdown turns markdown into HTML.
func renderMarkdown(body []byte) (template.HTML, error) {
	var buf bytes.Buffer
	if err := markdown.Convert(body, &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

var bodyTag = regexp.MustCompile(`(?is)<body[^>]*>(.*)</body>`)

// renderHTML takes the body of an HTML file, or the whole file if it has no
// body tag.
func renderHTML(body []byte) template.HTML {
	if match := bodyTag.FindSubmatch(body); match != nil {
		return template.HTML(match[1])
	}
	return template.HTML(body)
}

// linkSpec is what a .link file asks for.
type linkSpec struct {
	patterns []string
	limit    int
}

// parseLink reads a .link file: "url=" lines, one per pattern, and an optional
// "limit=".
func parseLink(body []byte) linkSpec {
	spec := linkSpec{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			// a bare line is a pattern
			spec.patterns = append(spec.patterns, line)
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "url", "urls", "link":
			for _, pattern := range strings.Split(value, ",") {
				if pattern = strings.TrimSpace(pattern); pattern != "" {
					spec.patterns = append(spec.patterns, pattern)
				}
			}
		case "limit", "max":
			if n, err := strconv.Atoi(value); err == nil {
				spec.limit = n
			}
		}
	}
	return spec
}

// resolveLink finds the items a pattern points at. Patterns are relative to
// from, "/" starts at the site root, ".." goes up, and "*" stands for every
// content item of a directory.
func (s *Site) resolveLink(pattern string, from *Item) []*Item {
	dir := from
	if strings.HasPrefix(pattern, "/") {
		dir = s.Root
		pattern = strings.TrimPrefix(pattern, "/")
	}
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		last := i == len(segments)-1
		switch segment {
		case "", ".":
			continue
		case "..":
			if dir.Parent != nil {
				dir = dir.Parent
			}
		case "*":
			return dir.ContentItems()
		default:
			child := findChild(dir, segment)
			if child == nil {
				return nil
			}
			if last {
				return []*Item{child}
			}
			if !child.IsDir {
				return nil
			}
			dir = child
		}
	}
	return []*Item{dir}
}

// findChild looks for a child by url segment or by its name on disk.
func findChild(dir *Item, segment string) *Item {
	for _, child := range dir.Children {
		if child.FileName == segment {
			return child
		}
		if child.name.HasOrder && child.name.URL == segment {
			return child
		}
		if !child.IsDir && strings.TrimSuffix(child.URL[len(child.Dir):], ".html") == segment {
			return child
		}
	}
	return nil
}
