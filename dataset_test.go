package goeval_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval"
)

func TestDatasetInterchangeRoundTrip(t *testing.T) {
	input := `[{"user_input":"Bakı?","response":"","reference":"Bakı","retrieved_contexts":[],"reference_contexts":["Bakı is in Azerbaijan."],"retrieved_context_ids":[1,"2",123456789012345678901234567890],"reference_context_ids":["1"],"metadata":{"label":"test"}},{"user_input":[{"type":"human","content":"weather?"},{"type":"ai","content":"","tool_calls":[{"name":"weather","args":{"city":"Bakı"}}]},{"type":"tool","content":"Sunny"}],"reference_tool_calls":[],"reference_topics":[]}]`
	d, err := goeval.ReadJSON(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d[0].RetrievedContextIDs, []string{"1", "2", "123456789012345678901234567890"}) {
		t.Fatal(d[0].RetrievedContextIDs)
	}
	if d[1].Messages[1].Role != "assistant" || d[1].Messages[1].ToolCalls[0].Arguments["city"] != "Bakı" {
		t.Fatal(d[1])
	}
	for _, format := range []string{"json", "jsonl", "csv"} {
		t.Run(format, func(t *testing.T) {
			var b bytes.Buffer
			var got goeval.Dataset
			var err error
			switch format {
			case "json":
				err = d.WriteJSON(&b)
				if err == nil {
					got, err = goeval.ReadJSON(&b)
				}
			case "jsonl":
				err = d.WriteJSONL(&b)
				if err == nil {
					got, err = goeval.ReadJSONL(&b)
				}
			case "csv":
				err = d.WriteCSV(&b)
				if err == nil {
					got, err = goeval.ReadCSV(&b, nil)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || got[0].RetrievedContexts == nil || got[1].ReferenceToolCalls == nil || got[1].ReferenceTopics == nil {
				t.Fatalf("empty arrays lost: %+v", got)
			}
			if err := got[0].Validate(goeval.Response); err != nil {
				t.Fatalf("present empty response rejected: %v", err)
			}
			if !reflect.DeepEqual(got[1].Messages, d[1].Messages) {
				t.Fatalf("messages changed: %+v", got[1].Messages)
			}
		})
	}
}
func TestDatasetColumnMappingAndErrors(t *testing.T) {
	d, err := goeval.ReadCSV(strings.NewReader("question,answer,truth\n[hello],Paris,Paris\n"), map[string]string{"question": "user_input", "answer": "response", "truth": "reference"})
	if err != nil || d[0].UserInput != "[hello]" {
		t.Fatalf("%+v %v", d, err)
	}
	for _, raw := range []string{`[null]`, `[{"user_input":[],"messages":[]}]`, `[{"user_input":"q","messages":[{"type":"human","content":"q"}]}]`, `[] {}`, `[{"retrieved_context_ids":[1.5]}]`, `[{"messages":[{"type":"invalid"}]}]`} {
		if _, err := goeval.ReadJSON(strings.NewReader(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if _, err := goeval.ReadJSONL(strings.NewReader("{}\n{bad}\n")); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatal(err)
	}
	if _, err := goeval.ReadCSV(strings.NewReader("response,answer\na,b\n"), map[string]string{"answer": "response"}); err == nil {
		t.Fatal("duplicate mapped columns accepted")
	}
}
func TestConversationPrettyRepr(t *testing.T) {
	s := goeval.Sample{Messages: []goeval.Message{{Role: "user", Content: "weather?"}, {Role: "assistant", ToolCalls: []goeval.ToolCall{{Name: "weather", Arguments: map[string]any{"city": "Bakı"}}}}, {Role: "tool", Content: "Sunny"}, {Role: "assistant", Content: "Sunny today"}}}
	want := "Human: weather?\nTools:\n  weather: {'city': 'Bakı'}\nToolOutput: Sunny\nAI: Sunny today"
	if s.PrettyRepr() != want {
		t.Fatalf("%s", s.PrettyRepr())
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"type":"human"`) || strings.Contains(string(b), `"messages"`) {
		t.Fatal(string(b))
	}
}
