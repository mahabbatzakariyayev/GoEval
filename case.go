package goeval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mahabbatzakariyayev/GoEval/judge"
)

// Tool is an actual or expected agent tool invocation.
type Tool struct {
	Name        string         `json:"name"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Output      any            `json:"output,omitempty"`
	Description string         `json:"description,omitempty"`
	Type        string         `json:"type,omitempty"`
}

// Case is one evaluation example. Context holds retrieved passages; Sources
// holds authoritative material for hallucination checks. Nil tools mean no
// calls. Nil Context or Sources means that evidence has not been supplied.
type Case struct {
	Input         string         `json:"input"`
	Output        string         `json:"output"`
	Expected      string         `json:"expected"`
	Context       []string       `json:"context"`
	Sources       []string       `json:"sources"`
	Tools         []Tool         `json:"tools,omitempty"`
	ExpectedTools []Tool         `json:"expected_tools,omitempty"`
	Name          string         `json:"name,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	present       map[string]bool
}

func (c *Case) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return fmt.Errorf("goeval: case must be a JSON object")
	}
	type plain Case
	var v plain
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	v.present = map[string]bool{}
	for key, value := range raw {
		v.present[key] = !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
	}
	*c = Case(v)
	return nil
}

func (c Case) MarshalJSON() ([]byte, error) {
	type plain Case
	b, err := json.Marshal(plain(c))
	if err != nil {
		return nil, err
	}
	if c.present == nil {
		return b, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	for key, value := range map[string]string{"input": c.Input, "output": c.Output, "expected": c.Expected} {
		if !c.present[key] && value == "" {
			delete(raw, key)
		}
	}
	return json.Marshal(raw)
}

// Sample converts a Case for use with existing datasets and metric packages.
// Nested metadata values and collections remain read-only during a run.
func (c Case) Sample() Sample {
	convert := func(tools []Tool) []judge.ToolCall {
		out := make([]judge.ToolCall, len(tools))
		for i, t := range tools {
			out[i] = judge.ToolCall{Name: t.Name, InputParameters: t.Arguments, Output: t.Output, Description: t.Description, Type: t.Type}
		}
		return out
	}
	s := judge.Case{Input: c.Input, ActualOutput: c.Output, ExpectedOutput: c.Expected, RetrievalContext: c.Context, Context: c.Sources, ToolsCalled: convert(c.Tools), ExpectedTools: convert(c.ExpectedTools), Name: c.Name, Metadata: c.Metadata}.ToSample()
	if c.present != nil {
		for key, field := range map[string]Field{"input": UserInput, "output": Response, "expected": Reference} {
			if !c.present[key] && map[string]string{"input": c.Input, "output": c.Output, "expected": c.Expected}[key] == "" {
				s = s.Without(field)
			}
		}
	}
	return s
}

// Cases can be evaluated directly or saved as JSON/JSONL.
type Cases []Case

// Samples converts friendly cases to the compatibility dataset representation.
func (c Cases) Samples() Dataset {
	out := make(Dataset, len(c))
	for i, row := range c {
		out[i] = row.Sample()
	}
	return out
}

// ReadCases reads a JSON array using GoEval's input/output/expected field names.
func ReadCases(r io.Reader) (Cases, error) {
	var cases Cases
	dec := json.NewDecoder(r)
	if err := dec.Decode(&cases); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("goeval: trailing case data")
	}
	return cases, nil
}

// ReadCasesJSONL reads one case per nonblank line, up to 16 MiB per line.
func ReadCasesJSONL(r io.Reader) (Cases, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 16<<20)
	cases := Cases{}
	for line := 1; s.Scan(); line++ {
		if strings.TrimSpace(s.Text()) == "" {
			continue
		}
		var c Case
		if err := json.Unmarshal(s.Bytes(), &c); err != nil {
			return nil, fmt.Errorf("goeval: line %d: %w", line, err)
		}
		cases = append(cases, c)
	}
	return cases, s.Err()
}
func (c Cases) WriteJSON(w io.Writer) error { return json.NewEncoder(w).Encode(c) }
func (c Cases) WriteJSONL(w io.Writer) error {
	enc := json.NewEncoder(w)
	for _, row := range c {
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}
