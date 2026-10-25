package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

type TaskCompletionOptions struct {
	Options
	Task string
}

func TaskCompletionMetric(llm goeval.LLM, cfg TaskCompletionOptions) Metric {
	cfg.Options = cloneOptions(cfg.Options)
	return newModelMetric(llm, "task_completion", "Task Completion", []SingleTurnParam{Input, ActualOutput}, cfg.Options, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		var workflow struct {
			Task    *string `json:"task"`
			Outcome *string `json:"outcome"`
		}
		key := "TaskCompletionMetric.extract_goal_and_outcome"
		formatted, err := formatToolCalls(c.ToolsCalled)
		if err != nil {
			return 0, "", nil, err
		}
		values := map[string]any{"input": c.Input, "actual_output": c.ActualOutput, "tools_called_formatted": formatted}
		if c.Trace != nil {
			key = "TaskCompletionMetric.extract_task_and_outcome_from_trace"
			raw, err := json.MarshalIndent(c.Trace, "", "  ")
			if err != nil {
				return 0, "", nil, err
			}
			values = map[string]any{"trace_json": string(raw)}
		}
		if err := judge(ctx, llm, key, values, schema(map[string]any{"task": str(), "outcome": str()}), &workflow); err != nil {
			return 0, "", nil, err
		}
		if workflow.Task == nil || workflow.Outcome == nil {
			return 0, "", nil, fmt.Errorf("judge: missing task or outcome")
		}
		task := *workflow.Task
		if cfg.Task != "" {
			task = cfg.Task
		}
		var verdict struct {
			Verdict *float64 `json:"verdict"`
			Reason  string   `json:"reason"`
		}
		if err := judge(ctx, llm, "TaskCompletionMetric.generate_verdict", map[string]any{"task": task, "actual_outcome": *workflow.Outcome}, schema(map[string]any{"verdict": map[string]any{"type": "number", "minimum": 0, "maximum": 1}, "reason": map[string]any{"type": []string{"string", "null"}}}), &verdict); err != nil {
			return 0, "", nil, err
		}
		if verdict.Verdict == nil || *verdict.Verdict < 0 || *verdict.Verdict > 1 {
			return 0, "", nil, fmt.Errorf("judge: invalid task verdict")
		}
		return strictScore(*verdict.Verdict, threshold, cfg.Options), verdict.Reason, map[string]any{"task": task, "outcome": *workflow.Outcome}, nil
	})
}
func PromptAlignmentMetric(llm goeval.LLM, instructions []string, opts Options) Metric {
	opts = cloneOptions(opts)
	instructions = append([]string(nil), instructions...)
	m := newModelMetric(llm, "prompt_alignment", "Prompt Alignment", []SingleTurnParam{Input, ActualOutput}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		if len(instructions) == 0 {
			return 0, "", nil, fmt.Errorf("judge: prompt instructions are required")
		}
		v, err := verdicts(ctx, llm, "PromptAlignmentMetric.generate_verdicts", map[string]any{"input": c.Input, "actual_output": c.ActualOutput, "prompt_instructions": instructions}, false, false)
		if err != nil {
			return 0, "", nil, err
		}
		if err := validateVerdictCount(v, len(instructions)); err != nil {
			return 0, "", nil, err
		}
		score := strictScore(fraction(v, []string{"yes"}, 1), threshold, opts)
		bad := []any{}
		for _, item := range v {
			if item.Verdict == "no" {
				bad = append(bad, reason(item))
			}
		}
		r, err := explanation(ctx, llm, "PromptAlignmentMetric.generate_reason", map[string]any{"input": c.Input, "actual_output": c.ActualOutput, "score": fmt.Sprintf("%.2f", score), "unalignment_reasons": bad}, opts)
		return score, r, map[string]any{"verdicts": v}, err
	})
	if len(instructions) == 0 {
		m.configErr = fmt.Errorf("judge: prompt instructions are required")
	}
	for _, instruction := range instructions {
		if strings.TrimSpace(instruction) == "" {
			m.configErr = fmt.Errorf("judge: prompt instructions must be nonempty")
		}
	}
	return m
}

