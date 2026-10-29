# Datasets and reports

Keep evaluation cases as versioned data. JSONL makes it easy to add examples,
review changes, and run the same cases against new application versions.

## Create a JSONL dataset

Save these objects as `cases.jsonl`, one per line:

```json
{"name":"France","input":"Capital of France?","output":"Paris","expected":"Paris"}
{"name":"Italy","input":"Capital of Italy?","output":" Rome ","expected":"Rome"}
```

Use `context` for retrieved passages, `sources` for authoritative material,
`tools` and `expected_tools` for agent invocations, and `metadata` for
application identifiers. See [detailed field behavior](../usage.md#case-fields).

## Evaluate saved cases

The following program loads a JSONL file and prints a report:

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/mahabbatzakariyayev/GoEval"
)

func main() {
	input := flag.String("input", "examples/data/cases.jsonl", "JSONL cases to evaluate")
	flag.Parse()
	file, err := os.Open(*input)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	cases, err := goeval.ReadCasesJSONL(file)
	if err != nil {
		log.Fatal(err)
	}
	evaluator, err := goeval.New(goeval.WithMetrics(goeval.ExactMatch(), goeval.NormalizedMatch()))
	if err != nil {
		log.Fatal(err)
	}
	report, runErr := evaluator.Evaluate(context.Background(), cases)
	if report != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			log.Fatal(err)
		}
	}
	if runErr != nil {
		log.Fatal(runErr)
	}
}
```

From the repository root:

```sh
go run ./examples/datasets -input examples/data/cases.jsonl > evaluation.report.json
```

In a separate application, pass the path to your own `cases.jsonl` file.
The expected means for this dataset are exact_match 0.5 and normalized_match 1.

## Load JSON arrays or write cases

`goeval.ReadCases(reader)` reads a JSON array. `ReadCasesJSONL(reader)` reads
one object per nonblank line. Both return `goeval.Cases`, which can be passed
directly to `Evaluator.Evaluate`.

Call `cases.WriteJSON(writer)` or `cases.WriteJSONL(writer)` to save them.
Readers preserve large numeric metadata and tool arguments as `json.Number`.

## Save and restore a report

Use `json.Marshal(report)` to serialize a report. Read the saved bytes with
`json.Unmarshal(data, &saved)`, where saved is a `goeval.Report`.
The restored report retains rows, means, metric names, failure messages, and
the information needed by `saved.Passed()`.

Go error types and causes do not survive JSON. `errors.Is` and `errors.As`
apply to live errors; restored reports preserve their messages.

## Keep failure and quality separate

A failed gate is a valid score, so Evaluate can return nil error while Passed
returns false. Execution failures return an error and may include a partial
report. Save that report when it is nonnil to retain successful scores and
failure details.

Means include successful numeric scores only. Inspect Rows to find omissions
and errors before interpreting averages.

## Use lower-level datasets

`Case.Sample()` and `Cases.Samples()` convert friendly cases into
`goeval.Sample` and `goeval.Dataset`. Their readers support JSON, JSONL,
and CSV using user_input, response, reference, and retrieval field names.

Pass these datasets to `Evaluator.EvaluateDataset` for the same friendly error
handling. The optional CLI uses this lower-level dataset representation.
