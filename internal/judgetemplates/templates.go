// Package judgetemplates renders compiled multi-stage judge prompts in Go.
package judgetemplates

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"text/template"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

//go:embed data.json
var data []byte
var bundle struct {
	Revision  string `json:"revision"`
	Templates map[string]struct {
		Original string `json:"original"`
		Compiled string `json:"compiled"`
	} `json:"templates"`
	Fragments map[string]string `json:"fragments"`
}
var compiled map[string]*template.Template

func init() {
	if err := json.Unmarshal(data, &bundle); err != nil {
		panic(err)
	}
	compiled = map[string]*template.Template{}
	funcs := template.FuncMap{"py": goeval.PythonString, "toFloat": func(v any) (float64, error) { return strconv.ParseFloat(fmt.Sprint(v), 64) }}
	for key, value := range bundle.Templates {
		t, err := template.New(key).Option("missingkey=error").Funcs(funcs).Parse(value.Compiled)
		if err != nil {
			panic(err)
		}
		compiled[key] = t
	}
}
func Render(key string, values map[string]any) (string, error) {
	t, ok := compiled[key]
	if !ok {
		return "", fmt.Errorf("judge: unsupported prompt %q", key)
	}
	input := map[string]any{"multimodal": false, "_fragments": bundle.Fragments}
	for k, v := range values {
		input[k] = v
	}
	var out bytes.Buffer
	if err := t.Execute(&out, input); err != nil {
		return "", fmt.Errorf("judge: render %s: %w", key, err)
	}
	return out.String(), nil
}
