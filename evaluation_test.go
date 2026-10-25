package goeval_test

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func TestEvaluationBoundedConcurrencyOrderAndMeans(t *testing.T) {
	var active, peak atomic.Int64
	m := goeval.MetricFunc{MetricName: "value", Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-ctx.Done():
			return goeval.Score{}, ctx.Err()
		case <-time.After(time.Millisecond):
		}
		v, err := strconv.ParseFloat(s.Response, 64)
		return goeval.Score{Value: v}, err
	}}
	d := make(goeval.Dataset, 30)
	for i := range d {
		d[i].Response = strconv.Itoa(i)
	}
	r, err := goeval.Evaluate(context.Background(), d, []goeval.Metric{m}, goeval.RunConfig{MaxWorkers: 3})
	if err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 3 || peak.Load() < 2 {
		t.Fatalf("concurrency %d", peak.Load())
	}
	if r.Means["value"] != 14.5 {
		t.Fatal(r.Means)
	}
	for i, row := range r.Rows {
		if row.Index != i || row.Scores["value"].Value != float64(i) {
			t.Fatalf("row order lost at %d", i)
		}
	}
}
func TestEvaluationFailuresExcludedAndRetries(t *testing.T) {
	var attempts atomic.Int64
	m := goeval.MetricFunc{MetricName: "retry", Evaluate: func(context.Context, goeval.Sample) (goeval.Score, error) {
		if attempts.Add(1) == 1 {
			return goeval.Score{}, errors.New("temporary")
		}
		return goeval.Score{Value: 0.8}, nil
	}}
	r, err := goeval.Evaluate(context.Background(), goeval.Dataset{{}}, []goeval.Metric{m}, goeval.RunConfig{MaxRetries: 2})
	if err != nil || attempts.Load() != 2 || r.Means["retry"] != 0.8 {
		t.Fatalf("%+v %v %d", r, err, attempts.Load())
	}
	m = goeval.MetricFunc{MetricName: "partial", Evaluate: func(_ context.Context, s goeval.Sample) (goeval.Score, error) {
		if s.Response == "bad" {
			return goeval.Score{}, errors.New("failed")
		}
		return goeval.Score{Value: 1}, nil
	}}
	r, err = goeval.Evaluate(context.Background(), goeval.Dataset{{Response: "bad"}, {Response: "good"}}, []goeval.Metric{m}, goeval.RunConfig{})
	if err != nil || r.Means["partial"] != 1 || r.Rows[0].Errors["partial"] == "" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, ok := r.Rows[0].Scores["partial"]; ok {
		t.Fatal("failure was assigned a score")
	}
}
func TestEvaluationCancellationTimeoutAndPanic(t *testing.T) {
	wait := goeval.MetricFunc{MetricName: "wait", Evaluate: func(ctx context.Context, _ goeval.Sample) (goeval.Score, error) {
		<-ctx.Done()
		return goeval.Score{}, ctx.Err()
	}}
	r, err := goeval.Evaluate(context.Background(), goeval.Dataset{{}}, []goeval.Metric{wait}, goeval.RunConfig{Timeout: time.Millisecond, FailFast: true})
	if !errors.Is(err, context.DeadlineExceeded) || r == nil {
		t.Fatalf("%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = goeval.Evaluate(ctx, goeval.Dataset{{}}, []goeval.Metric{wait}, goeval.RunConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	panicMetric := goeval.MetricFunc{MetricName: "panic", Evaluate: func(context.Context, goeval.Sample) (goeval.Score, error) { panic("boom") }}
	r, err = goeval.Evaluate(context.Background(), goeval.Dataset{{}}, []goeval.Metric{panicMetric}, goeval.RunConfig{})
	if err != nil || !strings.Contains(r.Rows[0].Errors["panic"], "boom") {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestEvaluationValidationAndNonfinite(t *testing.T) {
	d, err := goeval.ReadJSON(strings.NewReader(`[{"response":"answer"}]`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = goeval.Evaluate(context.Background(), d, []goeval.Metric{scorers.ExactMatch()}, goeval.RunConfig{})
	if err == nil {
		t.Fatal("missing reference accepted")
	}
	for _, ms := range [][]goeval.Metric{nil, {nil}, {scorers.ExactMatch(), scorers.ExactMatch()}} {
		if _, err := goeval.Evaluate(context.Background(), nil, ms, goeval.RunConfig{}); err == nil {
			t.Fatal("invalid metrics accepted")
		}
	}
	m := goeval.MetricFunc{MetricName: "nan", Evaluate: func(context.Context, goeval.Sample) (goeval.Score, error) {
		return goeval.Score{Value: math.NaN()}, nil
	}}
	r, err := goeval.Evaluate(context.Background(), goeval.Dataset{{}}, []goeval.Metric{m}, goeval.RunConfig{})
	if err != nil || len(r.Means) != 0 || r.Rows[0].Errors["nan"] == "" {
		t.Fatalf("%+v %v", r, err)
	}
}
