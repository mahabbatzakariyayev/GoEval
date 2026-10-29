# Agent tool evaluation

Evaluate tool use separately from answer quality. A correct tool name does not
prove that its arguments were correct or that the task succeeded.

## Start with tool-name matching

Use `goeval.ToolCorrectness()` with Tools and ExpectedTools. This root metric
is deterministic and compares tool names; it does not verify arguments or
outputs. Nil tool lists mean no calls.

Use `ArgumentCorrectness(model)` to judge actual arguments against Input, or
`TaskCompletion(model)` to judge the outcome from Input, Output, and called tools.

## Check arguments deterministically

Use the advanced tool metric to compare calls against expected arguments:

```go
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
```

Run:

```sh
go run ./examples/agents
```

Expected output:

```text
Tool arguments match: true
```

This metric implements the shared GoEval interface, so you can use it with
friendly cases. Its report name is `judge.tool_correctness`.

## Choose ordering and comparison fields

ToolCorrectnessOptions lets you select InputParameters and Output comparison.
ShouldExactMatch requires an exact call sequence. ShouldConsiderOrdering enables
ordered matching for the non-exact mode.

AvailableTools enables model-based tool-selection scoring. Provide a model in
that case; deterministic reference comparison alone needs no model.

## Describe actual calls

Each root Tool contains Name, Arguments, optional Output, Description, and Type.
Use expected calls that represent the behavior your application should perform.
Keep identifiers and arguments consistent across saved datasets.

When evaluating a lower-level sample, explicitly supplied Messages and
ReferenceToolCalls override tool data preserved in saved case metadata. Empty
updates also replace old calls.

## Check the whole application outcome

A tool call can match its reference while the final answer is still wrong.
Combine tool correctness with answer relevance, faithfulness when Context is
available, or task completion.

These are text and tool-call workflows. Full conversation simulation and
multimodal agent evaluation are outside the current feature set. See
[coverage](../coverage.md).
