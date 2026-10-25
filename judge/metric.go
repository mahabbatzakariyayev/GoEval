package judge

import (
	"context"
	"fmt"
	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	"math"
	"sort"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

// Options uses pointer fields where false/zero differs from the default.
// Every judge uses higher-is-better scores, including hallucination, bias,
// and toxicity checks.
type Options struct {
	Threshold               *float64
	IncludeReason           *bool
	StrictMode              bool
	TruthsExtractionLimit   *int
	PenalizeAmbiguousClaims bool
}

func Float(v float64) *float64 { return &v }
func Bool(v bool) *bool        { return &v }

func cloneOptions(opts Options) Options {
	opts.Threshold = check.CopyPtr(opts.Threshold)
	opts.IncludeReason = check.CopyPtr(opts.IncludeReason)
	opts.TruthsExtractionLimit = check.CopyPtr(opts.TruthsExtractionLimit)
	return opts
}

type Result struct {
	Metric    string         `json:"metric"`
	Score     float64        `json:"score"`
	Reason    string         `json:"reason,omitempty"`
	Threshold float64        `json:"threshold"`
	Success   bool           `json:"success"`
	Details   map[string]any `json:"details,omitempty"`
}
type Metric interface {
	goeval.Metric
	Measure(context.Context, Case) (Result, error)
	DisplayName() string
}
type evaluator struct {
	name, display    string
	params           []SingleTurnParam
	opts             Options
	defaultThreshold float64
	measure          func(context.Context, Case, float64) (float64, string, map[string]any, error)
	model            goeval.LLM
	requiresModel    bool
	configErr        error
	validateCase     func(Case) error
}

func (m *evaluator) Name() string        { return "judge." + m.name }
func (m *evaluator) DisplayName() string { return m.display }

// Validate checks options before an evaluation starts making model requests.
func (m *evaluator) Validate() error {
	if m.configErr != nil {
		return m.configErr
	}
	if m.requiresModel && check.Nil(m.model) {
		return fmt.Errorf("judge: %s requires a model", m.name)
	}
	if m.opts.Threshold != nil {
		v := *m.opts.Threshold
		if v < 0 || v > 1 || math.IsNaN(v) {
			return fmt.Errorf("judge: threshold must be in [0,1]")
		}
	}
	return nil
}
func (m *evaluator) RequiredFields() []goeval.Field {
	fields := []goeval.Field{}
	for _, p := range m.params {
		switch p {
		case Input:
			fields = append(fields, goeval.UserInput)
		case ActualOutput:
			fields = append(fields, goeval.Response)
		case ExpectedOutput:
			fields = append(fields, goeval.Reference)
		case Context:
			fields = append(fields, goeval.ReferenceContexts)
		case RetrievalContext:
			fields = append(fields, goeval.RetrievedContexts)
		}
	}
	return fields
}
func (m *evaluator) Measure(ctx context.Context, c Case) (Result, error) {
	zero := Result{}
	if err := check.Context(ctx); err != nil {
		return zero, err
	}
	if err := m.Validate(); err != nil {
		return zero, err
	}
	if err := c.Validate(m.params...); err != nil {
		return zero, err
	}
	if m.validateCase != nil {
		if err := m.validateCase(c); err != nil {
			return zero, err
		}
	}
	threshold := m.defaultThreshold
	if m.opts.Threshold != nil {
		threshold = *m.opts.Threshold
	}
	if threshold < 0 || threshold > 1 || math.IsNaN(threshold) {
		return zero, fmt.Errorf("judge: threshold must be in [0,1]")
	}
	if m.opts.StrictMode {
		threshold = 1
	}
	value, reason, details, err := m.measure(ctx, c, threshold)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return zero, fmt.Errorf("judge: non-finite metric score")
	}
	if m.opts.IncludeReason != nil && !*m.opts.IncludeReason {
		reason = ""
	}
	return Result{Metric: m.Name(), Score: value, Reason: reason, Threshold: threshold, Success: value >= threshold, Details: details}, nil
}

// ValidateSample includes Judge-only fields such as ToolsCalled in the shared
// runner's preflight, before any metric begins making model requests.
func (m *evaluator) ValidateSample(s goeval.Sample) error {
	if err := s.Validate(m.RequiredFields()...); err != nil {
		return err
	}
	c, err := FromSample(s)
	if err != nil {
		return err
	}
	if err := c.Validate(m.params...); err != nil {
		return err
	}
	if m.validateCase != nil {
		return m.validateCase(c)
	}
	return nil
}
func (m *evaluator) Score(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
	if err := m.ValidateSample(s); err != nil {
		return goeval.Score{}, err
	}
	c, err := FromSample(s)
	if err != nil {
		return goeval.Score{}, err
	}
	r, err := m.Measure(ctx, c)
	if err != nil {
		return goeval.Score{}, err
	}
	return goeval.Score{Value: r.Score, Reason: r.Reason, Success: &r.Success, Threshold: &r.Threshold, Details: r.Details}, nil
}
func newMetric(name, display string, params []SingleTurnParam, opts Options, fn func(context.Context, Case, float64) (float64, string, map[string]any, error)) *evaluator {
	return &evaluator{name: name, display: display, params: append([]SingleTurnParam(nil), params...), opts: cloneOptions(opts), defaultThreshold: 0.5, measure: fn}
}
func includeReason(opts Options) bool { return opts.IncludeReason == nil || *opts.IncludeReason }
func newModelMetric(model goeval.LLM, name, display string, params []SingleTurnParam, opts Options, fn func(context.Context, Case, float64) (float64, string, map[string]any, error)) *evaluator {
	m := newMetric(name, display, params, opts, fn)
	m.model, m.requiresModel = model, true
	return m
}
func strictScore(value, threshold float64, opts Options) float64 {
	if opts.StrictMode && value < threshold {
		return 0
	}
	return value
}

func schema(properties map[string]any) map[string]any {
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return map[string]any{"type": "object", "properties": properties, "required": keys, "additionalProperties": false}
}
func str() map[string]any           { return map[string]any{"type": "string"} }
func array(item any) map[string]any { return map[string]any{"type": "array", "items": item} }
