# Model-based evaluation

Pass an explicit model provider to the judge you want to run. GoEval supports a
compatible HTTP chat endpoint and lets you implement your own provider.

## Configure the environment

Choose the chat model available at your endpoint and set its credentials:

```sh
export OPENAI_MODEL='your-chat-model'
export OPENAI_API_KEY='your-api-key'
```

For a compatible endpoint, also set `OPENAI_BASE_URL` to its API root, such as
`http://localhost:8080/v1`. A local server may not require a key. The constructor
does not select a chat model automatically.

## Evaluate an answer against retrieved facts

This complete program uses faithfulness and answer relevance:

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

Save it as `main.go` in a consumer module and run `go run .`. From the GoEval
checkout, run `go run ./examples/rag`.

The JSON report includes per-row scores, metric means, configured gates, and
judgment evidence. The example prints the report; use `report.Passed()` or
`evaluator.Check` when you need to enforce gates.

## Understand Context and Sources

Use `Context` for passages retrieved by your application. Faithfulness compares
extracted output claims with facts extracted from those passages.

Use `Sources` for authoritative evidence passed to `Hallucination`. This is a
separate case field; setting Context does not automatically populate Sources.

A nil evidence slice means not supplied. An empty nonnil slice means supplied
but empty. Missing required fields fail preflight.

## Use embeddings

Set `EmbeddingModel` in `goeval.OpenAIConfig` and pass the resulting client to
`goeval.SemanticSimilarity(client)`. The client implements both the chat and
embedding interfaces; the two model names are independent.

Embedding responses must contain one finite, nonempty vector per input, in
input order. The HTTP adapter handles provider response indices.

## Request structured output

Set `StructuredOutput: true` only when your endpoint and selected model support
JSON-schema response mode. Otherwise the provider requests JSON-object mode.
Judges still decode and validate their required result fields.

Missing judgments, invalid verdicts, malformed JSON, refusals, and truncated
generations return errors. See [troubleshooting](troubleshooting.md) for diagnosis.

## Implement a provider

Implement `Generate(context.Context, goeval.Prompt) (string, error)`, or wrap a
function with `goeval.LLMFunc`. Send Prompt.Text when present; otherwise use
Instruction and Input. Respect Schema for structured results.

Providers must honor cancellation and permit concurrent calls. Implement
`goeval.Embedder` for embedding scorers and optionally `goeval.LogProbLLM`
for probability-weighted custom judgments.