// ArgumentCorrectnessMetric judges whether each tool invocation has appropriate
// arguments for the user's input, independently of reference tool matching.
func ArgumentCorrectnessMetric(llm goeval.LLM, opts Options) Metric {
	opts = cloneOptions(opts)
	m := newMetric("argument_correctness", "Argument Correctness", []SingleTurnParam{Input, ToolsCalled}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		if len(c.ToolsCalled) == 0 {
			return 1, "No tool calls provided", map[string]any{"verdicts": []Verdict{}}, nil
		}
		v := []Verdict{}
		var err error
		if len(c.ToolsCalled) > 0 {
			v, err = verdicts(ctx, llm, "ArgumentCorrectnessMetric.generate_verdicts", map[string]any{"input": c.Input, "stringified_tools_called": toolCallListRepr(c.ToolsCalled)}, false, false)
			if err != nil {
				return 0, "", nil, err
			}
		}
		score := strictScore(fraction(v, []string{"yes"}, 1), threshold, opts)
		if err := validateVerdictCount(v, len(c.ToolsCalled)); err != nil {
			return 0, "", nil, err
		}
		bad := []any{}
		for _, item := range v {
			if item.Verdict == "no" {
				bad = append(bad, reason(item))
			}
		}
		r, err := explanation(ctx, llm, "ArgumentCorrectnessMetric.generate_reason", map[string]any{"input": c.Input, "score": fmt.Sprintf("%.2f", score), "incorrect_tool_calls_reasons": bad}, opts)
		return score, r, map[string]any{"verdicts": v}, err
	})
	m.validateCase = func(c Case) error {
		if len(c.ToolsCalled) > 0 && check.Nil(llm) {
			return fmt.Errorf("judge: argument correctness requires a model for nonempty tool calls")
		}
		return nil
	}
	return m
}

func formatToolCalls(calls []ToolCall) (string, error) {
	if len(calls) == 0 {
		return "", nil
	}
	parts := make([]string, len(calls))
	for i, c := range calls {
		// Preserve upstream field order and include optional null fields.
		var description, reasoning any
		if c.Description != "" {
			description = c.Description
		}
		if c.Reasoning != "" {
			reasoning = c.Reasoning
		}
		value := struct {
			Name            string         `json:"name"`
			Type            string         `json:"type"`
			Description     any            `json:"description"`
			Reasoning       any            `json:"reasoning"`
			Output          any            `json:"output"`
			InputParameters map[string]any `json:"input_parameters"`
		}{c.Name, toolType(c), description, reasoning, c.Output, c.InputParameters}
		raw, err := json.MarshalIndent(value, "", "    ")
		if err != nil {
			return "", err
		}
		parts[i] = "  " + strings.ReplaceAll(string(raw), "\n", "\n  ")
	}
	return "[\n" + strings.Join(parts, ",\n") + "\n]", nil
}

func toolCallListRepr(calls []ToolCall) string {
	parts := make([]string, len(calls))
	for i, c := range calls {
		parts[i] = c.Repr()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// Repr renders the original ToolCall prompt representation using native Go.
// Go map keys are sorted; insertion order from arbitrary source objects is not retained.
func (c ToolCall) Repr() string {
	fields := []string{}
	if c.Name != "" {
		fields = append(fields, `name="`+c.Name+`"`)
	}
	fields = append(fields, `type="`+toolType(c)+`"`)
	if c.Description != "" {
		fields = append(fields, `description="`+c.Description+`"`)
	}
	if c.Reasoning != "" {
		fields = append(fields, `reasoning="`+c.Reasoning+`"`)
	}
	indent := func(name string, v any) string {
		raw, _ := json.MarshalIndent(v, "", "    ")
		return name + "=" + strings.ReplaceAll(string(raw), "\n", "\n    ")
	}
	if len(c.InputParameters) > 0 {
		fields = append(fields, indent("input_parameters", c.InputParameters))
	}
	if c.Output != nil {
		if _, ok := c.Output.(map[string]any); ok {
			fields = append(fields, indent("output", c.Output))
		} else {
			fields = append(fields, "output="+goeval.PythonRepr(c.Output))
		}
	}
	return "ToolCall(\n    " + strings.Join(fields, ",\n    ") + "\n)"
}
