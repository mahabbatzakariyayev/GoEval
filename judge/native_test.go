package judge

import (
	"context"
	"testing"
)

func TestToolMatchingModes(t *testing.T) {
	a, b := ToolCall{Name: "a"}, ToolCall{Name: "b"}
	for _, tt := range []struct {
		name             string
		actual, expected []ToolCall
		cfg              ToolCorrectnessOptions
		want             float64
	}{
		{"empty", []ToolCall{}, []ToolCall{}, ToolCorrectnessOptions{}, 1},
		{"unexpected", []ToolCall{a}, []ToolCall{}, ToolCorrectnessOptions{}, 0},
		{"missing", []ToolCall{}, []ToolCall{a}, ToolCorrectnessOptions{}, 0},
		{"unordered", []ToolCall{b, a}, []ToolCall{a, b}, ToolCorrectnessOptions{}, 1},
		{"ordered", []ToolCall{b, a}, []ToolCall{a, b}, ToolCorrectnessOptions{ShouldConsiderOrdering: true}, .5},
		{"exact", []ToolCall{b, a}, []ToolCall{a, b}, ToolCorrectnessOptions{ShouldExactMatch: true}, 0},
		{"exact extra", []ToolCall{a, b}, []ToolCall{a}, ToolCorrectnessOptions{ShouldExactMatch: true}, 0},
		{"duplicate value set", []ToolCall{a, a}, []ToolCall{a, a}, ToolCorrectnessOptions{}, .5},
		{"duplicate ordered", []ToolCall{a, a}, []ToolCall{a, a}, ToolCorrectnessOptions{ShouldConsiderOrdering: true}, 1},
		{"type mismatch", []ToolCall{{Name: "a", Type: "MCP"}}, []ToolCall{a}, ToolCorrectnessOptions{}, 0},
		{"numeric equivalence", []ToolCall{{Name: "a", InputParameters: map[string]any{"n": true}}}, []ToolCall{{Name: "a", InputParameters: map[string]any{"n": 1.0}}}, ToolCorrectnessOptions{EvaluationParams: []ToolCallParam{InputParameters}}, 1},
		{"partial", []ToolCall{{Name: "a", InputParameters: map[string]any{"x": 1, "y": 3}}}, []ToolCall{{Name: "a", InputParameters: map[string]any{"x": 1, "y": 2}}}, ToolCorrectnessOptions{EvaluationParams: []ToolCallParam{InputParameters}}, .5},
		{"output mismatch", []ToolCall{{Name: "a", Output: "a"}}, []ToolCall{{Name: "a", Output: "b"}}, ToolCorrectnessOptions{EvaluationParams: []ToolCallParam{Output}}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := ToolCorrectnessMetric(nil, tt.cfg)
			r, err := m.Measure(context.Background(), Case{ToolsCalled: tt.actual, ExpectedTools: tt.expected})
			if err != nil || r.Score != tt.want {
				t.Fatalf("%+v %v want %g", r, err, tt.want)
			}
		})
	}
	if got := compareDicts(map[string]any{"nested": map[string]any{"x": 1, "y": 2}}, map[string]any{"nested": map[string]any{"x": 1, "y": 3}}, false); got != .5 {
		t.Fatal(got)
	}
}

func TestToolSelectionCombinesWithCalling(t *testing.T) {
	c := allFields()
	c.ToolsCalled = []ToolCall{{Name: "a"}}
	c.ExpectedTools = []ToolCall{{Name: "a"}}
	m := ToolCorrectnessMetric(scripted(t, stage{map[string]any{"user_input": "question"}, `{"score":0.3,"reason":"A better tool exists."}`}), ToolCorrectnessOptions{AvailableTools: []ToolCall{{Name: "a", Description: "Available"}}})
	r, err := m.Measure(context.Background(), c)
	if err != nil || r.Score != .3 || r.Success {
		t.Fatal(r, err)
	}
}

func TestPatternMatchesEntireTrimmedOutput(t *testing.T) {
	m, err := PatternMatchMetric(" [a-z]+-[0-9]+ ", true, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]float64{" ABC-12 ": 1, "prefix ABC-12": 0, "ABC-12\nother": 0} {
		r, err := m.Measure(context.Background(), Case{ActualOutput: text})
		if err != nil || r.Score != want {
			t.Fatal(r, err)
		}
	}
	if _, err := PatternMatchMetric(`(?<=prefix)x`, false, Options{}); err == nil {
		t.Fatal("unsupported lookbehind accepted")
	}
}
