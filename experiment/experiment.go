// Package experiment runs an application against a dataset before evaluation.
package experiment

import (
	"context"
	"errors"
	"fmt"
	"sync"

	goeval "github.com/mahabbatzakariyayev/GoEval"
)

type Output struct {
	Response            string
	RetrievedContexts   []string
	RetrievedContextIDs []string
	Messages            []goeval.Message
}
type Application func(context.Context, goeval.Sample) (Output, error)
type Result struct {
	Name              string                   `json:"name"`
	Dataset           goeval.Dataset           `json:"dataset"`
	ApplicationErrors map[int]string           `json:"application_errors,omitempty"`
	Evaluation        *goeval.EvaluationResult `json:"evaluation,omitempty"`
}

// Run retains references, fills application outputs, and evaluates successful
// application rows. Evaluation row indices are remapped to the input dataset.
// Failed application rows are never evaluated with stale responses.
func Run(ctx context.Context, name string, d goeval.Dataset, app Application, metrics []goeval.Metric, cfg goeval.RunConfig) (*Result, error) {
	if ctx == nil {
		return nil, fmt.Errorf("experiment: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if app == nil || name == "" {
		return nil, fmt.Errorf("experiment: name and application are required")
	}
	if cfg.MaxWorkers < 0 || cfg.MaxRetries < 0 || cfg.Timeout < 0 || cfg.RetryDelay < 0 || cfg.MaxWait < 0 {
		return nil, fmt.Errorf("experiment: negative run configuration")
	}
	if err := goeval.ValidateMetrics(metrics); err != nil {
		return nil, err
	}
	result := &Result{Name: name, Dataset: make(goeval.Dataset, len(d)), ApplicationErrors: map[int]string{}}
	copy(result.Dataset, d)
	outputs := make([]Output, len(d))
	errs := make([]error, len(d))
	completed := make([]bool, len(d))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := cfg.MaxWorkers
	if workers == 0 {
		workers = 16
	}
	workers = min(workers, len(d))
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if runCtx.Err() != nil {
					errs[i] = runCtx.Err()
					continue
				}
				attemptCtx := runCtx
				done := func() {}
				if cfg.Timeout > 0 {
					attemptCtx, done = context.WithTimeout(runCtx, cfg.Timeout)
				}
				outputs[i], errs[i] = safeApplication(attemptCtx, app, d[i])
				if errs[i] == nil {
					errs[i] = attemptCtx.Err()
				}
				completed[i] = true
				done()
				if errs[i] != nil && cfg.FailFast {
					cancel()
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
	success := goeval.Dataset{}
	indices := []int{}
	joined := []error{}
	for i, out := range outputs {
		if !completed[i] && errs[i] == nil && runCtx.Err() != nil {
			errs[i] = runCtx.Err()
		}
		result.Dataset[i].Response = ""
		result.Dataset[i].RetrievedContexts = nil
		result.Dataset[i].RetrievedContextIDs = nil
		if errs[i] != nil {
			result.ApplicationErrors[i] = errs[i].Error()
			joined = append(joined, fmt.Errorf("application row %d: %w", i, errs[i]))
			continue
		}
		row := result.Dataset[i]
		row = row.WithResponse(out.Response)
		row.RetrievedContexts = out.RetrievedContexts
		row.RetrievedContextIDs = out.RetrievedContextIDs
		if out.Messages != nil {
			row.Messages = out.Messages
			row.UserInput = ""
		}
		result.Dataset[i] = row
		success = append(success, row)
		indices = append(indices, i)
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if cfg.FailFast && len(joined) > 0 {
		return result, errors.Join(joined...)
	}
	eval, err := goeval.Evaluate(ctx, success, metrics, cfg)
	if eval != nil {
		for i := range eval.Rows {
			eval.Rows[i].Index = indices[i]
		}
	}
	result.Evaluation = eval
	if err != nil {
		var e *goeval.EvaluationError
		if errors.As(err, &e) && e.Row < len(indices) {
			err = &goeval.EvaluationError{Row: indices[e.Row], Metric: e.Metric, Err: e.Err}
		}
		joined = append(joined, err)
	} else if eval != nil {
		// Row indices were remapped above; retain their errors and causes.
		joined = append(joined, eval.Err())
	}
	return result, errors.Join(joined...)
}
func safeApplication(ctx context.Context, app Application, s goeval.Sample) (out Output, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("experiment: application panic: %v", p)
		}
	}()
	return app(ctx, s)
}
