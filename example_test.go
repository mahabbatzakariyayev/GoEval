package goeval_test

import (
	"context"
	"fmt"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/judge"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func ExampleEvaluate() {
	cases := judge.Dataset{{Input: "Capital of France?", ActualOutput: " Paris ", ExpectedOutput: "Paris"}}
	result, err := goeval.Evaluate(context.Background(), cases.Samples(), []goeval.Metric{
		scorers.ExactMatch(), judge.ExactMatchMetric(judge.Options{}),
	}, goeval.RunConfig{})
	if err != nil {
		panic(err)
	}
	if err := result.Err(); err != nil {
		panic(err)
	}
	fmt.Println(result.Means["exact_match"])
	fmt.Println(result.Means["judge.exact_match"])
	// Output:
	// 0
	// 1
}

func ExampleCheck() {
	gate, err := goeval.WithThreshold(scorers.ExactMatch(), 1)
	if err != nil {
		panic(err)
	}
	_, err = goeval.Check(context.Background(), goeval.Sample{Response: "Paris", Reference: "Paris"}, []goeval.Metric{gate}, goeval.RunConfig{})
	fmt.Println(err == nil)
	// Output: true
}
