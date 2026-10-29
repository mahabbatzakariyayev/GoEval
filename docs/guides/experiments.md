# Application experiments

An experiment runs your application on each saved input, then evaluates the
outputs it produced. This is useful when comparing application or retrieval
changes against the same reference data.

## Supply an application callback

The experiment package uses lower-level datasets and an application function:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/mahabbatzakariyayev/GoEval"
    "github.com/mahabbatzakariyayev/GoEval/experiment"
)

func main() {
    data := goeval.Cases{{Input: "Capital of France?", Expected: "Paris"}}.Samples()
    app := func(ctx context.Context, sample goeval.Sample) (experiment.Output, error) {
        if err := ctx.Err(); err != nil { return experiment.Output{}, err }
        // Replace this response with your application call.
        return experiment.Output{Response: "Paris"}, nil
    }
    result, err := experiment.Run(context.Background(), "baseline", data,
        app, []goeval.Metric{goeval.ExactMatch()}, goeval.RunConfig{MaxWorkers: 4})
    if err != nil { log.Fatal(err) }
    fmt.Println(result.Evaluation.Means["exact_match"])
}
```

The output is 1. The callback is a deterministic stand-in for your application;
replace it with the actual generation and retrieval call.

## Preserve retrieval evidence

Return RetrievedContexts and RetrievedContextIDs from the application when
your metrics need them. Reference fields remain attached to each input row.

The callback must honor its context and support concurrent calls. Input data
and nested containers remain read-only while the experiment runs.

## Handle partial results

Failed application rows are not evaluated with old responses. The result retains
ApplicationErrors, successfully updated rows, and evaluation results for
successful application outputs.

Application and metric errors are returned with their causes. Evaluation row
indices refer to the original dataset, even when failed application rows were
excluded. Inspect the partial result when it is nonnil.

## Configure execution

RunConfig controls workers, per-operation timeouts, and metric scoring retry
settings. Application callbacks run once per row; metric retry settings do not
retry the application callback.

FailFast stops after failures. Otherwise, the experiment continues where
possible and returns aggregated execution errors. Experiment persistence and
remote storage are not part of this helper.
