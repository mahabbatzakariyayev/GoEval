package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	"math"
	"math/rand/v2"
	"sort"
	"sync"
	"time"
)

// Score contains a numeric value, optional quality gate, and judgment evidence.
// Details must be JSON serializable when results are written as JSON.
type Score struct {
	Value     float64        `json:"value"`
	Reason    string         `json:"reason,omitempty"`
	Success   *bool          `json:"success,omitempty"`
	Threshold *float64       `json:"threshold,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// Validate checks numeric fields before results enter aggregate reporting.
func (s Score) Validate() error {
	if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
		return fmt.Errorf("goeval: metric returned a non-finite score")
	}
	if s.Threshold != nil && (math.IsNaN(*s.Threshold) || math.IsInf(*s.Threshold, 0)) {
		return fmt.Errorf("goeval: metric returned a non-finite threshold")
	}
	return nil
}

type MetricResult = Score

// ErrUndefined represents Python's NaN score without emitting invalid JSON.
var ErrUndefined = errors.New("goeval: score is undefined for empty input")

// Metric is a named evaluation that can be shared across concurrent rows.
// Name and RequiredFields must be stable; Score must honor cancellation.
// Metrics may additionally implement Validate() error and/or
// ValidateSample(Sample) error. The runner invokes these before any scoring.
// Validation must be deterministic and must not call a model.
type Metric interface {
	Name() string
	RequiredFields() []Field
	Score(context.Context, Sample) (Score, error)
}

// MetricFunc adapts a custom evaluator to Metric. Callbacks must be concurrency-safe.
type MetricFunc struct {
	MetricName string
	Fields     []Field
	Evaluate   func(context.Context, Sample) (Score, error)
}

func (m MetricFunc) Name() string            { return m.MetricName }
func (m MetricFunc) RequiredFields() []Field { return append([]Field(nil), m.Fields...) }
func (m MetricFunc) Validate() error {
	if m.Evaluate == nil {
		return fmt.Errorf("goeval: nil metric callback")
	}
	return nil
}
func (m MetricFunc) Score(ctx context.Context, s Sample) (Score, error) {
	if err := check.Context(ctx); err != nil {
		return Score{}, err
	}
	if m.Evaluate == nil {
		return Score{}, fmt.Errorf("goeval: nil metric callback")
	}
	if err := s.Validate(m.Fields...); err != nil {
		return Score{}, err
	}
	score, err := m.Evaluate(ctx, s)
	if err == nil {
		err = ctx.Err()
	}
	return score, err
}

type RunConfig struct {
	MaxWorkers int
	Timeout    time.Duration // Per score attempt. Zero inherits the caller's deadline.
	MaxRetries int           // Total attempts, as in Python. Zero means one attempt.
	RetryDelay time.Duration
	MaxWait    time.Duration // Exponential jitter cap when RetryDelay is zero.
	Seed       uint64
	FailFast   bool
}

// DefaultRunConfig matches Python RunConfig's timeout, attempt count, worker
// limit, retry wait cap, and seed. The zero value disables retries and deadlines.
func DefaultRunConfig() RunConfig {
	return RunConfig{MaxWorkers: 16, Timeout: 180 * time.Second, MaxRetries: 10, MaxWait: 60 * time.Second, Seed: 42}
}

type RowResult struct {
	Index  int               `json:"index"`
	Scores map[string]Score  `json:"scores"`
	Errors map[string]string `json:"errors,omitempty"`
	causes map[string]error
}

// Err joins execution failures while preserving errors.Is/errors.As for live
// results. Deserialized JSON retains messages but cannot reconstruct Go errors.
// Failed quality thresholds are handled separately by Check and AssertTest.
func (r RowResult) Err() error {
	names := make([]string, 0, len(r.Errors))
	for name := range r.Errors {
		names = append(names, name)
	}
	sort.Strings(names)
	failures := make([]error, 0, len(names))
	for _, name := range names {
		err := r.causes[name]
		if err == nil {
			err = errors.New(r.Errors[name])
		}
		failures = append(failures, &EvaluationError{Row: r.Index, Metric: name, Err: err})
	}
	return errors.Join(failures...)
}

type EvaluationResult struct {
	Rows  []RowResult        `json:"rows"`
	Means map[string]float64 `json:"means"`
}

// Err aggregates every row's execution errors without discarding partial scores.
func (r *EvaluationResult) Err() error {
	if r == nil {
		return nil
	}
	failures := make([]error, 0)
	for _, row := range r.Rows {
		if err := row.Err(); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

type EvaluationError struct {
	Row    int
	Metric string
	Err    error
}

func (e *EvaluationError) Error() string {
	return fmt.Sprintf("row %d, metric %s: %v", e.Row, e.Metric, e.Err)
}
func (e *EvaluationError) Unwrap() error { return e.Err }

// Evaluate retains input order and averages successful scores only. A failed
// metric has an Errors entry and no Scores entry; it is never silently zeroed.
// With FailFast, it returns the partial result and the first scoring error.
func Evaluate(ctx context.Context, d Dataset, metrics []Metric, cfg RunConfig) (*EvaluationResult, error) {
	if err := check.Context(ctx); err != nil {
		return nil, err
	}
	if cfg.MaxWorkers < 0 || cfg.MaxRetries < 0 || cfg.Timeout < 0 || cfg.RetryDelay < 0 || cfg.MaxWait < 0 {
		return nil, fmt.Errorf("goeval: negative run configuration")
	}
	prepared, err := prepareMetrics(metrics)
	if err != nil {
		return nil, err
	}
	for _, m := range prepared {
		for i, s := range d {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := m.ValidateSample(s); err != nil {
				return nil, &EvaluationError{i, m.Name(), err}
			}
		}
	}
	res := &EvaluationResult{Rows: make([]RowResult, len(d)), Means: map[string]float64{}}
	for i := range d {
		res.Rows[i] = RowResult{Index: i, Scores: map[string]Score{}}
	}
	workers := cfg.MaxWorkers
	if workers == 0 {
		workers = 16
	}
	if workers > len(d) {
		workers = len(d)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var wg sync.WaitGroup
	var first error
	var once sync.Once
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				for _, m := range prepared {
					if runCtx.Err() != nil {
						return
					}
					score, err := scoreWithRetry(runCtx, m, d[i], cfg)
					if err != nil {
						if res.Rows[i].Errors == nil {
							res.Rows[i].Errors = map[string]string{}
							res.Rows[i].causes = map[string]error{}
						}
						res.Rows[i].Errors[m.Name()] = err.Error()
						res.Rows[i].causes[m.Name()] = err
						if cfg.FailFast {
							once.Do(func() { first = &EvaluationError{i, m.Name(), err}; cancel() })
							return
						}
					} else {
						res.Rows[i].Scores[m.Name()] = score
					}
				}
			}
		}()
	}
send:
	for i := range d {
		select {
		case jobs <- i:
		case <-runCtx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()
	for _, m := range prepared {
		mean, n := 0.0, 0.0
		for _, row := range res.Rows {
			if s, ok := row.Scores[m.Name()]; ok {
				n++
				delta := s.Value - mean
				if math.IsInf(delta, 0) {
					// Overflow is possible only for opposite signs; the weighted
					// terms then have opposite signs and cannot overflow their sum.
					mean = mean*(1-1/n) + s.Value/n
				} else {
					mean += delta / n
				}
			}
		}
		if n > 0 {
			res.Means[m.Name()] = mean
		}
	}
	if first != nil {
		return res, first
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	return res, nil
}
func scoreWithRetry(ctx context.Context, m Metric, s Sample, cfg RunConfig) (score Score, err error) {
	attempts := max(cfg.MaxRetries, 1)
	rng := rand.New(rand.NewPCG(cfg.Seed, cfg.Seed+1))
	for attempt := 0; attempt < attempts; attempt++ {
		if ctx.Err() != nil {
			return Score{}, ctx.Err()
		}
		attemptCtx := ctx
		cancel := func() {}
		if cfg.Timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		}
		score, err = safeScore(attemptCtx, m, s)
		if err == nil {
			err = attemptCtx.Err()
		}
		cancel()
		if err == nil {
			err = score.Validate()
		}
		if err == nil {
			return score, nil
		}
		var panicErr *PanicError
		if errors.Is(err, context.Canceled) || errors.Is(err, ErrUndefined) || errors.As(err, &panicErr) || ctx.Err() != nil {
			return Score{}, err
		}
		delay := cfg.RetryDelay
		if delay == 0 && cfg.MaxWait > 0 {
			cap := math.Min(float64(cfg.MaxWait), float64(time.Second)*math.Pow(2, float64(min(attempt, 60))))
			delay = time.Duration(rng.Float64() * cap)
		}
		if attempt+1 < attempts && delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Score{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return Score{}, err
}
func safeScore(ctx context.Context, m Metric, s Sample) (score Score, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = &PanicError{Value: p}
		}
	}()
	return m.Score(ctx, s)
}
