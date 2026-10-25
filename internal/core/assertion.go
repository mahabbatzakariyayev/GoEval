package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	"math"
)

type thresholdMetric struct {
	Metric
	threshold float64
}

func (m thresholdMetric) Validate() error {
	if validator, ok := m.Metric.(interface{ Validate() error }); ok {
		return validator.Validate()
	}
	return nil
}
func (m thresholdMetric) ValidateSample(s Sample) error {
	if validator, ok := m.Metric.(interface{ ValidateSample(Sample) error }); ok {
		return validator.ValidateSample(s)
	}
	return s.Validate(m.RequiredFields()...)
}

// WithThreshold turns any score into a higher-is-better pass/fail gate, including
// GoEval scorers. The original name and required dataset fields are preserved.
func WithThreshold(metric Metric, threshold float64) (Metric, error) {
	if check.Nil(metric) || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return nil, fmt.Errorf("goeval: a metric and finite threshold are required")
	}
	return thresholdMetric{Metric: metric, threshold: threshold}, nil
}

func (m thresholdMetric) Score(ctx context.Context, sample Sample) (Score, error) {
	score, err := m.Metric.Score(ctx, sample)
	if err != nil {
		return Score{}, err
	}
	success := score.Value >= m.threshold
	score.Threshold, score.Success = &m.threshold, &success
	return score, nil
}

// TestingT is satisfied by *testing.T and *testing.B. No testing dependency is
// needed by applications that import the library.
type TestingT interface {
	Helper()
	Errorf(string, ...any)
}

// Check evaluates one sample and returns errors for execution failures, failed
// thresholds, or metrics without a threshold. Use WithThreshold for score-only
// scorers. Partial scores are retained for inspection.
func Check(ctx context.Context, sample Sample, metrics []Metric, cfg RunConfig) (*EvaluationResult, error) {
	result, err := Evaluate(ctx, Dataset{sample}, metrics, cfg)
	if err != nil {
		return result, err
	}
	failures := []error{}
	row := result.Rows[0]
	if err := row.Err(); err != nil {
		failures = append(failures, err)
	}
	for _, metric := range metrics {
		name := metric.Name()
		if _, ok := row.Errors[name]; ok {
			continue
		}
		score := row.Scores[name]
		if score.Success == nil || score.Threshold == nil {
			failures = append(failures, fmt.Errorf("%s: assertion requires a threshold; use WithThreshold", name))
		} else if !*score.Success {
			failures = append(failures, &ThresholdError{Metric: name, Score: score.Value, Threshold: *score.Threshold, Reason: score.Reason})
		}
	}
	return result, errors.Join(failures...)
}

// AssertTest integrates evaluation with Go's testing package. It reports all
// failing gates and retains ordinary Go test control flow (it does not Fatal).
func AssertTest(t TestingT, ctx context.Context, sample Sample, metrics []Metric) {
	t.Helper()
	if _, err := Check(ctx, sample, metrics, RunConfig{}); err != nil {
		t.Errorf("evaluation failed: %v", err)
	}
}
