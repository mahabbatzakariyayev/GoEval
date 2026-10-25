package judge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

type SingleTurnParam string

const (
	Input            SingleTurnParam = "input"
	ActualOutput     SingleTurnParam = "actual_output"
	ExpectedOutput   SingleTurnParam = "expected_output"
	Context          SingleTurnParam = "context"
	RetrievalContext SingleTurnParam = "retrieval_context"
	ToolsCalled      SingleTurnParam = "tools_called"
	ExpectedTools    SingleTurnParam = "expected_tools"
	Metadata         SingleTurnParam = "metadata"
	Tags             SingleTurnParam = "tags"
)

type ToolCall struct {
	Name            string         `json:"name"`
	Type            string         `json:"type,omitempty"` // FUNCTION (default) or MCP.
	Description     string         `json:"description,omitempty"`
	Reasoning       string         `json:"reasoning,omitempty"`
	InputParameters map[string]any `json:"input_parameters,omitempty"`
	Output          any            `json:"output,omitempty"`
}
type Case struct {
	Input            string         `json:"input"`
	ActualOutput     string         `json:"actual_output"`
	ExpectedOutput   string         `json:"expected_output,omitempty"`
	Context          []string       `json:"context,omitempty"`
	RetrievalContext []string       `json:"retrieval_context,omitempty"`
	ToolsCalled      []ToolCall     `json:"tools_called,omitempty"`
	ExpectedTools    []ToolCall     `json:"expected_tools,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Tags             []string       `json:"tags,omitempty"`
	Name             string         `json:"name,omitempty"`
	Trace            map[string]any `json:"trace,omitempty"`
	Multimodal       bool           `json:"multimodal,omitempty"`
	present          map[string]bool
}

var caseAliases = map[string]string{
	"actualOutput": "actual_output", "expectedOutput": "expected_output",
	"retrievalContext": "retrieval_context", "toolsCalled": "tools_called",
	"expectedTools": "expected_tools", "additionalMetadata": "metadata",
	"additional_metadata": "metadata",
}

func normalizeAliases(data []byte, aliases map[string]string) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("judge: expected an object")
	}
	for alias, canonical := range aliases {
		if v, ok := raw[alias]; ok {
			if _, exists := raw[canonical]; exists {
				return nil, fmt.Errorf("judge: both %q and %q supplied", alias, canonical)
			}
			raw[canonical] = v
			delete(raw, alias)
		}
	}
	return raw, nil
}

func (c *ToolCall) UnmarshalJSON(data []byte) error {
	raw, err := normalizeAliases(data, map[string]string{"inputParameters": "input_parameters"})
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	type plain ToolCall
	var v plain
	dec := json.NewDecoder(strings.NewReader(string(encoded)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	if v.Type != "" && v.Type != "FUNCTION" && v.Type != "MCP" {
		return fmt.Errorf("judge: invalid tool type %q", v.Type)
	}
	*c = ToolCall(v)
	return nil
}

// MarshalJSON retains empty collections, which are valid supplied inputs and
// must remain distinct from omitted parameters when datasets are reloaded.
func (c Case) MarshalJSON() ([]byte, error) {
	type plain Case
	b, err := json.Marshal(plain(c))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	put := func(p SingleTurnParam, value any) error {
		encoded, err := json.Marshal(value)
		if err == nil {
			raw[string(p)] = encoded
		}
		return err
	}
	for _, p := range []SingleTurnParam{Input, ActualOutput, ExpectedOutput} {
		if c.present == nil || c.present[string(p)] || c.value(p) != "" {
			if err := put(p, c.value(p)); err != nil {
				return nil, err
			}
		} else {
			delete(raw, string(p))
		}
	}
	for _, p := range []SingleTurnParam{Context, RetrievalContext, ToolsCalled, ExpectedTools, Tags, Metadata} {
		// Interfaces holding nil slices/maps are not nil themselves.
		switch p {
		case Context:
			if c.Context != nil {
				if err := put(p, c.Context); err != nil {
					return nil, err
				}
			}
		case RetrievalContext:
			if c.RetrievalContext != nil {
				if err := put(p, c.RetrievalContext); err != nil {
					return nil, err
				}
			}
		case ToolsCalled:
			if c.ToolsCalled != nil {
				if err := put(p, c.ToolsCalled); err != nil {
					return nil, err
				}
			}
		case ExpectedTools:
			if c.ExpectedTools != nil {
				if err := put(p, c.ExpectedTools); err != nil {
					return nil, err
				}
			}
		case Tags:
			if c.Tags != nil {
				if err := put(p, c.Tags); err != nil {
					return nil, err
				}
			}
		case Metadata:
			if c.Metadata != nil {
				if err := put(p, c.Metadata); err != nil {
					return nil, err
				}
			}
		}
	}
	return json.Marshal(raw)
}

func (c *Case) UnmarshalJSON(data []byte) error {
	raw, err := normalizeAliases(data, caseAliases)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	type plain Case
	var v plain
	dec := json.NewDecoder(strings.NewReader(string(encoded)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	v.present = map[string]bool{}
	for k, val := range raw {
		v.present[k] = !bytes.Equal(bytes.TrimSpace(val), []byte("null"))
	}
	*c = Case(v)
	return nil
}
func (c Case) Validate(params ...SingleTurnParam) error {
	if c.Multimodal {
		return fmt.Errorf("judge: multimodal test cases are not supported yet")
	}
	for _, p := range params {
		ok := false
		switch p {
		case Input:
			ok = c.Input != "" || c.present == nil || c.present[string(p)]
		case ActualOutput:
			ok = c.ActualOutput != "" || c.present == nil || c.present[string(p)]
		case ExpectedOutput:
			ok = c.ExpectedOutput != "" || c.present == nil || c.present[string(p)]
		case Context:
			ok = c.Context != nil
		case RetrievalContext:
			ok = c.RetrievalContext != nil
		case ToolsCalled:
			ok = c.ToolsCalled != nil
		case ExpectedTools:
			ok = c.ExpectedTools != nil
		case Metadata:
			ok = c.Metadata != nil
		case Tags:
			ok = c.Tags != nil
		default:
			return fmt.Errorf("judge: unsupported parameter %q", p)
		}
		if !ok {
			return fmt.Errorf("judge: required parameter %q is missing", p)
		}
	}
	return nil
}
func (c Case) value(p SingleTurnParam) any {
	switch p {
	case Input:
		return c.Input
	case ActualOutput:
		return c.ActualOutput
	case ExpectedOutput:
		return c.ExpectedOutput
	case Context:
		return c.Context
	case RetrievalContext:
		return c.RetrievalContext
	case ToolsCalled:
		return toolCallListRepr(c.ToolsCalled)
	case ExpectedTools:
		return toolCallListRepr(c.ExpectedTools)
	case Metadata:
		return c.Metadata
	case Tags:
		return c.Tags
	}
	return nil
}

const caseMetadataKey = "goeval.judge"

// ToSample maps shared fields and preserves additional case fields in reserved
// metadata. User metadata remains accessible at the top level of Sample.Metadata.
func (c Case) ToSample() goeval.Sample {
	metadata := make(map[string]any, len(c.Metadata)+1)
	for key, value := range c.Metadata {
		metadata[key] = value
	}
	metadata[caseMetadataKey] = c
	s := goeval.Sample{UserInput: c.Input, Response: c.ActualOutput, Reference: c.ExpectedOutput, RetrievedContexts: c.RetrievalContext, ReferenceContexts: c.Context, Metadata: metadata}
	if c.present != nil {
		for p, field := range map[SingleTurnParam]goeval.Field{Input: goeval.UserInput, ActualOutput: goeval.Response, ExpectedOutput: goeval.Reference} {
			if !c.present[string(p)] && c.value(p) == "" {
				s = s.Without(field)
			}
		}
	}
	return s
}
func FromSample(s goeval.Sample) (Case, error) {
	c := Case{}
	if raw, ok := s.Metadata[caseMetadataKey]; ok {
		if original, ok := raw.(Case); ok {
			c = original
		} else {
			b, err := json.Marshal(raw)
			if err != nil {
				return c, err
			}
			if err := json.Unmarshal(b, &c); err != nil {
				return c, err
			}
		}
	} else {
		c.Metadata = s.Metadata
	}
	c.present = map[string]bool{}
	for p, field := range map[SingleTurnParam]goeval.Field{Input: goeval.UserInput, ActualOutput: goeval.Response, ExpectedOutput: goeval.Reference} {
		c.present[string(p)] = s.Validate(field) == nil
	}
	c.Input = s.UserInput
	c.ActualOutput = s.Response
	c.ExpectedOutput = s.Reference
	c.RetrievalContext = s.RetrievedContexts
	c.Context = s.ReferenceContexts
	if s.Messages != nil {
		c.ToolsCalled = []ToolCall{}
		for _, m := range s.Messages {
			if m.Role != "assistant" && m.Role != "ai" {
				continue
			}
			for _, call := range m.ToolCalls {
				c.ToolsCalled = append(c.ToolsCalled, ToolCall{Name: call.Name, InputParameters: call.Arguments})
			}
		}
	}
	if s.ReferenceToolCalls != nil {
		c.ExpectedTools = make([]ToolCall, len(s.ReferenceToolCalls))
		for i, call := range s.ReferenceToolCalls {
			c.ExpectedTools[i] = ToolCall{Name: call.Name, InputParameters: call.Arguments}
		}
	}
	return c, nil
}

type Dataset []Case

func (d Dataset) Samples() goeval.Dataset {
	samples := make(goeval.Dataset, len(d))
	for i, c := range d {
		samples[i] = c.ToSample()
	}
	return samples
}
func ReadJSON(r io.Reader) (Dataset, error) {
	var d Dataset
	dec := json.NewDecoder(r)
	if err := dec.Decode(&d); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("judge: trailing dataset content")
	}
	return d, nil
}
func ReadJSONL(r io.Reader) (Dataset, error) {
	d := Dataset{}
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 64*1024), 16<<20)
	for line := 1; scan.Scan(); line++ {
		if strings.TrimSpace(scan.Text()) == "" {
			continue
		}
		var c Case
		if err := json.Unmarshal(scan.Bytes(), &c); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		d = append(d, c)
	}
	return d, scan.Err()
}
func (d Dataset) WriteJSON(w io.Writer) error { return json.NewEncoder(w).Encode(d) }
func (d Dataset) WriteJSONL(w io.Writer) error {
	enc := json.NewEncoder(w)
	for _, c := range d {
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	return nil
}
