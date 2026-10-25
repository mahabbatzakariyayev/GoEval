package judge

import (
	"context"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

// Evaluate uses the shared runner, including bounded concurrency, cancellation,
// per-attempt timeouts and retries. Scores retain threshold and success fields.
func Evaluate(ctx context.Context, dataset Dataset, metrics []Metric, cfg goeval.RunConfig) (*goeval.EvaluationResult, error) {
	return goeval.Evaluate(ctx, dataset.Samples(), sharedMetrics(metrics), cfg)
}

func sharedMetrics(metrics []Metric) []goeval.Metric {
	out := make([]goeval.Metric, len(metrics))
	for i, m := range metrics {
		out[i] = m
	}
	return out
}

// AssertTest is the native Go counterpart of assert_test. Use goeval.AssertTest
// with Case.ToSample when combining Judge and GoEval gates in one test.
func AssertTest(t goeval.TestingT, ctx context.Context, c Case, metrics []Metric) {
	t.Helper()
	goeval.AssertTest(t, ctx, c.ToSample(), sharedMetrics(metrics))
}
