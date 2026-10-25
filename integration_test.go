package goeval_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/cache"
	"github.com/mahabbatzakariyayev/GoEval/providers/openai"
)

func TestModelEvaluationThroughHTTPAndCache(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer fixture-key" {
			http.Error(w, "invalid provider request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/embeddings" {
			var input struct {
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
				return
			}
			data := make([]map[string]any, len(input.Input))
			for i, text := range input.Input {
				vector := []float64{1, 0}
				if text == "Sydney" {
					vector = []float64{0, 1}
				}
				// Return out of order to exercise the provider's index handling.
				data[len(data)-1-i] = map[string]any{"index": i, "embedding": vector}
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
				t.Error(err)
			}
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var input struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Schema struct {
					Schema struct {
						Properties map[string]json.RawMessage `json:"properties"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			return
		}
		if input.Model != "fixture-model" || len(input.Messages) == 0 || input.Messages[0].Content == "" {
			http.Error(w, "model or prompt missing", http.StatusBadRequest)
			return
		}
		properties := input.ResponseFormat.Schema.Schema.Properties
		text := input.Messages[0].Content
		answer := map[string]any{}
		switch {
		case properties["truths"] != nil:
			answer["truths"] = []string{"Paris is the capital of France."}
		case properties["claims"] != nil:
			claim := "Paris is the capital of France."
			if strings.Contains(text, "Sydney") {
				claim = "Sydney is the capital of France."
			}
			answer["claims"] = []string{claim}
		case properties["verdicts"] != nil:
			verdict := "yes"
			if strings.Contains(text, "Sydney") {
				verdict = "no"
			}
			answer["verdicts"] = []map[string]string{{"verdict": verdict, "reason": "Compared the claim with the retrieved facts."}}
		case properties["steps"] != nil:
			answer["steps"] = []string{"Compare the answer with the reference."}
		case properties["score"] != nil:
			score := 9
			if strings.Contains(text, "Sydney") {
				score = 0
			}
			answer["score"], answer["reason"] = score, "Compared the answer with the reference."
		case properties["reason"] != nil:
			answer["reason"] = "The score reflects the supported claims."
		default:
			http.Error(w, "unexpected judgment schema", http.StatusBadRequest)
			return
		}
		content, err := json.Marshal(answer)
		if err != nil {
			t.Error(err)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"content": string(content)}, "finish_reason": "stop",
		}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := goeval.NewOpenAI(goeval.OpenAIConfig{
		APIKey: "fixture-key", BaseURL: server.URL + "/v1", Model: "fixture-model",
		EmbeddingModel: "fixture-embeddings", StructuredOutput: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := cache.NewMemory(64)
	if err != nil {
		t.Fatal(err)
	}
	model := cache.LLM(client, store, "fixture-model")
	embedder := cache.Embeddings(client, store, "fixture-embeddings")
	evaluator, err := goeval.New(goeval.WithMetrics(
		goeval.Faithfulness(model), goeval.NormalizedMatch(), goeval.SemanticSimilarity(embedder),
		goeval.Judge(model, "correctness", "Compare factual accuracy.", goeval.AgainstExpected()),
	), goeval.WithConcurrency(2), goeval.WithTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	cases, err := goeval.ReadCasesJSONL(strings.NewReader(
		`{"input":"Capital of France?","output":"Paris","expected":"Paris","context":["Paris is the capital of France."]}` + "\n" +
			`{"input":"Capital of France?","output":"Sydney","expected":"Paris","context":["Paris is the capital of France."]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		before := requests.Load()
		report, err := evaluator.Evaluate(context.Background(), cases)
		if err != nil {
			t.Fatal(err)
		}
		if report.Passed() || report.Means["faithfulness"] != .5 || report.Means["semantic_similarity"] != .5 || report.Means["normalized_match"] != .5 {
			t.Fatal("unexpected evaluation", report.Means)
		}
		if attempt == 1 && requests.Load() != before {
			t.Fatal("repeated evaluation did not use cache")
		}
		data, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		var saved goeval.Report
		if err := json.Unmarshal(data, &saved); err != nil || saved.Passed() {
			t.Fatal("saved report lost failed gates", err)
		}
	}
	if requests.Load() == 0 {
		t.Fatal("provider was never called")
	}
	if report, err := evaluator.Check(context.Background(), cases[0]); err != nil || !report.Passed() {
		t.Fatal(report, err)
	}
}

func TestHTTPFailureRemainsInspectableThroughEvaluator(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "fixture-key: too many requests", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client, err := goeval.NewOpenAI(goeval.OpenAIConfig{APIKey: "fixture-key", BaseURL: server.URL, Model: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := goeval.New(goeval.WithMetrics(goeval.Faithfulness(client)), goeval.WithRunConfig(goeval.RunConfig{MaxRetries: 2}))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Evaluate(context.Background(), []goeval.Case{{Input: "q", Output: "a", Context: []string{"source"}}})
	var httpErr *openai.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusTooManyRequests || requests.Load() != 2 || r == nil || r.Passed() {
		t.Fatal("HTTP error or retry count lost", err, requests.Load())
	}
	if strings.Contains(err.Error(), "fixture-key") {
		t.Fatal("API key leaked into error")
	}
}
