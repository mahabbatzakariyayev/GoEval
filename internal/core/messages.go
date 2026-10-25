package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// UnmarshalJSON accepts upstream multi-turn user_input arrays as well as the
// explicit messages field. Scalar and conversational input cannot coexist.
func (s *Sample) UnmarshalJSON(data []byte) error {
	type plain Sample
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	if obj == nil {
		return fmt.Errorf("goeval: sample must be a JSON object")
	}
	input := bytes.TrimSpace(obj["user_input"])
	if len(input) > 0 && input[0] == '[' {
		if _, exists := obj["messages"]; exists {
			return fmt.Errorf("goeval: both messages and conversational user_input supplied")
		}
		obj["messages"] = input
		delete(obj, "user_input")
	}
	// Upstream accepts string or integer context IDs and compares their string
	// representations. Normalize integers without converting through float64.
	for _, key := range []string{"retrieved_context_ids", "reference_context_ids"} {
		raw, ok := obj[key]
		if !ok || string(raw) == "null" {
			continue
		}
		var ids []json.RawMessage
		if err := json.Unmarshal(raw, &ids); err != nil {
			return err
		}
		normalized := make([]string, len(ids))
		for i, id := range ids {
			if len(id) > 0 && id[0] == '"' {
				if err := json.Unmarshal(id, &normalized[i]); err != nil {
					return err
				}
			} else {
				var number json.Number
				if err := json.Unmarshal(id, &number); err != nil {
					return fmt.Errorf("goeval: context ID must be a string or integer")
				}
				if strings.ContainsAny(number.String(), ".eE") || number.String() == "" {
					return fmt.Errorf("goeval: context ID must be a string or integer")
				}
				normalized[i] = number.String()
			}
		}
		b, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		obj[key] = b
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	var decoded plain
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return err
	}
	if decoded.UserInput != "" && decoded.Messages != nil {
		return fmt.Errorf("goeval: cannot combine scalar user_input and messages")
	}
	*s = Sample(decoded)
	s.present = map[string]bool{}
	for key, value := range obj {
		s.present[key] = !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
	}
	return nil
}
func (s Sample) MarshalJSON() ([]byte, error) {
	type plain Sample
	if s.UserInput != "" && s.Messages != nil {
		return nil, fmt.Errorf("goeval: cannot combine scalar user_input and messages")
	}
	b, err := json.Marshal(plain(s))
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	for key, value := range map[string]string{"user_input": s.UserInput, "response": s.Response, "reference": s.Reference} {
		if value != "" || s.present == nil || s.present[key] {
			v, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			obj[key] = v
		}
	}
	// Preserve present-but-empty arrays for required-field validation on reload.
	for key, value := range map[string][]string{"retrieved_contexts": s.RetrievedContexts, "reference_contexts": s.ReferenceContexts, "retrieved_context_ids": s.RetrievedContextIDs, "reference_context_ids": s.ReferenceContextIDs, "reference_topics": s.ReferenceTopics} {
		if value != nil {
			v, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			obj[key] = v
		}
	}
	if s.ReferenceToolCalls != nil {
		v, err := json.Marshal(s.ReferenceToolCalls)
		if err != nil {
			return nil, err
		}
		obj["reference_tool_calls"] = v
	}
	if s.Messages != nil {
		v, err := json.Marshal(s.Messages)
		if err != nil {
			return nil, err
		}
		obj["user_input"] = v
		delete(obj, "messages")
	}
	return json.Marshal(obj)
}

// WithResponse marks an application response as present, including an empty
// response, while preserving JSON field-presence information on loaded rows.
func (s Sample) WithResponse(response string) Sample {
	s.Response = response
	if s.present != nil {
		p := map[string]bool{}
		for k, v := range s.present {
			p[k] = v
		}
		p["response"] = true
		s.present = p
	}
	return s
}

