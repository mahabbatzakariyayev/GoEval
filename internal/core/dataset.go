package core

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Field string

const (
	UserInput           Field = "user_input"
	Response            Field = "response"
	Reference           Field = "reference"
	RetrievedContexts   Field = "retrieved_contexts"
	ReferenceContexts   Field = "reference_contexts"
	RetrievedContextIDs Field = "retrieved_context_ids"
	ReferenceContextIDs Field = "reference_context_ids"
	Messages            Field = "messages"
	ReferenceToolCalls  Field = "reference_tool_calls"
	ReferenceTopics     Field = "reference_topics"
	Rubrics             Field = "rubrics"
)

type ToolCall struct {
	Name               string         `json:"name"`
	Arguments          map[string]any `json:"arguments,omitempty"`
	rawArguments       json.RawMessage
	canonicalArguments string
	rawValues          map[string]json.RawMessage
	canonicalValues    map[string]string
}
type Message struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Sample uses upstream single-turn field names. Messages represents a multi-turn
// conversation; ReferenceToolCalls represents its expected tool sequence.
type Sample struct {
	present             map[string]bool
	UserInput           string            `json:"user_input,omitempty"`
	Response            string            `json:"response,omitempty"`
	Reference           string            `json:"reference,omitempty"`
	RetrievedContexts   []string          `json:"retrieved_contexts,omitempty"`
	ReferenceContexts   []string          `json:"reference_contexts,omitempty"`
	RetrievedContextIDs []string          `json:"retrieved_context_ids,omitempty"`
	ReferenceContextIDs []string          `json:"reference_context_ids,omitempty"`
	Messages            []Message         `json:"messages,omitempty"`
	ReferenceToolCalls  []ToolCall        `json:"reference_tool_calls,omitempty"`
	ReferenceTopics     []string          `json:"reference_topics,omitempty"`
	Rubrics             map[string]string `json:"rubrics,omitempty"`
	Metadata            map[string]any    `json:"metadata,omitempty"`
}
type SingleTurnSample = Sample
type MultiTurnSample = Sample
type Dataset []Sample
type EvaluationDataset = Dataset

func (s Sample) Validate(fields ...Field) error {
	if s.UserInput != "" && s.Messages != nil {
		return fmt.Errorf("goeval: cannot combine scalar user_input and messages")
	}
	for _, f := range fields {
		present := false
		switch f {
		case UserInput:
			present = s.UserInput != "" || s.present == nil || s.present[string(UserInput)]
		case Response:
			present = s.Response != "" || s.present == nil || s.present[string(Response)]
		case Reference:
			present = s.Reference != "" || s.present == nil || s.present[string(Reference)]
		case RetrievedContexts:
			present = s.RetrievedContexts != nil
		case ReferenceContexts:
			present = s.ReferenceContexts != nil
		case RetrievedContextIDs:
			present = s.RetrievedContextIDs != nil
		case ReferenceContextIDs:
			present = s.ReferenceContextIDs != nil
		case Messages:
			present = len(s.Messages) > 0
		case ReferenceToolCalls:
			present = s.ReferenceToolCalls != nil
		case ReferenceTopics:
			present = s.ReferenceTopics != nil
		case Rubrics:
			present = len(s.Rubrics) > 0
		default:
			return fmt.Errorf("goeval: unknown required field %q", f)
		}
		if !present {
			return &MissingFieldError{Field: f}
		}
	}
	return nil
}

func ReadJSON(r io.Reader) (Dataset, error) {
	var d Dataset
	dec := json.NewDecoder(r)
	if err := dec.Decode(&d); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("goeval: trailing dataset content")
	}
	return d, nil
}
func (d Dataset) WriteJSON(w io.Writer) error { return json.NewEncoder(w).Encode(d) }
func ReadJSONL(r io.Reader) (Dataset, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 16*1024*1024)
	d := Dataset{}
	for line := 1; s.Scan(); line++ {
		if strings.TrimSpace(s.Text()) == "" {
			continue
		}
		var row Sample
		if err := json.Unmarshal(s.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		d = append(d, row)
	}
	return d, s.Err()
}
func (d Dataset) WriteJSONL(w io.Writer) error {
	e := json.NewEncoder(w)
	for _, s := range d {
		if err := e.Encode(s); err != nil {
			return err
		}
	}
	return nil
}

// CSV stores lists, rubrics, and messages as JSON cells. ColumnMap maps input
// header names to canonical GoEval field names (e.g. question -> user_input).
func ReadCSV(r io.Reader, columnMap map[string]string) (Dataset, error) {
	reader := csv.NewReader(r)
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i, h := range header {
		if mapped, ok := columnMap[h]; ok {
			header[i] = mapped
		}
		if seen[header[i]] {
			return nil, fmt.Errorf("goeval: duplicate CSV field %q", header[i])
		}
		seen[header[i]] = true
	}
	d := Dataset{}
	for n := 2; ; n++ {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", n, err)
		}
		obj := map[string]json.RawMessage{}
		for i, value := range row {
			quoted, _ := json.Marshal(value) // JSON marshaling a string cannot fail.
			if value == "" && header[i] != "user_input" && header[i] != "response" && header[i] != "reference" {
				continue
			}
			switch header[i] {
			case "retrieved_contexts", "reference_contexts", "retrieved_context_ids", "reference_context_ids", "messages", "reference_tool_calls", "reference_topics", "rubrics", "metadata":
				obj[header[i]] = json.RawMessage(value)
			case "user_input":
				if strings.HasPrefix(strings.TrimSpace(value), "[") && json.Valid([]byte(value)) {
					obj[header[i]] = json.RawMessage(value)
				} else {
					obj[header[i]] = quoted
				}
			default:
				obj[header[i]] = quoted
			}
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", n, err)
		}
		var s Sample
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("row %d: %w", n, err)
		}
		d = append(d, s)
	}
	return d, nil
}
func (d Dataset) WriteCSV(w io.Writer) error {
	c := csv.NewWriter(w)
	header := []string{"user_input", "response", "reference", "retrieved_contexts", "reference_contexts", "retrieved_context_ids", "reference_context_ids", "messages", "reference_tool_calls", "reference_topics", "rubrics", "metadata"}
	if err := c.Write(header); err != nil {
		return err
	}
	for _, s := range d {
		b, err := json.Marshal(s)
		if err != nil {
			return err
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		row := make([]string, len(header))
		for i, key := range header {
			if v := obj[key]; v != nil {
				if i < 3 && len(v) > 0 && v[0] == '"' {
					if err := json.Unmarshal(v, &row[i]); err != nil {
						return err
					}
				} else {
					row[i] = string(v)
				}
			}
		}
		if err := c.Write(row); err != nil {
			return err
		}
	}
	c.Flush()
	return c.Error()
}
