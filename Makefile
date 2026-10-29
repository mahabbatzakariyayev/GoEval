.PHONY: test vet fmt check build example lint fuzz bench docs docs-check docs-serve

test:
	go test -race -cover ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

check:
	test -z "$$(gofmt -l .)"
	go vet ./...
	go test -race ./...
	go build ./...
	go run ./internal/cmd/check-docs
	go run ./internal/cmd/docs

build:
	go build -o bin/goeval ./cmd/goeval

example:
	go run ./examples/quickstart

docs:
	go run ./internal/cmd/docs

docs-check:
	go run ./internal/cmd/check-docs
	go test ./internal/docsite
	go run ./internal/cmd/docs

docs-serve:
	go run ./internal/cmd/docs -serve 127.0.0.1:4173

lint:
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...

fuzz:
	go test -run='^$$' -fuzz=FuzzSampleJSONRoundTrip -fuzztime=15s -parallel=4 .

bench:
	go test -run='^$$' -bench=BenchmarkEvaluateOffline -benchmem .
