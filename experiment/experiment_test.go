package experiment

import (
	"context"
	"errors"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func TestRunPreservesReferencesAndRemapsRows(t *testing.T) {
	d, err := goeval.ReadJSON(strings.NewReader(`[{"user_input":"good","reference":"Paris"},{"user_input":"bad","reference":"Paris","response":"stale"},{"user_input":"good2","reference":"Paris"}]`))
	if err != nil {
		t.Fatal(err)
	}
	app := func(_ context.Context, s goeval.Sample) (Output, error) {
		if s.UserInput == "bad" {
			return Output{}, errors.New("application failed")
		}
		return Output{Response: "Paris", RetrievedContexts: []string{"Paris is the capital of France"}}, nil
	}
	r, err := Run(context.Background(), "test", d, app, []goeval.Metric{scorers.ExactMatch()}, goeval.RunConfig{MaxWorkers: 2})
	if err == nil || len(r.ApplicationErrors) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if len(r.Evaluation.Rows) != 2 || r.Evaluation.Rows[1].Index != 2 || r.Evaluation.Means["exact_match"] != 1 {
		t.Fatalf("%+v", r.Evaluation)
	}
	if r.Dataset[1].Response != "" || d[1].Response != "stale" {
		t.Fatal("failed output retained or input mutated")
	}
	if d[0].Response != "" || r.Dataset[0].Reference != "Paris" {
		t.Fatal("reference/input changed")
	}
}
func TestApplicationPanicBecomesError(t *testing.T) {
	app := func(context.Context, goeval.Sample) (Output, error) { panic("bad application") }
	r, err := Run(context.Background(), "panic", goeval.Dataset{{}}, app, []goeval.Metric{scorers.ExactMatch()}, goeval.RunConfig{FailFast: true})
	if err == nil || r.ApplicationErrors[0] == "" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestFailFastRetainsCompletedEmptyResponse(t *testing.T) {
	app := func(_ context.Context, s goeval.Sample) (Output, error) {
		if s.UserInput == "fail" {
			return Output{}, errors.New("failed")
		}
		return Output{Response: ""}, nil
	}
	r, err := Run(context.Background(), "empty", goeval.Dataset{{UserInput: "empty"}, {UserInput: "fail"}}, app, []goeval.Metric{scorers.ExactMatch()}, goeval.RunConfig{MaxWorkers: 1, FailFast: true})
	if err == nil || len(r.ApplicationErrors) != 1 || r.ApplicationErrors[1] == "" {
		t.Fatal(r, err)
	}
	if _, bad := r.ApplicationErrors[0]; bad {
		t.Fatal("successful empty output marked cancelled", r.ApplicationErrors)
	}
}

func TestInvalidMetricsAreRejectedBeforeApplicationRuns(t *testing.T) {
	called := false
	app := func(context.Context, goeval.Sample) (Output, error) { called = true; return Output{}, nil }
	if _, err := Run(context.Background(), "invalid", goeval.Dataset{{}}, app, []goeval.Metric{nil}, goeval.RunConfig{}); err == nil || called {
		t.Fatal("application ran before metric validation", err)
	}
}

func TestRunReturnsMetricFailuresWithOriginalRowIndices(t *testing.T) {
	appErr := errors.New("application unavailable")
	metricErr := errors.New("judge unavailable")
	app := func(_ context.Context, s goeval.Sample) (Output, error) {
		if s.UserInput == "skip" {
			return Output{}, appErr
		}
		return Output{Response: s.UserInput}, nil
	}
	m := goeval.MetricFunc{MetricName: "judge", Evaluate: func(_ context.Context, s goeval.Sample) (goeval.Score, error) {
		if s.Response == "bad" {
			return goeval.Score{}, metricErr
		}
		return goeval.Score{Value: 1}, nil
	}}
	r, err := Run(context.Background(), "partial", goeval.Dataset{{UserInput: "skip"}, {UserInput: "good"}, {UserInput: "bad"}}, app, []goeval.Metric{m}, goeval.RunConfig{})
	if !errors.Is(err, appErr) || !errors.Is(err, metricErr) {
		t.Fatal("failure was hidden", err)
	}
	var rowErr *goeval.EvaluationError
	if !errors.As(err, &rowErr) || rowErr.Row != 2 || rowErr.Metric != "judge" {
		t.Fatal("wrong error row", err)
	}
	if r.Evaluation.Means["judge"] != 1 || r.Evaluation.Rows[1].Index != 2 {
		t.Fatal(r.Evaluation)
	}
}
