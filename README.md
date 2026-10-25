# GoEval

[![Go](https://github.com/mahabbatzakariyayev/GoEval/actions/workflows/ci.yml/badge.svg)](https://github.com/mahabbatzakariyayev/GoEval/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/mahabbatzakariyayev/GoEval.svg)](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval)
[![Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

GoEval is a native Go library for evaluating AI answers, retrieved context,
summaries, and agent tools. Use it in applications and Go tests with deterministic
scorers, model-based judges, custom criteria, and pass/fail gates.

- One import for common evaluation workflows.
- Go 1.23 or newer; standard library only.
- Offline text scoring and explicitly configured model providers.
- Bounded concurrency, cancellation, retries, typed errors, and partial results.
- JSON/JSONL datasets, saved reports, and Go test assertions.

**Status: pre-release.** The implemented workflows are tested; public APIs may
change before v1.0. See [coverage and limitations](docs/coverage.md) for the
supported feature set.

## Install

```sh
go get github.com/mahabbatzakariyayev/GoEval
```

The import path is case-sensitive. The Go package name is `goeval`.

## Quick start

This complete program needs no model, API key, or external service:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/mahabbatzakariyayev/GoEval"
)

func main() {
    evaluator, err := goeval.New(
        goeval.WithMetrics(goeval.ExactMatch(), goeval.NormalizedMatch()),
    )
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

Output:

```text
Exact match: 0.5
Normalized match: 1
All checks passed: false
```

Exact matching includes whitespace; normalized matching trims surrounding
whitespace. The run completes successfully, but the second exact-match gate
fails. `goeval.New()` with no options selects exact matching with threshold 1.

Run the checked-in version with `go run ./examples/quickstart`.

## Model-based evaluation

Configure your chat model explicitly. The provider supports compatible hosted
and local HTTP endpoints. This program checks whether an answer is supported by
retrieved facts and addresses the question:

```go
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	goeval "github.com/mahabbatzakariyayev/GoEval"
)

func main() {
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		log.Fatal("set OPENAI_MODEL and OPENAI_API_KEY (or OPENAI_BASE_URL for a local server)")
	}
	client, err := goeval.NewOpenAI(goeval.OpenAIConfig{
		APIKey: os.Getenv("OPENAI_API_KEY"), BaseURL: os.Getenv("OPENAI_BASE_URL"), Model: model,
	})
	if err != nil {
		log.Fatal(err)
	}
	evaluator, err := goeval.New(
		goeval.WithMetrics(goeval.Faithfulness(client, goeval.Threshold(0.8)), goeval.AnswerRelevance(client)),
		goeval.WithConcurrency(4),
		goeval.WithTimeout(2*time.Minute),
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := evaluator.Evaluate(ctx, []goeval.Case{{
		Input: "What is the capital of France?", Output: "Paris is the capital of France.",
		Context: []string{"Paris is the capital and largest city of France."},
	}})
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		log.Fatal(err)
	}
}
```

Set `OPENAI_MODEL` to your model name and `OPENAI_API_KEY` for an authenticated
endpoint. Set `OPENAI_BASE_URL` when using a compatible endpoint. Then run:

```sh
go run ./examples/rag
```

Model-based evaluation sends case data to your chosen provider. Metrics may make
multiple requests. Custom providers implement `goeval.LLM`; embedding metrics
accept `goeval.Embedder`. No vendor SDK is required.

## Built-in metrics

| Metric | Required case fields | What it measures |
| --- | --- | --- |
| `ExactMatch`, `NormalizedMatch`, `Contains`, `StringSimilarity` | Output, Expected | Deterministic text comparison |
| `SemanticSimilarity` | Output, Expected | Embedding cosine similarity |
| `Faithfulness` | Input, Output, Context | Support for answer claims in retrieved facts |
| `AnswerRelevance` | Input, Output | Relevance of answer statements |
| `ContextPrecision`, `ContextRecall` | Input, Output, Expected, Context | Ranked retrieval quality and reference coverage |
| `ContextRelevance` | Input, Output, Context | Relevance of retrieved statements |
| `Hallucination` | Input, Output, Sources | Agreement with authoritative evidence |
| `Bias`, `Toxicity` | Input, Output | Freedom from biased or toxic opinions |
| `Summarization` | Input, Output | Summary alignment and coverage; Input is the source text |
| `ToolCorrectness` | Input, Tools, ExpectedTools | Deterministic tool-name matching |
| `ArgumentCorrectness` | Input, Tools | Appropriateness of tool arguments |
| `TaskCompletion` | Input, Output; optional Tools | Completion of the requested task |
| `PromptAlignment` | Input, Output | Adherence to supplied instructions |
| `Judge` | Input, Output; optional Expected | Your own evaluation criteria |

Root built-ins use higher-is-better scores. Model judges default to threshold
0.5; exact and normalized matching require 1. Cosine similarity can be negative.
Use `Threshold`, `Strict`, and `WithoutReason` to configure model judges.

For example, use this metric with `WithMetrics`:

```go
goeval.Judge(client, "helpfulness",
    "The answer directly addresses the request with actionable details.",
    goeval.Threshold(0.8),
)
```

Add `AgainstExpected()` to include the reference answer in custom criteria.
Custom judges generate reusable evaluation steps. `LogProbs()` opts into
probability weighting when the model supports it.

## Use with go test

Save this as `evaluation_test.go`:

```go
package evaluation_test

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
    evaluator.Assert(t, context.Background(), goeval.Case{
        Input: "Capital of France?", Output: "Paris", Expected: "Paris",
    })
}
```

Run `go test ./...`. Tests that use model judges need your provider configuration;
the repository's own tests use deterministic responses and local HTTP servers.

## Cases and saved reports

A case uses these JSON fields:

```json
{
  "input": "What is the capital of France?",
  "output": "Paris",
  "expected": "Paris",
  "context": ["Paris is the capital of France."],
  "metadata": {"case_id": "capital-france"}
}
```

Load arrays with `goeval.ReadCases(reader)` or one case per line with
`goeval.ReadCasesJSONL(reader)`. Save `goeval.Cases` with `WriteJSON` or
`WriteJSONL`. Optional `sources` holds authoritative evidence for hallucination
checks; `tools` and `expected_tools` hold agent calls.

Nil Context or Sources means evidence was not supplied; an empty nonnil slice
means it was supplied but empty. Nil tool lists mean no calls. JSON readers
distinguish missing/null scalar fields from explicitly supplied empty strings.
Custom scorers can read case metadata from `Sample.Metadata`.

Reports expose `Rows`, `Means`, `Passed()`, and `Err()`. Save a report using
`json.Marshal(report)` and restore it with `json.Unmarshal(data, &report)` into a
`goeval.Report`. Requested metric names and failure messages survive this round
trip, so pass/fail decisions remain available. Original Go error types do not
survive JSON.

## Errors and execution

- `Evaluator.Evaluate` returns execution failures and retains partial results.
- A failed quality gate is a valid score. Inspect `report.Passed()`.
- `Evaluator.Check` evaluates one case and returns a `*goeval.ThresholdError`
  when a gate fails.
- `errors.Is` and `errors.As` work on live evaluation and provider errors.
- Score-only custom metrics need `goeval.WithThreshold` to participate in gates.

Validation runs before scoring. Incomplete or unrecognized model judgments
produce errors rather than inflated scores. Means include successful numeric
scores only.

The evaluator defaults to 16 concurrent cases, one attempt, and the caller's
deadline. Use `WithConcurrency`, `WithTimeout`, or `WithRunConfig` to override
them. Timeouts apply per scoring attempt; use a context deadline for a whole
run. Providers and custom metrics must honor contexts and support concurrent
calls. Keep case data and nested configuration read-only while a run is active.

## Advanced packages

| Package | Purpose |
| --- | --- |
| `goeval` | Cases, built-ins, evaluator, reports, provider interfaces, assertions |
| `scorers` | BLEU, ROUGE, chrF, retrieval, factual, rubric, SQL, and agent scorers |
| `judge` | Configurable multi-stage judges, custom rubrics, decision graphs, direct measurements |
| `providers/openai` | Compatible chat, structured JSON, token probabilities, embeddings |
| `cache` | Memory LRU and atomic disk caches for model responses and embeddings |
| `prompt` | Typed prompts, examples, serialization, and rendering |
| `experiment` | Run an application on saved inputs and evaluate its outputs |
| `testset` | Experimental graph-based evaluation data generation |

Advanced metrics implement `goeval.Metric` and can share an evaluator with
root built-ins. The lower-level `goeval.Dataset` API supports JSON, JSONL, and
CSV with `user_input`, `response`, `reference`, and retrieval fields. Read
[usage and behavior](docs/usage.md) for custom metrics, conversion, caching,
retry settings, and lower-level error handling.

An optional CLI evaluates lower-level datasets:

```sh
go run ./cmd/goeval evaluate -input examples/data/evaluation.jsonl -metrics exact_match
go run ./cmd/goeval metrics
```

## Development

```sh
make check
make lint
make example
```

Checks include formatting, vet, race tests, and builds. CI also exercises the
minimum Go version, Linux/macOS/Windows, and a separate consumer module.
See [CONTRIBUTING.md](CONTRIBUTING.md), [release instructions](docs/releasing.md),
and [the changelog](CHANGELOG.md).

## License

[Apache License 2.0](LICENSE). Applicable copyright and adaptation notices are
preserved in [NOTICE](NOTICE).
