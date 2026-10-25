package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

// ExactMatchMetric follows Judge's whitespace-trimmed comparison. GoEval's
// raw-string ExactMatch remains available independently in package scorers.
func ExactMatchMetric(opts Options) Metric {
	opts = cloneOptions(opts)
	m := newMetric("exact_match", "Exact Match", []SingleTurnParam{ActualOutput, ExpectedOutput}, opts, func(_ context.Context, c Case, _ float64) (float64, string, map[string]any, error) {
		if strings.TrimSpace(c.ActualOutput) == strings.TrimSpace(c.ExpectedOutput) {
			return 1, "The actual and expected outputs are exact matches.", nil, nil
		}
		return 0, "The actual and expected outputs are different.", nil, nil
	})
	m.defaultThreshold = 1
	return m
}
func PatternMatchMetric(pattern string, ignoreCase bool, opts Options) (Metric, error) {
	opts = cloneOptions(opts)
	pattern = strings.TrimSpace(pattern)
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	compiled, err := regexp.Compile("\\A(?:" + pattern + ")\\z")
	if err != nil {
		return nil, fmt.Errorf("judge: invalid or unsupported pattern: %w", err)
	}
	m := newMetric("pattern_match", "Pattern Match", []SingleTurnParam{ActualOutput}, opts, func(_ context.Context, c Case, _ float64) (float64, string, map[string]any, error) {
		if compiled.MatchString(strings.TrimSpace(c.ActualOutput)) {
			return 1, "The actual output fully matches the pattern.", nil, nil
		}
		return 0, "The actual output does not match the pattern.", nil, nil
	})
	m.defaultThreshold = 1
	return m, nil
}

type ToolCallParam string

const (
	InputParameters ToolCallParam = "input_parameters"
	Output          ToolCallParam = "output"
)

type ToolCorrectnessOptions struct {
	Options
	EvaluationParams       []ToolCallParam
	ShouldExactMatch       bool
	ShouldConsiderOrdering bool
	AvailableTools         []ToolCall
}

