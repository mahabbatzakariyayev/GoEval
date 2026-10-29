package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mahabbatzakariyayev/GoEval"
)

func main() {
	shortAnswer := goeval.MetricFunc{
		MetricName: "short_answer",
		Fields:     []goeval.Field{goeval.Response},
		Evaluate: func(ctx context.Context, sample goeval.Sample) (goeval.Score, error) {
			if err := ctx.Err(); err != nil {
				return goeval.Score{}, err
			}
			words := len(strings.Fields(sample.Response))
			score := 0.0
			if words <= 8 {
				score = 1
			}
			return goeval.Score{Value: score, Reason: fmt.Sprintf("Answer has %d words; limit is 8.", words)}, nil
		},
	}
	gate, err := goeval.WithThreshold(shortAnswer, 1)
	if err != nil {
		log.Fatal(err)
	}
	evaluator, err := goeval.New(goeval.WithMetrics(gate))
	if err != nil {
		log.Fatal(err)
	}
	report, err := evaluator.Evaluate(context.Background(), []goeval.Case{
		{Output: "Paris is the capital of France."},
		{Output: "Paris is the capital of France and its largest city."},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Short answers:", report.Means["short_answer"])
	fmt.Println("All checks passed:", report.Passed())
}
