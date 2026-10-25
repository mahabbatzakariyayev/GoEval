# Contributing to GoEval

Use Go 1.23 or newer. The library and prompt maintenance tools use the standard
library. Read [supported features](docs/coverage.md) before adding metrics.

```sh
gofmt -w .
go vet ./...
go test -race -cover ./...
go build ./...
go run ./examples/quickstart
go run ./internal/cmd/check-docs
```

`make lint` runs the pinned Staticcheck release with a modern Go toolchain
without adding runtime dependencies. `make fuzz` exercises dataset round trips;
`make bench` measures the offline runner. CI tests Go 1.23 and stable, with
stable runs on Linux, macOS, and Windows.

Keep metric validation free of model calls. Providers and metrics must honor
contexts and support concurrent calls. Input data and nested configuration
containers remain read-only during evaluation. Return errors with their causes
so callers can use `errors.Is` and `errors.As`.

Test scoring stages and reducers with deterministic responses and recorded
reference scores. Cover absent fields, empty inputs, malformed judgments,
thresholds, and cancellation when relevant. Do not replace a documented
multi-stage algorithm with a generic judge under the same name.

The committed prompt snapshots record source revisions and original text.
After editing an original judge template, recompile its Go representation:

```sh
go run ./internal/cmd/compile-prompts
go test ./internal/judgetemplates ./judge ./scorers
```

The compiler rejects unsupported expressions. Template tests compare every
compiled entry with its original conversion. Preserve copyright notices and
record deliberate changes to algorithms, prompts, and reference fixtures.

Examples and tests must not make billable model requests automatically. Never
commit API keys or private production evaluation data. Report bugs and propose
changes through this repository's issues and pull requests. Contributions are
licensed under Apache-2.0; retain LICENSE and applicable notices in NOTICE.
