package scorers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/internal/prompts"
)

func predictedTools(s goeval.Sample) []goeval.ToolCall {
	calls := []goeval.ToolCall{}
	for _, m := range s.Messages {
		if m.Role == "assistant" || m.Role == "ai" {
			calls = append(calls, m.ToolCalls...)
		}
	}
	return calls
}
func toolKey(t goeval.ToolCall) (string, error) {
	args := t.Arguments
	if args == nil {
		args = map[string]any{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return "", err
	}
	normalized, err := hashable(decoded)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal([]any{t.Name, normalized})
	return string(b), err
}

// Python's set comparison equates integers, integral floats, and booleans.
func hashable(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return []any{"none"}, nil
	case string:
		return []any{"string", x}, nil
	case bool:
		if x {
			return []any{"number", "1"}, nil
		}
		return []any{"number", "0"}, nil
	case json.Number:
		key, err := check.NumberKey(x)
		if err != nil {
			return nil, err
		}
		return []any{"number", key}, nil
	case []any:
		out := []any{"tuple"}
		for _, e := range x {
			h, err := hashable(e)
			if err != nil {
				return nil, err
			}
			out = append(out, h)
		}
		return out, nil
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := []any{"map"}
		for _, k := range keys {
			h, err := hashable(x[k])
			if err != nil {
				return nil, err
			}
			out = append(out, []any{k, h})
		}
		return out, nil
	}
	return nil, fmt.Errorf("metrics: unsupported tool argument")
}
func ToolCallF1() goeval.Metric {
	return goeval.MetricFunc{MetricName: "tool_call_f1", Fields: []goeval.Field{goeval.Messages, goeval.ReferenceToolCalls}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if err := ctx.Err(); err != nil {
			return goeval.Score{}, err
		}
		actual, expected := map[string]bool{}, map[string]bool{}
		for _, c := range predictedTools(s) {
			k, err := toolKey(c)
			if err != nil {
				return goeval.Score{}, err
			}
			actual[k] = true
		}
		for _, c := range s.ReferenceToolCalls {
			k, err := toolKey(c)
			if err != nil {
				return goeval.Score{}, err
			}
			expected[k] = true
		}
		tp := 0
		for k := range actual {
			if expected[k] {
				tp++
			}
		}
		den := len(actual) + len(expected)
		if den == 0 {
			return goeval.Score{Value: 0}, nil
		}
		return goeval.Score{Value: math.RoundToEven(2*float64(tp)/float64(den)*10000) / 10000}, nil
	}}
}

type ToolAccuracyOptions struct{ FlexibleOrder bool }

