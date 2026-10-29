package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/judge"
)

func main() {
	metric := judge.ToolCorrectnessMetric(nil, judge.ToolCorrectnessOptions{
		EvaluationParams: []judge.ToolCallParam{judge.InputParameters},
		ShouldExactMatch: true,
	})
	evaluator, err := goeval.New(goeval.WithMetrics(metric))
	if err != nil {
		log.Fatal(err)
	}
	report, err := evaluator.Check(context.Background(), goeval.Case{
		Input:         "Search for Paris",
		Tools:         []goeval.Tool{{Name: "search", Arguments: map[string]any{"q": "Paris"}}},
		ExpectedTools: []goeval.Tool{{Name: "search", Arguments: map[string]any{"q": "Paris"}}},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Tool arguments match:", report.Passed())
}
