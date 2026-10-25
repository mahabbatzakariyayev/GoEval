.PHONY: test vet fmt check build example lint fuzz bench docs

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

build:
	go build -o bin/goeval ./cmd/goeval

example:
	go run ./examples/quickstart

docs:
	go run ./internal/cmd/check-docs

lint:
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...

fuzz:
	go test -run='^$$' -fuzz=FuzzSampleJSONRoundTrip -fuzztime=15s -parallel=4 .

bench:
	go test -run='^$$' -bench=BenchmarkEvaluateOffline -benchmem .
