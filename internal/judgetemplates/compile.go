package judgetemplates

import (
	"fmt"
	"regexp"
	"strings"
)

var sourceToken = regexp.MustCompile(`(?s)\{\{.*?\}\}|\{%.*?%\}`)
var reference = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
var indexedReference = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.]*)\[([0-9]+)\]$`)
var floatComparison = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.]*)\s*\|\s*float\s*(>=|<=|==|!=|>|<)\s*([A-Za-z_][A-Za-z0-9_.]*)\s*\|\s*float$`)

// CompileSource converts only the source constructs used by the pinned prompt
// snapshot. Unsupported expressions fail explicitly during synchronization.
// It never executes source code and does not implement a general Jinja engine.
func CompileSource(source string) (string, error) {
	var conversionError error
	out := sourceToken.ReplaceAllStringFunc(source, func(token string) string {
		if conversionError != nil {
			return ""
		}
		value := strings.TrimSpace(token[2 : len(token)-2])
		if strings.HasPrefix(token, "{{") {
			if reference.MatchString(value) {
				return "{{ ." + value + " | py }}"
			}
			if m := indexedReference.FindStringSubmatch(value); m != nil {
				return "{{ index ." + m[1] + " " + m[2] + " | py }}"
			}
		} else {
			switch value {
			case "else":
				return "{{ else }}"
			case "endif":
				return "{{ end }}"
			}
			if strings.HasPrefix(value, "if ") {
				condition := strings.TrimSpace(strings.TrimPrefix(value, "if "))
				if reference.MatchString(condition) {
					return "{{ if ." + condition + " }}"
				}
				if strings.HasPrefix(condition, "not ") && reference.MatchString(condition[4:]) {
					return "{{ if not ." + condition[4:] + " }}"
				}
				if m := floatComparison.FindStringSubmatch(condition); m != nil {
					op := map[string]string{">=": "ge", "<=": "le", ">": "gt", "<": "lt", "==": "eq", "!=": "ne"}[m[2]]
					return "{{ if " + op + " (toFloat ." + m[1] + ") (toFloat ." + m[3] + ") }}"
				}
			}
		}
		conversionError = fmt.Errorf("unsupported source template expression %q", token)
		return ""
	})
	if conversionError != nil {
		return "", conversionError
	}
	// Jinja's default environment removes one trailing newline.
	return strings.TrimSuffix(out, "\n"), nil
}
