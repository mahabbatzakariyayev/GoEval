# Usage and behavior

Start with the root `goeval` package. Use `judge` and `scorers` when you need
algorithm-specific configuration or direct measurements.

## Case fields

| Field | Meaning |
| --- | --- |
| `Input` | The user's request; source text when judging a summary |
| `Output` | The generated answer or summary |
| `Expected` | A reference answer, when the selected metric needs it |
| `Context` | Retrieved passages used for grounding and retrieval metrics |
| `Sources` | Authoritative evidence used by hallucination checks |
| `Tools`, `ExpectedTools` | Actual and expected agent invocations |
| `Name`, `Metadata` | Optional identifiers and application metadata |

Nil Context or Sources means evidence was not supplied. An empty nonnil slice
means evidence was supplied but contains no passages. Nil tool slices mean no
tool calls. In Go literals, empty Input, Output, and Expected are supplied empty
strings. JSON readers distinguish absent/null scalar fields from supplied empty
strings. This lets preflight validation reject missing required fields.

```go
cases, err := goeval.ReadCasesJSONL(reader)
if err != nil {
    return err
}
report, err := evaluator.Evaluate(ctx, cases)
```

JSONL uses one object per line:

```json
{"input":"Capital of France?","output":"Paris","expected":"Paris"}
```

Use `cases.WriteJSON(writer)` or `WriteJSONL(writer)` to save cases. Metadata and
tool argument numbers are retained as `json.Number` when decoding untyped maps.
Custom scorers can read application metadata directly from `Sample.Metadata`.
The `goeval.judge` key is reserved for internal case conversion.

## Results and gates

`Evaluator.Evaluate` returns a report and any execution error. Reports retain
successful scores when other cases or metrics fail. `report.Err()` aggregates
execution failures and preserves causes for `errors.Is` and `errors.As`. Reports
from `Check` also retain threshold failures. Means include successful numeric
scores only.

Threshold failures are valid scores with `Success: false`, rather than execution
errors. `report.Passed()` requires a nonempty evaluation and a passing gate for
every selected metric and row. Use `evaluator.Check(ctx, case)` for one case when
threshold failure must return a `*goeval.ThresholdError`, or `evaluator.Assert`
inside Go tests.

Root built-ins include gates. Custom and advanced score-only metrics need
`goeval.WithThreshold(metric, minimum)` before `Passed` can succeed. Some advanced
scorers return negative similarity or unnormalized values: choose gates according
to that algorithm's documented scale. Scores expose optional reason and details.

Save reports with `json.Marshal(report)` and restore them into a `goeval.Report`
using `json.Unmarshal`. Reports retain rows, means, requested metric names, and
any run error so `Passed()` continues to work after loading. Error messages
survive JSON; original Go error types and causes do not. Older rows/means-only
exports can still be read, but cannot establish that every requested metric ran
and therefore do not pass quality gates.

Faithfulness, answer relevance, bias, and toxicity require a judgment for every
extracted claim, statement, or opinion. Retrieval precision, hallucination,
prompt alignment, tool arguments, and summary alignment similarly check judgment
counts against their inputs. Missing judgments and unrecognized verdicts return
execution errors instead of producing optimistic scores.

## Concurrency and retries

Evaluators preserve input order and default to 16 concurrent rows, one scoring
attempt, and the caller's context deadline. Set `WithConcurrency`, `WithTimeout`,
or `WithRunConfig` to override these defaults. A timeout applies per metric
attempt; set a context deadline for the entire evaluation. `RunConfig.MaxRetries`
is the total number of attempts, with zero using one attempt.

`DefaultRunConfig()` explicitly selects a different retry profile: ten attempts,
180 seconds per attempt, 16 workers, a 60-second retry delay cap, and seed 42.
Retries can repeat model requests and incur additional cost.

Providers and custom metrics must honor contexts and support concurrent calls.
Do not mutate cases, nested maps/slices, or provider configuration during a run.
The evaluator copies the selected metric slice; it does not deep-copy your data
or custom metric objects. Reusable custom criteria cache their generated steps.

## Advanced datasets and metrics

`Case.Sample()` and `Cases.Samples()` convert friendly fields into the lower-level
`goeval.Sample`/`Dataset` representation. That representation uses UserInput,
Response, Reference, RetrievedContexts, ReferenceContexts, and message/tool fields.
Its JSON/JSONL/CSV readers are `ReadJSON`, `ReadJSONL`, and `ReadCSV`.

`Evaluator.EvaluateDataset` retains the friendly evaluator's execution error
handling for existing datasets. The lower-level package function
`goeval.Evaluate(ctx, dataset, metrics, config)` preserves per-row failures when
FailFast is false; inspect `result.Err()` even if its returned error is nil.

Import advanced scorers from `github.com/mahabbatzakariyayev/GoEval/scorers` and
judges from `github.com/mahabbatzakariyayev/GoEval/judge`. Both implement
`goeval.Metric` and can share a single evaluation. Advanced judge result names
start with `judge.`; root built-ins use concise names such as `faithfulness`.

`judge.Case` supports actual_output, expected_output, retrieval_context, and
camelCase aliases in saved datasets. Its `ToSample` conversion preserves
additional fields in namespaced metadata. Avoid altering that metadata by hand.
When a sample also supplies Messages or ReferenceToolCalls, those explicit tool
lists take precedence over the saved case, including explicit empty updates.

## Custom providers and metrics

Implement `goeval.LLM.Generate(context.Context, goeval.Prompt)` to connect a model.
Use `Prompt.Text` when present; otherwise send `Instruction` and `Input`. Respect
`Schema` for structured judgments. `goeval.LLMFunc` adapts a function to this
interface. Implement `Embedder` for embedding scorers, and optionally `LogProbLLM`
for probability-weighted custom judgments. Root `Judge` enables that optional
mode only when `goeval.LogProbs()` is requested.

A custom metric implements `Name`, `RequiredFields`, and `Score`. The runner
also recognizes optional `Validate() error` and `ValidateSample(Sample) error`
methods during preflight. Validation must not make model calls. Metric names
must be nonempty and unique within an evaluation.

Use `cache` to wrap models or embedders when repeated evaluations should reuse
responses. The disk cache writes atomically; its format is specific to GoEval.
