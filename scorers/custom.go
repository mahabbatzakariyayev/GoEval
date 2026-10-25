package scorers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/prompt"
)

func sampleValues(s goeval.Sample) (map[string]any, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var values map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&values); err != nil {
		return nil, err
	}
	return values, nil
}
func requiredValues(values map[string]any, fields []goeval.Field) error {
	for _, f := range fields {
		v, ok := values[string(f)]
		if !ok || v == nil {
			return fmt.Errorf("metrics: missing required input %q", f)
		}
	}
	return nil
}

type Numeric struct {
	LLM        goeval.LLM
	MetricName string
	Definition string
	Min, Max   float64
	Fields     []goeval.Field
}

var _ goeval.Metric = (*Numeric)(nil)

func NumericMetric(llm goeval.LLM, name, definition string, low, high float64, fields ...goeval.Field) *Numeric {
	return &Numeric{LLM: llm, MetricName: name, Definition: definition, Min: low, Max: high, Fields: append([]goeval.Field(nil), fields...)}
}
func (m *Numeric) Name() string                   { return m.MetricName }
func (m *Numeric) RequiredFields() []goeval.Field { return append([]goeval.Field(nil), m.Fields...) }
func (m *Numeric) Validate() error {
	if check.Nil(m.LLM) || strings.TrimSpace(m.MetricName) == "" || strings.TrimSpace(m.Definition) == "" || math.IsNaN(m.Min) || math.IsNaN(m.Max) || math.IsInf(m.Min, 0) || math.IsInf(m.Max, 0) || m.Min > m.Max {
		return fmt.Errorf("scorers: invalid numeric metric configuration")
	}
	return nil
}
func (m *Numeric) Score(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
	if err := s.Validate(m.Fields...); err != nil {
		return goeval.Score{}, err
	}
	values, err := sampleValues(s)
	if err != nil {
		return goeval.Score{}, err
	}
	return m.ScoreInput(ctx, values)
}

// ScoreInput corresponds to Python's metric.ascore(**inputs), allowing arbitrary
// named inputs and the same common {field} prompt substitutions.
func (m *Numeric) ScoreInput(ctx context.Context, values map[string]any) (goeval.Score, error) {
	if err := check.Context(ctx); err != nil {
		return goeval.Score{}, err
	}
	if err := m.Validate(); err != nil {
		return goeval.Score{}, err
	}
	if err := requiredValues(values, m.Fields); err != nil {
		return goeval.Score{}, err
	}
	instruction, err := prompt.Format(m.Definition, values)
	if err != nil {
		return goeval.Score{}, err
	}
	var out struct {
		Value  *float64 `json:"value"`
		Reason string   `json:"reason"`
	}
	err = goeval.GenerateJSON(ctx, m.LLM, goeval.Prompt{Instruction: instruction, Input: values, Schema: objectSchema(map[string]any{"value": map[string]any{"type": "number", "minimum": m.Min, "maximum": m.Max}, "reason": stringSchema()})}, &out)
	if err != nil {
		return goeval.Score{}, err
	}
	if out.Value == nil || math.IsNaN(*out.Value) || math.IsInf(*out.Value, 0) || *out.Value < m.Min || *out.Value > m.Max {
		return goeval.Score{}, fmt.Errorf("metrics: model score must be in [%g,%g]", m.Min, m.Max)
	}
	return goeval.Score{Value: *out.Value, Reason: out.Reason}, nil
}

type Discrete struct {
	LLM           goeval.LLM
	MetricName    string
	Definition    string
	AllowedValues []string
	Fields        []goeval.Field
}

