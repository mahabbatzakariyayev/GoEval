package prompt

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

// Format renders Python-style {field} placeholders, {{escaped braces}}, !r/!s
// conversions, and dictionary/list selectors. Unsupported format specifications
// return errors so a migrated prompt is never silently changed.
func Format(source string, values map[string]any) (string, error) {
	var out strings.Builder
	for i := 0; i < len(source); {
		if source[i] == '{' {
			if i+1 < len(source) && source[i+1] == '{' {
				out.WriteByte('{')
				i += 2
				continue
			}
			end := strings.IndexByte(source[i+1:], '}')
			if end < 0 {
				return "", fmt.Errorf("prompt: unmatched opening brace")
			}
			end += i + 1
			field := source[i+1 : end]
			if strings.ContainsAny(field, ":{") {
				return "", fmt.Errorf("prompt: unsupported format specification %q", field)
			}
			conversion := ""
			if pos := strings.IndexByte(field, '!'); pos >= 0 {
				conversion = field[pos+1:]
				field = field[:pos]
				if conversion != "r" && conversion != "s" {
					return "", fmt.Errorf("prompt: unsupported conversion %q", conversion)
				}
			}
			value, err := lookup(values, field)
			if err != nil {
				return "", err
			}
			if conversion == "r" {
				out.WriteString(goeval.PythonRepr(value))
			} else {
				out.WriteString(goeval.PythonString(value))
			}
			i = end + 1
			continue
		}
		if source[i] == '}' {
			if i+1 < len(source) && source[i+1] == '}' {
				out.WriteByte('}')
				i += 2
				continue
			}
			return "", fmt.Errorf("prompt: unmatched closing brace")
		}
		out.WriteByte(source[i])
		i++
	}
	return out.String(), nil
}
func lookup(values map[string]any, path string) (any, error) {
	end := strings.IndexAny(path, ".[")
	if end < 0 {
		v, ok := values[path]
		if !ok {
			return nil, fmt.Errorf("prompt: missing input %q", path)
		}
		return v, nil
	}
	value, ok := values[path[:end]]
	if !ok {
		return nil, fmt.Errorf("prompt: missing input %q", path[:end])
	}
	rest := path[end:]
	for rest != "" {
		key := ""
		if rest[0] == '.' {
			rest = rest[1:]
			end = strings.IndexAny(rest, ".[")
			if end < 0 {
				end = len(rest)
			}
			key, rest = rest[:end], rest[end:]
		} else if rest[0] == '[' {
			end = strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, fmt.Errorf("prompt: unmatched selector in %q", path)
			}
			key, rest = rest[1:end], rest[end+1:]
		} else {
			return nil, fmt.Errorf("prompt: invalid selector %q", path)
		}
		v := reflect.ValueOf(value)
		for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
			v = v.Elem()
		}
		if !v.IsValid() {
			return nil, fmt.Errorf("prompt: cannot select %q from nil", key)
		}
		switch v.Kind() {
		case reflect.Map:
			if v.Type().Key().Kind() != reflect.String {
				return nil, fmt.Errorf("prompt: only string dictionary keys are supported")
			}
			entry := v.MapIndex(reflect.ValueOf(key).Convert(v.Type().Key()))
			if !entry.IsValid() {
				return nil, fmt.Errorf("prompt: missing selector %q", key)
			}
			value = entry.Interface()
		case reflect.Slice, reflect.Array:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= v.Len() {
				return nil, fmt.Errorf("prompt: invalid list index %q", key)
			}
			value = v.Index(index).Interface()
		case reflect.Struct:
			field := v.FieldByName(key)
			if !field.IsValid() || !field.CanInterface() {
				return nil, fmt.Errorf("prompt: missing struct field %q", key)
			}
			value = field.Interface()
		default:
			return nil, fmt.Errorf("prompt: cannot select %q", key)
		}
	}
	return value, nil
}
