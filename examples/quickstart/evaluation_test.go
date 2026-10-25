package main

import (
	"context"
	"testing"

	"github.com/mahabbatzakariyayev/GoEval"
)

func TestAnswer(t *testing.T) {
	evaluator, err := goeval.New()
	if err != nil {
		t.Fatal(err)
	}
	evaluator.Assert(t, context.Background(), goeval.Case{Input: "Capital of France?", Output: "Paris", Expected: "Paris"})
}
