package docsite

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var numbered = regexp.MustCompile(`^\d+\.\s+`)
var tableRule = regexp.MustCompile(`^:?-{3,}:?$`)

type linkResolver func(string) (string, error)

func slug(text string) string {
	var out strings.Builder
	separator := false
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if separator && out.Len() > 0 {
				out.WriteByte('-')
			}
			out.WriteRune(r)
			separator = false
		} else if r != '`' {
			separator = true
		}
	}
	if out.Len() == 0 {
		return "section"
	}
	return out.String()
}

func inline(text string, resolve linkResolver) (string, error) {
	var out strings.Builder
	for len(text) > 0 {
		if text[0] == '`' {
			if end := strings.IndexByte(text[1:], '`'); end >= 0 {
				out.WriteString("<code>" + html.EscapeString(text[1:end+1]) + "</code>")
				text = text[end+2:]
				continue
			}
		}
		if strings.HasPrefix(text, "**") {
			if end := strings.Index(text[2:], "**"); end >= 0 {
				value, err := inline(text[2:end+2], resolve)
				if err != nil {
					return "", err
				}
				out.WriteString("<strong>" + value + "</strong>")
				text = text[end+4:]
				continue
			}
		}
		if text[0] == '[' {
			if middle := strings.Index(text, "]("); middle > 0 {
				if end := strings.IndexByte(text[middle+2:], ')'); end >= 0 {
					target, err := resolve(text[middle+2 : middle+2+end])
					if err != nil {
						return "", err
					}
					label, err := inline(text[1:middle], resolve)
					if err != nil {
						return "", err
					}
					out.WriteString(`<a href="` + html.EscapeString(target) + `">` + label + `</a>`)
					text = text[middle+end+3:]
					continue
				}
			}
		}
		out.WriteString(html.EscapeString(text[:1]))
		text = text[1:]
	}
	return out.String(), nil
}

func tableCells(line string) []string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
func isTableRule(line string) bool {
	if !strings.Contains(line, "|") {
		return false
	}
	for _, cell := range tableCells(line) {
		if !tableRule.MatchString(cell) {
			return false
		}
	}
	return true
}
func headingLine(line string) (int, string) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level > 0 && level <= 6 && len(line) > level && line[level] == ' ' {
		return level, strings.TrimSpace(line[level:])
	}
	return 0, ""
}
func listLine(line string) (string, string) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return "ul", line[2:]
	}
	if prefix := numbered.FindString(line); prefix != "" {
		return "ol", line[len(prefix):]
	}
	return "", ""
}

func renderMarkdown(markdown string, resolve linkResolver) (string, []Heading, error) {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	var out strings.Builder
	headings := []Heading{}
	ids := map[string]int{}
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}
		if strings.HasPrefix(line, "```") {
			language := strings.TrimSpace(strings.TrimPrefix(line, "```"))
			i++
			start := i
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				i++
			}
			if i == len(lines) {
				return "", nil, fmt.Errorf("unclosed code fence")
			}
			code := strings.Join(lines[start:i], "\n")
			out.WriteString(`<div class="code-block"><div class="code-toolbar"><span>` + html.EscapeString(language) + `</span><button type="button" class="copy-button" aria-label="Copy code">Copy</button></div><pre tabindex="0"><code class="language-` + html.EscapeString(language) + `">` + html.EscapeString(code) + `</code></pre></div>`)
			i++
			continue
		}
		if level, text := headingLine(line); level > 0 {
			id := slug(text)
			ids[id]++
			if ids[id] > 1 {
				id += "-" + strconv.Itoa(ids[id])
			}
			value, err := inline(text, resolve)
			if err != nil {
				return "", nil, err
			}
			fmt.Fprintf(&out, `<h%d id="%s">%s<a class="heading-anchor" href="#%s" aria-label="Link to %s">#</a></h%d>`, level, id, value, id, html.EscapeString(text), level)
			if level == 2 || level == 3 {
				headings = append(headings, Heading{Text: text, ID: id, Level: level})
			}
			i++
			continue
		}
		if i+1 < len(lines) && isTableRule(strings.TrimSpace(lines[i+1])) {
			header := tableCells(line)
			out.WriteString(`<div class="table-wrap"><table><thead><tr>`)
			for _, cell := range header {
				value, err := inline(cell, resolve)
				if err != nil {
					return "", nil, err
				}
				out.WriteString(`<th scope="col">` + value + `</th>`)
			}
			out.WriteString(`</tr></thead><tbody>`)
			i += 2
			for i < len(lines) && strings.Contains(lines[i], "|") && strings.TrimSpace(lines[i]) != "" {
				cells := tableCells(lines[i])
				if len(cells) != len(header) {
					return "", nil, fmt.Errorf("table column count differs at line %d", i+1)
				}
				out.WriteString("<tr>")
				for _, cell := range cells {
					value, err := inline(cell, resolve)
					if err != nil {
						return "", nil, err
					}
					out.WriteString("<td>" + value + "</td>")
				}
				out.WriteString("</tr>")
				i++
			}
			out.WriteString("</tbody></table></div>")
			continue
		}
		if kind, _ := listLine(line); kind != "" {
			out.WriteString("<" + kind + ">")
			for i < len(lines) {
				itemKind, text := listLine(lines[i])
				if itemKind != kind {
					break
				}
				i++
				for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
					if k, _ := listLine(lines[i]); k != "" {
						break
					}
					if level, _ := headingLine(strings.TrimSpace(lines[i])); level > 0 {
						break
					}
					if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
						break
					}
					text += " " + strings.TrimSpace(lines[i])
					i++
				}
				value, err := inline(text, resolve)
				if err != nil {
					return "", nil, err
				}
				out.WriteString("<li>" + value + "</li>")
			}
			out.WriteString("</" + kind + ">")
			continue
		}
		if strings.HasPrefix(line, "> ") {
			text := strings.TrimPrefix(line, "> ")
			i++
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "> ") {
				text += " " + strings.TrimPrefix(strings.TrimSpace(lines[i]), "> ")
				i++
			}
			value, err := inline(text, resolve)
			if err != nil {
				return "", nil, err
			}
			out.WriteString(`<aside class="callout">` + value + `</aside>`)
			continue
		}
		text := line
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			next := strings.TrimSpace(lines[i])
			level, _ := headingLine(next)
			kind, _ := listLine(next)
			if level > 0 || kind != "" || strings.HasPrefix(next, "```") || strings.HasPrefix(next, "> ") {
				break
			}
			text += " " + next
			i++
		}
		value, err := inline(text, resolve)
		if err != nil {
			return "", nil, err
		}
		out.WriteString("<p>" + value + "</p>")
	}
	return out.String(), headings, nil
}
