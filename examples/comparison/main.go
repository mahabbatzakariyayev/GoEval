// This example combines advanced scorers and judges without model requests.
package main

import (
	"context"
	"fmt"
	"log"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/judge"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func main() {
	cases := judge.Dataset{
		{Input: "Capital of France?", ActualOutput: "Paris", ExpectedOutput: "Paris"},
		{Input: "Capital of Italy?", ActualOutput: " Rome ", ExpectedOutput: "Rome"},
	}
	result, err := goeval.Evaluate(context.Background(), cases.Samples(), []goeval.Metric{
		scorers.ExactMatch(),
		judge.ExactMatchMetric(judge.Options{}),
	}, goeval.RunConfig{MaxWorkers: 4})
	if err != nil {
		log.Fatal(err)
	}
	// These metrics differ in their treatment of surrounding whitespace.
	fmt.Printf("Raw exact match: %.1f\n", result.Means["exact_match"])
	fmt.Printf("Normalized exact match: %.1f\n", result.Means["judge.exact_match"])
}