// Without returns a copy with the named fields removed. It also marks scalar
// fields absent, unlike assigning an empty string (which is a supplied value).
// Unknown fields have no effect. Other maps and slices remain shared.
func (s Sample) Without(fields ...Field) Sample {
	provided := map[string]bool{}
	if s.present == nil {
		for _, f := range []Field{UserInput, Response, Reference} {
			provided[string(f)] = true
		}
	} else {
		for key, value := range s.present {
			provided[key] = value
		}
	}
	for _, field := range fields {
		switch field {
		case UserInput:
			s.UserInput = ""
		case Response:
			s.Response = ""
		case Reference:
			s.Reference = ""
		case RetrievedContexts:
			s.RetrievedContexts = nil
		case ReferenceContexts:
			s.ReferenceContexts = nil
		case RetrievedContextIDs:
			s.RetrievedContextIDs = nil
		case ReferenceContextIDs:
			s.ReferenceContextIDs = nil
		case Messages:
			s.Messages = nil
		case ReferenceToolCalls:
			s.ReferenceToolCalls = nil
		case ReferenceTopics:
			s.ReferenceTopics = nil
		case Rubrics:
			s.Rubrics = nil
		}
		provided[string(field)] = false
	}
	s.present = provided
	return s
}

func (t *ToolCall) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("goeval: tool call must be an object")
	}
	var wire struct {
		Name      string         `json:"name"`
		Args      map[string]any `json:"args"`
		Arguments map[string]any `json:"arguments"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if wire.Args != nil && wire.Arguments != nil {
		return fmt.Errorf("goeval: both args and arguments supplied")
	}
	if wire.Args == nil {
		wire.Args = wire.Arguments
	}
	*t = ToolCall{Name: wire.Name, Arguments: wire.Args}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.rawArguments = raw["args"]
	if t.rawArguments == nil {
		t.rawArguments = raw["arguments"]
	}
	b, err := json.Marshal(t.Arguments)
	if err != nil {
		return err
	}
	t.canonicalArguments = string(b)
	if len(t.rawArguments) > 0 && string(t.rawArguments) != "null" {
		if err := json.Unmarshal(t.rawArguments, &t.rawValues); err != nil {
			return err
		}
		t.canonicalValues = map[string]string{}
		for k, v := range t.Arguments {
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			t.canonicalValues[k] = string(b)
		}
	}
	return nil
}
func (t ToolCall) MarshalJSON() ([]byte, error) {
	args := t.Arguments
	if args == nil {
		args = map[string]any{}
	}
	b, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	if len(t.rawArguments) > 0 && string(b) == t.canonicalArguments {
		b = t.rawArguments
	}
	return json.Marshal(struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}{t.Name, b})
}
func (m *Message) UnmarshalJSON(data []byte) error {
	type plain Message
	var wire struct {
		plain
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*m = Message(wire.plain)
	if m.Role == "" {
		m.Role = wire.Type
	}
	switch m.Role {
	case "human":
		m.Role = "user"
	case "ai":
		m.Role = "assistant"
	}
	if m.Role != "user" && m.Role != "assistant" && m.Role != "tool" {
		return fmt.Errorf("goeval: invalid message type %q", m.Role)
	}
	return nil
}
func (m Message) MarshalJSON() ([]byte, error) {
	role := m.Role
	switch role {
	case "user":
		role = "human"
	case "assistant":
		role = "ai"
	}
	return json.Marshal(struct {
		Type       string         `json:"type"`
		Content    string         `json:"content"`
		ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
		ToolCallID string         `json:"tool_call_id,omitempty"`
		Metadata   map[string]any `json:"metadata,omitempty"`
	}{role, m.Content, m.ToolCalls, m.ToolCallID, m.Metadata})
}

// PrettyRepr uses the Human/AI/Tools/ToolOutput conversation format expected by
// upstream's agent prompts. Go argument maps are rendered in sorted key order.
func (s Sample) PrettyRepr() string {
	lines := []string{}
	for _, m := range s.Messages {
		switch m.Role {
		case "user", "human":
			lines = append(lines, "Human: "+m.Content)
		case "tool":
			lines = append(lines, "ToolOutput: "+m.Content)
		case "assistant", "ai":
			if m.Content != "" {
				lines = append(lines, "AI: "+m.Content)
			}
			if m.ToolCalls != nil {
				lines = append(lines, "Tools:")
				for _, t := range m.ToolCalls {
					lines = append(lines, "  "+t.Name+": "+PythonRepr(t.Arguments))
				}
			}
		default:
			lines = append(lines, m.Role+": "+m.Content)
		}
	}
	return strings.Join(lines, "\n")
}
func pythonRepr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		quote := "'"
		if strings.Contains(x, "'") && !strings.Contains(x, "\"") {
			quote = "\""
		}
		x = strings.ReplaceAll(x, "\\", "\\\\")
		x = strings.ReplaceAll(x, quote, "\\"+quote)
		x = strings.NewReplacer("\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(x)
		return quote + x + quote
	case bool:
		if x {
			return "True"
		}
		return "False"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{}
		for _, k := range keys {
			parts = append(parts, pythonRepr(k)+": "+pythonRepr(x[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, len(x))
		for i, v := range x {
			parts[i] = pythonRepr(v)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case float64:
		return pythonFloat(x)
	case json.Number:
		if strings.ContainsAny(x.String(), ".eE") {
			f, err := x.Float64()
			if err == nil {
				return pythonFloat(f)
			}
		}
		return x.String()
	default:
		return fmt.Sprint(v)
	}
}

// ArgumentString follows Python str() for the argument comparison performed by
// ToolCallAccuracy. JSON numbers retain integer-versus-float spelling on load.
func (t ToolCall) ArgumentString(name string) (string, bool) {
	v, ok := t.Arguments[name]
	if !ok {
		return "", false
	}
	if text, ok := v.(string); ok {
		return text, true
	}
	if raw, ok := t.rawValues[name]; ok {
		b, err := json.Marshal(v)
		if err == nil && string(b) == t.canonicalValues[name] {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if text, err := pythonJSONRepr(dec); err == nil {
				return text, true
			}
		}
	}
	return pythonRepr(v), true
}

// PythonString implements the Python str representation used by GoEval prompt
// field substitution. Map keys are sorted when no original JSON order exists.
func PythonString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return PythonRepr(v)
}

// PythonRepr includes quotes around strings, as Python's !r conversion does.
func PythonRepr(v any) string {
	switch v.(type) {
	case string, bool, float64, json.Number, nil:
		return pythonRepr(v)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<unsupported %T>", v)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	value, err := pythonJSONRepr(dec)
	if err != nil {
		return pythonRepr(v)
	}
	return value
}

func pythonFloat(x float64) string {
	format := byte('e')
	a := math.Abs(x)
	if a == 0 || (a >= 1e-4 && a < 1e16) {
		format = 'f'
	}
	text := strconv.FormatFloat(x, format, -1, 64)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}

// Decode tokens recursively to retain Python dictionary insertion order in
// string comparisons, even though the exposed Go argument map is unordered.
func pythonJSONRepr(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	if delim, ok := tok.(json.Delim); ok {
		parts := []string{}
		switch delim {
		case '{':
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return "", err
				}
				value, err := pythonJSONRepr(dec)
				if err != nil {
					return "", err
				}
				parts = append(parts, pythonRepr(key)+": "+value)
			}
			if _, err := dec.Token(); err != nil {
				return "", err
			}
			return "{" + strings.Join(parts, ", ") + "}", nil
		case '[':
			for dec.More() {
				value, err := pythonJSONRepr(dec)
				if err != nil {
					return "", err
				}
				parts = append(parts, value)
			}
			if _, err := dec.Token(); err != nil {
				return "", err
			}
			return "[" + strings.Join(parts, ", ") + "]", nil
		}
		return "", fmt.Errorf("goeval: unexpected JSON delimiter")
	}
	return pythonRepr(tok), nil
}

// SortKey follows the tuple of name, sorted argument names, and argument strings
// used for flexible-order tool-call alignment upstream.
func (t ToolCall) SortKey() []string {
	keys := make([]string, 0, len(t.Arguments))
	for k := range t.Arguments {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []string{t.Name}
	for _, k := range keys {
		v, _ := t.ArgumentString(k)
		out = append(out, k, v)
	}
	return out
}
