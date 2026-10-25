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
