// Package prompt provides serializable typed prompts and few-shot examples.
package prompt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

type Example[I, O any] struct {
	Input  I `json:"input"`
	Output O `json:"output"`
}
type Prompt[I, O any] struct {
	Name        string          `json:"name"`
	Instruction string          `json:"instruction"`
	Schema      map[string]any  `json:"schema"`
	Examples    []Example[I, O] `json:"examples,omitempty"`
}

func (p Prompt[I, O]) Generate(ctx context.Context, llm goeval.LLM, input I, validate func(O) error) (O, error) {
	var out O
	if strings.TrimSpace(p.Instruction) == "" || p.Schema == nil {
		return out, fmt.Errorf("prompt: instruction and output schema are required")
	}
	instruction := p.Instruction
	if len(p.Examples) > 0 {
		b, err := json.Marshal(p.Examples)
		if err != nil {
			return out, err
		}
		instruction += "\nExamples (input/output pairs):\n" + string(b)
	}
	if err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: instruction, Input: input, Schema: p.Schema}, &out); err != nil {
		return out, err
	}
	if validate != nil {
		if err := validate(out); err != nil {
			return out, fmt.Errorf("prompt: output validation: %w", err)
		}
	}
	return out, nil
}
func (p Prompt[I, O]) WriteJSON(w io.Writer) error { return json.NewEncoder(w).Encode(p) }
func ReadJSON[I, O any](r io.Reader) (Prompt[I, O], error) {
	var p Prompt[I, O]
	dec := json.NewDecoder(r)
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return p, fmt.Errorf("prompt: trailing content")
	}
	if p.Instruction == "" || p.Schema == nil {
		return p, fmt.Errorf("prompt: invalid saved prompt")
	}
	return p, nil
}

// Render uses Go text/template syntax and rejects missing template keys.
func Render(source string, values any) (string, error) {
	t, err := template.New("prompt").Option("missingkey=error").Parse(source)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, values); err != nil {
		return "", err
	}
	return b.String(), nil
}
