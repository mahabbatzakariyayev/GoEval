# Evaluate AI applications in Go

GoEval helps you check answers, retrieved context, summaries, and agent tools
from your application or Go tests. Start with an offline comparison, then add
model-based judges where your application needs them.

## Start with a working evaluation

Install the package, create a case, choose a metric, and inspect the result.
The [getting started guide](guides/getting-started.md) takes you through the
complete program without an API key.

```sh
go get github.com/mahabbatzakariyayev/GoEval
```

Go 1.23 or newer is required. The library and documentation builder use the Go
standard library. Model providers are configured explicitly.

## Follow a learning path

- [Getting started](guides/getting-started.md): run an evaluation and understand a failed gate.
- [Choose your metrics](guides/metrics.md): decide what to measure and which data you need.
- [Model-based evaluation](guides/models.md): connect a judge to a compatible HTTP endpoint.
- [Datasets and reports](guides/datasets.md): evaluate saved examples and preserve results.
- [Testing and CI](guides/testing.md): turn evaluation gates into Go test failures.

## Build your own workflow

- [Custom metrics and judges](guides/custom-metrics.md): add a deterministic rule or your own criteria.
- [Agent tool evaluation](guides/agents.md): check tool names, arguments, and task outcomes.
- [Application experiments](guides/experiments.md): run your application before scoring.
- [Caching and performance](guides/performance.md): control parallel work, deadlines, and retries.

## Know what a result means

An evaluation can finish without execution errors and still fail its quality
gates. Inspect `report.Passed()` for quality and the returned error for execution
failures. Successful numeric scores contribute to means; failed metric
executions retain errors instead of silently becoming zero.

GoEval is a pre-release library. See [feature coverage](coverage.md) for supported
capabilities and remaining limits, and [API reference](reference.md) for the
public interfaces.
