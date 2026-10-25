package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mahabbatzakariyayev/GoEval"
)

func main() {
	evaluator, err := goeval.New(goeval.WithMetrics(goeval.ExactMatch(), goeval.NormalizedMatch()))
	if err != nil {
		log.Fatal(err)
	}
	report, err := evaluator.Evaluate(context.Background(), []goeval.Case{
		{Input: "Capital of France?", Output: "Paris", Expected: "Paris"},
		{Input: "Capital of Italy?", Output: " Rome ", Expected: "Rome"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Exact match:", report.Means["exact_match"])
	fmt.Println("Normalized match:", report.Means["normalized_match"])
	fmt.Println("All checks passed:", report.Passed())
}