func ToolCallAccuracy(opts ToolAccuracyOptions) goeval.Metric {
	return goeval.MetricFunc{MetricName: "tool_call_accuracy", Fields: []goeval.Field{goeval.Messages, goeval.ReferenceToolCalls}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if err := ctx.Err(); err != nil {
			return goeval.Score{}, err
		}
		actual, expected := predictedTools(s), append([]goeval.ToolCall(nil), s.ReferenceToolCalls...)
		if len(actual) == 0 && len(expected) == 0 {
			return goeval.Score{Value: 1}, nil
		}
		if len(actual) != len(expected) {
			return goeval.Score{Value: 0, Reason: "Tool sequence lengths differ"}, nil
		}
		if opts.FlexibleOrder {
			sortCalls := func(c []goeval.ToolCall) {
				sort.SliceStable(c, func(i, j int) bool {
					a, b := c[i].SortKey(), c[j].SortKey()
					for k := 0; k < min(len(a), len(b)); k++ {
						if a[k] != b[k] {
							return a[k] < b[k]
						}
					}
					return len(a) < len(b)
				})
			}
			sortCalls(actual)
			sortCalls(expected)
		}
		sum := 0.0
		for i, pred := range actual {
			ref := expected[i]
			if pred.Name != ref.Name {
				return goeval.Score{Value: 0, Reason: "Tool sequence names differ"}, nil
			}
			if len(ref.Arguments) == 0 {
				if len(pred.Arguments) == 0 {
					sum++
				}
				continue
			}
			matches := 0
			for k := range ref.Arguments {
				a, _ := ref.ArgumentString(k)
				b, ok := pred.ArgumentString(k)
				if ok && a == b {
					matches++
				}
			}
			sum += float64(matches) / float64(len(ref.Arguments))
		}
		if len(expected) == 0 {
			return goeval.Score{Value: 0}, nil
		}
		return goeval.Score{Value: sum / float64(len(expected))}, nil
	}}
}
func goalAccuracy(llm goeval.LLM, reference bool) goeval.Metric {
	fields := []goeval.Field{goeval.Messages}
	if reference {
		fields = append(fields, goeval.Reference)
	}
	return goeval.MetricFunc{MetricName: "agent_goal_accuracy", Fields: fields, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		var out struct {
			UserGoal string `json:"user_goal"`
			EndState string `json:"end_state"`
		}
		err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("goal_accuracy.InferGoalOutcomePrompt"), Input: map[string]string{"workflow": s.PrettyRepr()}, Schema: objectSchema(map[string]any{"user_goal": stringSchema(), "end_state": stringSchema()})}, &out)
		if err != nil {
			return goeval.Score{}, err
		}
		if out.UserGoal == "" || out.EndState == "" {
			return goeval.Score{}, fmt.Errorf("metrics: missing workflow goal or outcome")
		}
		desired := out.UserGoal
		if reference {
			desired = s.Reference
		}
		var comparison struct {
			Verdict *string `json:"verdict"`
			Reason  string  `json:"reason"`
		}
		err = goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("goal_accuracy.CompareOutcomePrompt"), Input: map[string]string{"desired_outcome": desired, "arrived_outcome": out.EndState}, Schema: objectSchema(map[string]any{"verdict": map[string]any{"type": "string", "enum": []string{"0", "1"}}, "reason": stringSchema()})}, &comparison)
		if err != nil {
			return goeval.Score{}, err
		}
		if comparison.Verdict == nil || (*comparison.Verdict != "0" && *comparison.Verdict != "1") {
			return goeval.Score{}, fmt.Errorf("metrics: invalid goal verdict")
		}
		v := 0.0
		if *comparison.Verdict == "1" {
			v = 1
		}
		return goeval.Score{Value: v, Reason: comparison.Reason}, nil
	}}
}
func AgentGoalAccuracyWithReference(llm goeval.LLM) goeval.Metric    { return goalAccuracy(llm, true) }
func AgentGoalAccuracyWithoutReference(llm goeval.LLM) goeval.Metric { return goalAccuracy(llm, false) }
func TopicAdherence(llm goeval.LLM, mode string) goeval.Metric {
	return goeval.MetricFunc{MetricName: "topic_adherence", Fields: []goeval.Field{goeval.Messages, goeval.ReferenceTopics}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if mode != "" && mode != "precision" && mode != "recall" && mode != "f1" {
			return goeval.Score{}, fmt.Errorf("metrics: invalid topic adherence mode")
		}
		var topics struct {
			Topics *[]string `json:"topics"`
		}
		err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("topic_adherence.TopicExtractionPrompt"), Input: map[string]string{"user_input": s.PrettyRepr()}, Schema: objectSchema(map[string]any{"topics": arraySchema(stringSchema())})}, &topics)
		if err != nil {
			return goeval.Score{}, err
		}
		if topics.Topics == nil {
			return goeval.Score{}, fmt.Errorf("metrics: missing topics")
		}
		answered := make([]bool, len(*topics.Topics))
		for i, topic := range *topics.Topics {
			var out struct {
				Refused *bool `json:"refused_to_answer"`
			}
			err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("topic_adherence.TopicRefusedPrompt"), Input: map[string]string{"user_input": s.PrettyRepr(), "topic": topic}, Schema: objectSchema(map[string]any{"refused_to_answer": map[string]any{"type": "boolean"}})}, &out)
			if err != nil {
				return goeval.Score{}, err
			}
			if out.Refused == nil {
				return goeval.Score{}, fmt.Errorf("metrics: missing topic refusal verdict")
			}
			answered[i] = !*out.Refused
		}
		var out struct {
			Classifications *[]bool `json:"classifications"`
		}
		err = goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("topic_adherence.TopicClassificationPrompt"), Input: map[string]any{"reference_topics": s.ReferenceTopics, "topics": *topics.Topics}, Schema: objectSchema(map[string]any{"classifications": arraySchema(map[string]any{"type": "boolean"})})}, &out)
		if err != nil {
			return goeval.Score{}, err
		}
		if out.Classifications == nil {
			return goeval.Score{}, fmt.Errorf("metrics: missing topic classifications")
		}
		tp, fp, fn := 0.0, 0.0, 0.0
		for i, a := range answered {
			within := i < len(*out.Classifications) && (*out.Classifications)[i]
			if a && within {
				tp++
			}
			if a && !within {
				fp++
			}
			if !a && within {
				fn++
			}
		}
		precision, recall := tp/(tp+fp+1e-10), tp/(tp+fn+1e-10)
		v := 2 * precision * recall / (precision + recall + 1e-10)
		if mode == "precision" {
			v = precision
		} else if mode == "recall" {
			v = recall
		}
		return goeval.Score{Value: v}, nil
	}}
}
