package goeval

import (
	"context"
	"fmt"
	"time"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
)

// Config configures a reusable evaluator. The default metric is ExactMatch,
// with 16 concurrent rows, one attempt and the caller's context deadline.
type Config struct {
	Metrics []Metric
	Run     RunConfig
}

// Option configures an evaluator before construction.
type Option func(*Config) error

// WithMetrics selects scorers. Calling it with no metrics is an error.
func WithMetrics(metrics ...Metric) Option {
	copy := append([]Metric{}, metrics...)
	return func(cfg *Config) error { cfg.Metrics = append([]Metric{}, copy...); return nil }
}

// WithConcurrency limits simultaneously evaluated cases. Zero uses 16.
func WithConcurrency(workers int) Option {
	return func(cfg *Config) error { cfg.Run.MaxWorkers = workers; return nil }
}

// WithTimeout sets a deadline per metric attempt. Zero uses the caller's deadline.
func WithTimeout(timeout time.Duration) Option {
	return func(cfg *Config) error { cfg.Run.Timeout = timeout; return nil }
}

// WithRunConfig supplies timeout, retry, fail-fast and worker settings together.
func WithRunConfig(run RunConfig) Option {
	return func(cfg *Config) error { cfg.Run = run; return nil }
}

// Evaluator is safe to reuse across concurrent calls when its metrics/providers
// are concurrency-safe. Evaluation results and thresholds belong to each run.
type Evaluator struct {
	metrics []Metric
	run     RunConfig
}

// New builds an evaluator and validates configuration without model requests.
func New(options ...Option) (*Evaluator, error) {
	cfg := Config{}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("goeval: nil evaluator option")
		}
		if err := option(&cfg); err != nil {
			return nil, err
		}
	}
	if cfg.Metrics == nil {
		cfg.Metrics = []Metric{ExactMatch()}
	}
	if err := ValidateMetrics(cfg.Metrics); err != nil {
		return nil, err
	}
	r := cfg.Run
	if r.MaxWorkers < 0 || r.Timeout < 0 || r.MaxRetries < 0 || r.RetryDelay < 0 || r.MaxWait < 0 {
		return nil, fmt.Errorf("goeval: negative run configuration")
	}
	return &Evaluator{metrics: append([]Metric(nil), cfg.Metrics...), run: cfg.Run}, nil
}

// Evaluate scores friendly cases and returns a report. Execution failures are
// returned as errors even when a run continues, with partial scores preserved.
// Failed thresholds remain valid scores; report.Passed reports quality gates.
func (e *Evaluator) Evaluate(ctx context.Context, cases []Case) (*Report, error) {
	if err := check.Context(ctx); err != nil {
		return nil, err
	}
	return e.EvaluateDataset(ctx, Cases(cases).Samples())
}

// EvaluateDataset accepts existing compatibility datasets with the same runner.
func (e *Evaluator) EvaluateDataset(ctx context.Context, data Dataset) (*Report, error) {
	if e == nil || len(e.metrics) == 0 {
		return nil, fmt.Errorf("goeval: evaluator must be constructed with New")
	}
	r, err := Evaluate(ctx, data, e.metrics, e.run)
	if r == nil {
		return nil, err
	}
	if err == nil {
		err = r.Err()
	}
	names := make([]string, len(e.metrics))
	for i, m := range e.metrics {
		names[i] = m.Name()
	}
	return &Report{EvaluationResult: r, metrics: names, runErr: err}, err
}

// Check evaluates one case and returns threshold or execution failures.
func (e *Evaluator) Check(ctx context.Context, c Case) (*Report, error) {
	if e == nil || len(e.metrics) == 0 {
		return nil, fmt.Errorf("goeval: evaluator must be constructed with New")
	}
	r, err := Check(ctx, c.Sample(), e.metrics, e.run)
	if r == nil {
		return nil, err
	}
	names := make([]string, len(e.metrics))
	for i, m := range e.metrics {
		names[i] = m.Name()
	}
	return &Report{EvaluationResult: r, metrics: names, runErr: err}, err
}

// Assert evaluates one case through Go's testing interface.
func (e *Evaluator) Assert(t TestingT, ctx context.Context, c Case) {
	t.Helper()
	if _, err := e.Check(ctx, c); err != nil {
		t.Errorf("evaluation failed: %v", err)
	}
}

// Report embeds scores, means and execution errors. Use Passed for quality gates.
// Its JSON representation includes rows, means, metric names and run errors,
// preserving quality-gate decisions when a report is saved and loaded.
type Report struct {
	*EvaluationResult
	metrics []string
	runErr  error
}

// Passed requires at least one case and successful, configured gates for every
// requested metric. Score-only custom metrics need WithThreshold for this check.
func (r *Report) Passed() bool {
	if r == nil || r.EvaluationResult == nil || len(r.Rows) == 0 || len(r.metrics) == 0 || r.runErr != nil || r.Err() != nil {
		return false
	}
	for _, row := range r.Rows {
		for _, name := range r.metrics {
			score, ok := row.Scores[name]
			if !ok || score.Success == nil || score.Threshold == nil || !*score.Success {
				return false
			}
		}
	}
	return true
}
