# Custom metrics and judges

Extend GoEval with a deterministic rule or a model-based criterion. Both can
participate in the same evaluator and quality gates as built-in metrics.

## Write a deterministic metric

Use MetricFunc to adapt a function. This complete example checks an eight-word
answer limit:

```go
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
```

Run it from the checkout:

```sh
go run ./examples/custom-metric
```

Expected output:

```text
Short answers: 0.5
All checks passed: false
```

The callback is evaluated concurrently across cases. It checks cancellation,
declares its required field, and returns a numeric score and reason. WithThreshold
adds the pass/fail decision.

## Implement the full interface

A metric implements Name, RequiredFields, and Score. Names must be stable,
nonempty, and unique within an evaluator. RequiredFields declares which sample
fields preflight must check.

Optional `Validate() error` and `ValidateSample(goeval.Sample) error` methods
are also called during preflight. Validation must not call a model.
Numeric results and thresholds must be finite.

Case metadata is available directly through Sample.Metadata. Keep input
collections and nested metadata read-only during a run.

## Define a model criterion

Create a criterion and pass it to WithMetrics:

```go
metric := goeval.Judge(client, "helpfulness",
    "The answer directly addresses the request with actionable details.",
    goeval.Threshold(0.8),
)
evaluator, err := goeval.New(goeval.WithMetrics(metric))
```

This fragment uses an already configured client. See
[model-based evaluation](models.md) for a complete provider setup.

Judge uses Input and Output by default. Add AgainstExpected to include Expected.
Evaluation steps are generated once and reused by that metric instance.

## Use advanced criteria

The judge package exposes GEvalOptions for supplied steps, rubric score ranges,
evaluation fields, additional context, and probability weighting.

Root Judge uses raw-score judgments by default. LogProbs opts into weighting
when the provider supports that capability. Some compatible endpoints do not
provide token probabilities.

Use decision graphs for explicit branching workflows, or the scorers package's
Numeric, Discrete, and Ranking APIs when you need typed result schemas.
The [package reference](../reference.md#advanced-packages) links to their Go APIs.