func DiscreteMetric(llm goeval.LLM, name, definition string, values []string) *Discrete {
	if values == nil {
		values = []string{"pass", "fail"}
	}
	return &Discrete{LLM: llm, MetricName: name, Definition: definition, AllowedValues: append([]string(nil), values...)}
}
func (m *Discrete) Score(ctx context.Context, s goeval.Sample) (Judgment[string], error) {
	if err := s.Validate(m.Fields...); err != nil {
		return Judgment[string]{}, err
	}
	values, err := sampleValues(s)
	if err != nil {
		return Judgment[string]{}, err
	}
	return m.ScoreInput(ctx, values)
}
func (m *Discrete) ScoreInput(ctx context.Context, values map[string]any) (Judgment[string], error) {
	var zero Judgment[string]
	if m.MetricName == "" || m.Definition == "" || len(m.AllowedValues) == 0 {
		return zero, fmt.Errorf("metrics: incomplete discrete metric")
	}
	if err := requiredValues(values, m.Fields); err != nil {
		return zero, err
	}
	allowed := map[string]bool{}
	for _, v := range m.AllowedValues {
		if v == "" || allowed[v] {
			return zero, fmt.Errorf("metrics: discrete values must be nonempty and unique")
		}
		allowed[v] = true
	}
	instruction, err := prompt.Format(m.Definition, values)
	if err != nil {
		return zero, err
	}
	var out struct {
		Value  *string `json:"value"`
		Reason string  `json:"reason"`
	}
	err = goeval.GenerateJSON(ctx, m.LLM, goeval.Prompt{Instruction: instruction, Input: values, Schema: objectSchema(map[string]any{"value": map[string]any{"type": "string", "enum": m.AllowedValues}, "reason": stringSchema()})}, &out)
	if err != nil {
		return zero, err
	}
	if out.Value == nil || !allowed[*out.Value] {
		return zero, fmt.Errorf("metrics: disallowed categorical result")
	}
	return Judgment[string]{Value: *out.Value, Reason: out.Reason}, nil
}

// AsMetric requires an explicit numeric interpretation of every category.
func (m *Discrete) AsMetric(values map[string]float64) (goeval.Metric, error) {
	copied := map[string]float64{}
	for _, v := range m.AllowedValues {
		n, ok := values[v]
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("metrics: missing finite mapping for %q", v)
		}
		copied[v] = n
	}
	return goeval.MetricFunc{MetricName: m.MetricName, Fields: append([]goeval.Field(nil), m.Fields...), Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		v, err := m.Score(ctx, s)
		if err != nil {
			return goeval.Score{}, err
		}
		score, ok := copied[v.Value]
		if !ok {
			return goeval.Score{}, fmt.Errorf("metrics: category mapping changed after construction")
		}
		return goeval.Score{Value: score, Reason: v.Reason}, nil
	}}, nil
}

type Ranking struct {
	LLM        goeval.LLM
	MetricName string
	Definition string
	Count      int
	Items      []string
}

// RankingMetric takes Python's allowed_values item count. Passing []string
// instead is a Go extension that also enforces a permutation of exact labels.
func RankingMetric(llm goeval.LLM, name, definition string, allowed any) *Ranking {
	m := &Ranking{LLM: llm, MetricName: name, Definition: definition, Count: 2}
	switch v := allowed.(type) {
	case nil:
	case int:
		m.Count = v
	case []string:
		m.Items = append([]string{}, v...)
		m.Count = len(v)
	default:
		m.Count = -1
	}
	return m
}
func (m *Ranking) Score(ctx context.Context, s goeval.Sample) (Judgment[[]string], error) {
	values, err := sampleValues(s)
	if err != nil {
		return Judgment[[]string]{}, err
	}
	return m.ScoreInput(ctx, values)
}
func (m *Ranking) ScoreInput(ctx context.Context, values map[string]any) (Judgment[[]string], error) {
	var zero Judgment[[]string]
	if m.MetricName == "" || m.Definition == "" || m.Count < 0 {
		return zero, fmt.Errorf("metrics: incomplete ranking metric")
	}
	expected := map[string]bool{}
	for _, v := range m.Items {
		if v == "" || expected[v] {
			return zero, fmt.Errorf("metrics: ranking labels must be nonempty and unique")
		}
		expected[v] = true
	}
	input := map[string]any{}
	for k, v := range values {
		input[k] = v
	}
	if m.Items != nil {
		if _, ok := input["items"]; !ok {
			input["items"] = m.Items
		}
	}
	instruction, err := prompt.Format(m.Definition, input)
	if err != nil {
		return zero, err
	}
	itemSchema := stringSchema()
	if m.Items != nil {
		itemSchema["enum"] = m.Items
	}
	var out struct {
		Value  *[]string `json:"value"`
		Reason string    `json:"reason"`
	}
	err = goeval.GenerateJSON(ctx, m.LLM, goeval.Prompt{Instruction: instruction, Input: input, Schema: objectSchema(map[string]any{"value": arraySchema(itemSchema), "reason": stringSchema()})}, &out)
	if err != nil {
		return zero, err
	}
	if out.Value == nil || len(*out.Value) != m.Count {
		return zero, fmt.Errorf("metrics: wrong ranking length")
	}
	if m.Items != nil {
		seen := map[string]bool{}
		for _, v := range *out.Value {
			if !expected[v] || seen[v] {
				return zero, fmt.Errorf("metrics: invalid ranking item %q", v)
			}
			seen[v] = true
		}
	}
	return Judgment[[]string]{Value: *out.Value, Reason: out.Reason}, nil
}
