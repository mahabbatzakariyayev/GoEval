# Testing and CI

Put evaluation gates next to your Go tests. Start with deterministic metrics so
the basic test suite runs without credentials or model requests.

## Add an offline test

Save this as `evaluation_test.go` in your consumer module:

```go
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
```

Run it with:

```sh
go test ./...
```

`Assert` reports a failure through the test interface when execution fails,
a quality gate fails, or a selected metric lacks a threshold.

## Evaluate a batch as a gate

Use Evaluator.Evaluate for a batch. Check its returned error, then require
`report.Passed()` to be true. An empty dataset does not pass, and a score-only
custom metric needs `goeval.WithThreshold`.

Use Evaluator.Check for a single case when you want a returned
`*goeval.ThresholdError`. Inspect the metric name, score, threshold, and reason
with `errors.As`.

## Separate model-dependent tests

Model judgments can vary between requests. Keep tests that call your provider
separate from deterministic checks, and opt into them explicitly in your CI
environment. Configure the model and credentials through that environment.

For stable unit tests, inject `goeval.LLMFunc` with predetermined JSON responses
or use a local HTTP test server. The repository's integration tests exercise
provider transport, model stages, caching, retries, and reports this way.

## Add a consumer CI job

A basic workflow for your application can run the same Go test command:

```yaml
name: Evaluation
on: [push, pull_request]
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go test -race ./...
```

This workflow assumes the selected tests run offline. Add provider credentials
only to a separate, intentionally configured model-evaluation job.

## Inspect failures

An execution error can indicate missing inputs, invalid model JSON, a provider
error, or cancellation. A threshold error means scoring completed but quality
was below the configured gate. Save a report as a CI artifact when you want
per-case reasons available for review.

See [troubleshooting](troubleshooting.md) for common failure patterns.
