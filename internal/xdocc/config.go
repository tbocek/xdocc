package xdocc

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// XdoccFile is the per-directory settings file.
const XdoccFile = ".xdocc"

// TemplateDir holds the templates of a site.
const TemplateDir = ".templates"

// LoadXdocc reads a .xdocc file. It is YAML, with one extra convenience: a line
// that is a bare word is taken as a flag, so "nosplit" means "split: false".
// A missing file is not an error.
func LoadXdocc(path string) (Props, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Props{}, nil
	}
	if err != nil {
		return nil, err
	}
	props, err := parseYAMLProps(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return props, nil
}

// parseYAMLProps parses YAML into properties, accepting bare words as flags.
func parseYAMLProps(data []byte) (Props, error) {
	props := Props{}
	var out bytes.Buffer
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// A bare word is a flag: not indented, not "key: value", not a list
		// item. It may carry a value ("layout=wide") and several may share a
		// line, separated like in a filename ("nav|nosplit").
		if trimmed == line && !strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
			for _, token := range strings.Split(trimmed, "|") {
				if token = strings.TrimSpace(token); token != "" {
					key, value, _ := strings.Cut(token, "=")
					props.Set(key, value)
				}
			}
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if out.Len() == 0 {
		return props, nil
	}
	var raw map[string]any
	if err := yaml.Unmarshal(out.Bytes(), &raw); err != nil {
		return nil, err
	}
	for key, value := range raw {
		props.Set(key, scalar(value))
	}
	return props, nil
}

// scalar renders a YAML value as the string xdocc works with. A bare flag
// ("nav") and "nav: true" both end up as an empty value, which reads as true.
func scalar(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case bool:
		if v {
			return ""
		}
		return "false"
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case time.Time:
		// yaml resolves dates for us; render them back the way they were
		// written so the wall clock survives
		if v.Hour() == 0 && v.Minute() == 0 && v.Second() == 0 {
			return v.Format("2006-01-02")
		}
		return v.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprint(v)
	}
}

// SplitFrontmatter separates an optional YAML front matter block from the body.
// The block is fenced by lines of three or more dashes and must start on the
// first line.
func SplitFrontmatter(data []byte) (Props, []byte, error) {
	text := string(data)
	if !isFence(firstLine(text)) {
		return Props{}, data, nil
	}
	_, rest, ok := strings.Cut(text, "\n")
	if !ok {
		return Props{}, data, nil
	}
	var block strings.Builder
	for {
		line, remainder, ok := strings.Cut(rest, "\n")
		if isFence(line) {
			props, err := parseYAMLProps([]byte(block.String()))
			if err != nil {
				return nil, nil, fmt.Errorf("front matter: %w", err)
			}
			return props, []byte(remainder), nil
		}
		if !ok {
			// no closing fence: there is no front matter after all
			return Props{}, data, nil
		}
		block.WriteString(line)
		block.WriteByte('\n')
		rest = remainder
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

func isFence(line string) bool {
	line = strings.TrimRight(strings.TrimSpace(line), "\r")
	if len(line) < 3 {
		return false
	}
	return strings.Trim(line, "-") == ""
}
