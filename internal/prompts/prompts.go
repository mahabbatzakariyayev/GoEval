// Package prompts embeds upstream prompt text and few-shot examples verbatim.
// The committed JSON snapshot records the pinned source revision.
package prompts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data.json
var data []byte

type Definition struct {
	Instruction string                     `json:"instruction"`
	Examples    json.RawMessage            `json:"examples"`
	Variants    map[string]json.RawMessage `json:"variants"`
	Source      string                     `json:"source"`
}

var bundle struct {
	Revision string                     `json:"revision"`
	Prompts  map[string]json.RawMessage `json:"prompts"`
}

func init() {
	if err := json.Unmarshal(data, &bundle); err != nil {
		panic(err)
	}
}
func Instruction(key string) string { return InstructionVariant(key, "") }
func InstructionVariant(key, variant string) string {
	raw, ok := bundle.Prompts[key]
	if !ok {
		panic("unknown upstream prompt: " + key)
	}
	var p Definition
	if err := json.Unmarshal(raw, &p); err != nil {
		panic(err)
	}
	if variant != "" {
		v, ok := p.Variants[variant]
		if !ok {
			panic("unknown prompt variant: " + variant)
		}
		p.Examples = v
	}
	text := p.Instruction
	if len(p.Examples) > 0 && string(p.Examples) != "[]" {
		var compact bytes.Buffer
		if err := json.Compact(&compact, p.Examples); err != nil {
			panic(err)
		}
		text += "\nExamples (input/output pairs):\n" + compact.String()
	}
	return text
}
func Template(key, name string) string {
	var attrs map[string]json.RawMessage
	if err := json.Unmarshal(bundle.Prompts[key], &attrs); err != nil {
		panic(err)
	}
	var value string
	if err := json.Unmarshal(attrs[name], &value); err != nil {
		panic(fmt.Sprintf("unknown upstream template %s.%s", key, name))
	}
	return value
}
