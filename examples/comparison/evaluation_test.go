package main

import (
	"context"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/judge"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func TestAnswer(t *testing.T) {
	gate, err := goeval.WithThreshold(scorers.ExactMatch(), 1)
	if err != nil {
		t.Fatal(err)
	}
	c := judge.Case{Input: "Capital of France?", ActualOutput: "Paris", ExpectedOutput: "Paris"}
	goeval.AssertTest(t, context.Background(), c.ToSample(), []goeval.Metric{
		gate, judge.ExactMatchMetric(judge.Options{}),
	})
}
