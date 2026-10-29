# Getting started

Run your first evaluation in a small Go application. This guide uses offline
text metrics, so you can follow it without a model, credentials, or external
service.

## Create a Go module

Install Go 1.23 or newer, then run:

```sh
mkdir my-evaluation
cd my-evaluation
go mod init example.com/my-evaluation
go get github.com/mahabbatzakariyayev/GoEval
```

The import path is case-sensitive. Its Go package name is `goeval`.

If you are working from a local checkout before publishing, point your consumer
module at it:

```sh
go mod edit -replace=github.com/mahabbatzakariyayev/GoEval=/absolute/path/to/GoEval
```

## Write the evaluation

Save the following program as `main.go`:

```go
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
```

## Run it

```sh
go run .
```

Expected output:

```text
Exact match: 0.5
Normalized match: 1
All checks passed: false
```

The first case passes both metrics. The second output has surrounding spaces:
exact matching fails, while normalized matching passes. The mean is the average
successful numeric score across cases.

## Use the default evaluator

`goeval.New()` selects exact matching with threshold 1. Supply `Output` and
`Expected` on every case. Use `WithMetrics` to choose another metric or combine
several. The constructor validates configuration before making model requests.

`Input` is the question, `Output` is the generated answer, and `Expected` is
the reference answer. Retrieved passages belong in `Context`.

## Choose the next step

Use [model-based evaluation](models.md) when text comparison is too narrow.
Use [datasets and reports](datasets.md) to evaluate many saved cases.
Use [testing and CI](testing.md) to make a quality gate fail your test suite.