func toolType(t ToolCall) string {
	if t.Type == "" {
		return "FUNCTION"
	}
	return t.Type
}
func semanticEqual(a, b any) bool {
	normalize := func(v any) any {
		raw, err := json.Marshal(v)
		if err != nil {
			return v
		}
		var out any
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		if err := dec.Decode(&out); err != nil {
			return v
		}
		return out
	}
	return equalJSON(normalize(a), normalize(b))
}
func equalJSON(a, b any) bool {
	number := func(v any) (string, bool) {
		switch x := v.(type) {
		case bool:
			if x {
				return "1", true
			}
			return "0", true
		case json.Number:
			key, err := check.NumberKey(x)
			return key, err == nil
		}
		return "", false
	}
	if x, ok := number(a); ok {
		y, ok := number(b)
		return ok && x == y
	}
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equalJSON(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i, v := range x {
			if !equalJSON(v, y[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}
func compareDicts(expected, actual map[string]any, exact bool) float64 {
	if semanticEqual(expected, actual) {
		return 1
	}
	if exact || expected == nil || actual == nil {
		return 0
	}
	union := map[string]bool{}
	for k := range expected {
		union[k] = true
	}
	for k := range actual {
		union[k] = true
	}
	score := 0.0
	for key, a := range expected {
		b, ok := actual[key]
		if !ok {
			continue
		}
		if semanticEqual(a, b) {
			score += 1 / float64(len(union))
		} else {
			am, aok := a.(map[string]any)
			bm, bok := b.(map[string]any)
			if aok && bok {
				score += compareDicts(am, bm, false) / float64(len(union))
			}
		}
	}
	return score
}
func toolPairScore(expected, actual ToolCall, cfg ToolCorrectnessOptions) float64 {
	if expected.Name != actual.Name || toolType(expected) != toolType(actual) {
		return 0
	}
	score := 1.0
	for _, param := range cfg.EvaluationParams {
		switch param {
		case InputParameters:
			score *= compareDicts(expected.InputParameters, actual.InputParameters, cfg.ShouldExactMatch)
		case Output:
			if !semanticEqual(expected.Output, actual.Output) {
				return 0
			}
		}
	}
	return score
}
func toolCorrectness(ctx context.Context, actual, expected []ToolCall, cfg ToolCorrectnessOptions) (float64, error) {
	if len(expected) == 0 {
		if len(actual) == 0 {
			return 1, nil
		}
		return 0, nil
	}
	if cfg.ShouldExactMatch {
		if len(actual) != len(expected) {
			return 0, nil
		}
		for i, e := range expected {
			if toolPairScore(e, actual[i], cfg) != 1 {
				return 0, nil
			}
		}
		return 1, nil
	}
	if cfg.ShouldConsiderOrdering {
		row := make([]float64, len(actual)+1)
		for _, e := range expected {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			previous := row[0]
			for j, a := range actual {
				old := row[j+1]
				match := toolPairScore(e, a, cfg)
				row[j+1] = max(row[j+1], row[j])
				if match > 0 {
					row[j+1] = max(row[j+1], previous+match)
				}
				previous = old
			}
		}
		return row[len(actual)] / float64(len(expected)), nil
	}
	used := make([]bool, len(actual))
	total := 0.0
	for _, e := range expected {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		best, index := 0.0, -1
		for j, a := range actual {
			if used[j] {
				continue
			}
			score := toolPairScore(e, a, cfg)
			if score > best {
				best, index = score, j
			}
		}
		if index < 0 {
			continue
		}
		total += best
		selected := actual[index]
		for j, a := range actual {
			if a.Name == selected.Name && semanticEqual(a.InputParameters, selected.InputParameters) && semanticEqual(a.Output, selected.Output) {
				used[j] = true
			}
		}
	}
	return total / float64(len(expected)), nil
}

// ToolCorrectnessMetric uses deterministic reference matching and optionally
// evaluates tool selection against available tool descriptions with the LLM.
func ToolCorrectnessMetric(llm goeval.LLM, cfg ToolCorrectnessOptions) Metric {
	cfg.Options = cloneOptions(cfg.Options)
	cfg.EvaluationParams = append([]ToolCallParam(nil), cfg.EvaluationParams...)
	cfg.AvailableTools = append([]ToolCall(nil), cfg.AvailableTools...)
	m := newMetric("tool_correctness", "Tool Correctness", []SingleTurnParam{Input, ToolsCalled, ExpectedTools}, cfg.Options, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		for _, p := range cfg.EvaluationParams {
			if p != InputParameters && p != Output {
				return 0, "", nil, fmt.Errorf("judge: unsupported tool comparison parameter %q", p)
			}
		}
		score, err := toolCorrectness(ctx, c.ToolsCalled, c.ExpectedTools, cfg)
		if err != nil {
			return 0, "", nil, err
		}
		calling := strictScore(score, threshold, cfg.Options)
		selection := 1.0
		selectionReason := "No available tools were provided to assess tool selection criteria"
		if len(cfg.AvailableTools) > 0 {
			called, err := formatToolCalls(c.ToolsCalled)
			if err != nil {
				return 0, "", nil, err
			}
			available, err := formatToolCalls(cfg.AvailableTools)
			if err != nil {
				return 0, "", nil, err
			}
			var out struct {
				Score  *float64 `json:"score"`
				Reason *string  `json:"reason"`
			}
			err = judge(ctx, llm, "ToolCorrectnessMetric.get_tool_selection_score", map[string]any{"user_input": c.Input, "tools_called": called, "available_tools": available}, schema(map[string]any{"score": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "reason": str()}), &out)
			if err != nil {
				return 0, "", nil, err
			}
			if out.Score == nil || out.Reason == nil || *out.Score < 0 || *out.Score > 1 {
				return 0, "", nil, fmt.Errorf("judge: invalid tool selection result")
			}
			selection, selectionReason = *out.Score, *out.Reason
		}
		return min(calling, selection), fmt.Sprintf("Tool calling score: %.2f. Tool selection: %s", calling, selectionReason), map[string]any{"tool_calling_score": calling, "tool_selection_score": selection}, nil
	})
	for _, p := range cfg.EvaluationParams {
		if p != InputParameters && p != Output {
			m.configErr = fmt.Errorf("judge: unsupported tool comparison parameter %q", p)
		}
	}
	m.model, m.requiresModel = llm, len(cfg.AvailableTools) > 0
	return m
}
