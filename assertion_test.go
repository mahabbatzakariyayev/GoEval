package goeval_test

import (
	"context"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/judge"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

type testReporter struct {
	errors  []string
	helpers int
}

func (r *testReporter) Helper()                           { r.helpers++ }
func (r *testReporter) Errorf(format string, args ...any) { r.errors = append(r.errors, format) }

func TestLibraryAssertions(t *testing.T) {
	ctx := context.Background()
	c := judge.Case{ActualOutput: "Paris", ExpectedOutput: "Paris"}
	gate, err := goeval.WithThreshold(scorers.ExactMatch(), 1)
	if err != nil {
		t.Fatal(err)
	}
	checks := []goeval.Metric{gate, judge.ExactMatchMetric(judge.Options{})}
	if _, err := goeval.Check(ctx, c.ToSample(), checks, goeval.RunConfig{}); err != nil {
		t.Fatal(err)
	}
	reporter := &testReporter{}
	goeval.AssertTest(reporter, ctx, c.ToSample(), checks)
	if len(reporter.errors) != 0 || reporter.helpers == 0 {
		t.Fatal(reporter)
	}
	c.ActualOutput = "Rome"
	r, err := goeval.Check(ctx, c.ToSample(), checks, goeval.RunConfig{})
	if err == nil || !strings.Contains(err.Error(), "exact_match") || len(r.Rows[0].Scores) != 2 {
		t.Fatal(r, err)
	}
	goeval.AssertTest(reporter, ctx, c.ToSample(), checks)
	if len(reporter.errors) != 1 {
		t.Fatal(reporter)
	}
	if _, err := goeval.Check(ctx, c.ToSample(), []goeval.Metric{scorers.ExactMatch()}, goeval.RunConfig{}); err == nil || !strings.Contains(err.Error(), "WithThreshold") {
		t.Fatal(err)
	}
	judge.AssertTest(reporter, ctx, judge.Case{ActualOutput: "x", ExpectedOutput: "x"}, []judge.Metric{judge.ExactMatchMetric(judge.Options{})})
	if len(reporter.errors) != 1 {
		t.Fatal(reporter)
	}
}
