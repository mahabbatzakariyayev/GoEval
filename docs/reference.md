# API reference

The root package covers common workflows with one import. Advanced packages
expose algorithm-specific configuration and integration helpers.

## Root types

| Type | Purpose |
| --- | --- |
| `Case`, `Cases` | Friendly input/output/expected cases, retrieval evidence, tools, metadata |
| `Tool` | An actual or expected tool invocation |
| `Evaluator` | A reusable, configured evaluation runner |
| `Report` | Rows, Means, Passed, Err, JSON report persistence |
| `Metric`, `MetricFunc` | Custom scorer interface and function adapter |
| `Score` | Numeric value, reason, optional gate, judgment details |
| `Sample`, `Dataset` | Lower-level single-turn and conversation data |
| `RunConfig` | Concurrency, timeout, attempts, retry delay, fail-fast settings |
| `LLM`, `Embedder`, `LogProbLLM` | Provider capabilities |
| `Prompt` | Raw text or instruction/input request with an optional result schema |

## Construct an evaluator

`New(options ...Option) (*Evaluator, error)` validates configuration.
Its default metric is exact matching with threshold 1.

| Option | Purpose |
| --- | --- |
| `WithMetrics(...Metric)` | Select one or more metrics; an explicit empty list is an error |
| `WithConcurrency(int)` | Limit concurrent cases; zero uses 16 |
| `WithTimeout(time.Duration)` | Per metric-attempt timeout; zero inherits the context |
| `WithRunConfig(RunConfig)` | Configure workers, attempts, delays, and fail-fast together |

## Evaluate and assert

| Method | Behavior |
| --- | --- |
| `Evaluator.Evaluate(ctx, []Case)` | Returns a report and execution failures |
| `Evaluator.EvaluateDataset(ctx, Dataset)` | Runs an existing lower-level dataset |
| `Evaluator.Check(ctx, Case)` | Returns execution failures or failed/missing gates |
| `Evaluator.Assert(t, ctx, Case)` | Reports gate and execution failures through Go testing |
| `Report.Passed()` | Requires a nonempty, complete evaluation with passing gates |
| `Report.Err()` | Returns live execution errors or recorded Check failures |

The lower-level package function Evaluate preserves per-row failures when
FailFast is false; inspect EvaluationResult.Err even when its returned error is
nil. Prefer the Evaluator methods for the common application workflow.

## Model judge options

Threshold sets a minimum in zero to one. Strict requires a perfect result and
enables strict scoring. WithoutReason skips optional reason generation.

AgainstExpected includes the reference in a custom Judge. LogProbs enables
optional token-probability weighting for custom Judge. These two options apply
to custom Judge rather than the other root model factories.

## Datasets and reports

ReadCases and ReadCasesJSONL load friendly cases. Cases.WriteJSON and WriteJSONL
save them. Case.Sample and Cases.Samples convert to lower-level data.

ReadJSON, ReadJSONL, and ReadCSV load lower-level datasets. Dataset.WriteJSON,
WriteJSONL, and WriteCSV save them. Report supports standard JSON marshaling and
unmarshaling.

See [datasets and reports](guides/datasets.md) and [detailed behavior](usage.md).

## Advanced packages

| Package | Guide | Go API |
| --- | --- | --- |
| goeval | [Getting started](guides/getting-started.md) | [Root package](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval) |
| scorers | [Choose metrics](guides/metrics.md) | [Scorers](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval/scorers) |
| judge | [Custom criteria](guides/custom-metrics.md) | [Judges](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval/judge) |
| providers/openai | [Models](guides/models.md) | [HTTP provider](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval/providers/openai) |
| cache | [Performance](guides/performance.md) | [Caching](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval/cache) |
| experiment | [Experiments](guides/experiments.md) | [Application runner](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval/experiment) |
| testset | [Coverage](coverage.md) | [Generation](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval/testset) |
| prompt | [Custom providers](guides/models.md#implement-a-provider) | [Prompts](https://pkg.go.dev/github.com/mahabbatzakariyayev/GoEval/prompt) |

Package-index documentation becomes available after the published module is
indexed. For a local checkout, use go doc.

## Read reference documentation locally

```sh
go doc .
go doc .Evaluator
go doc .Case
go doc ./judge
go doc ./scorers
```

Run these commands from the GoEval repository root.
