package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestCompatibleRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "application/json" || r.Method != "POST" {
			t.Error("incorrect request headers/method")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/v1/chat/completions":
			if body["model"] != "chat-model" {
				t.Error(body)
			}
			format := body["response_format"].(map[string]any)
			if format["type"] != "json_schema" {
				t.Error(format)
			}
			if body["temperature"] != 0.1 {
				t.Error("prompt temperature not honored")
			}
			io.WriteString(w, `{"choices":[{"message":{"content":"{\"value\":1}"},"finish_reason":"stop"}]}`)
		case "/v1/embeddings":
			if body["model"] != "embedding-model" {
				t.Error(body)
			}
			io.WriteString(w, `{"data":[{"index":1,"embedding":[0,1]},{"index":0,"embedding":[1,0]}]}`)
		default:
			t.Errorf("unexpected URL %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	client, err := New(Config{APIKey: "secret", BaseURL: srv.URL + "/v1/", Model: "chat-model", EmbeddingModel: "embedding-model", StructuredOutput: true})
	if err != nil {
		t.Fatal(err)
	}
	temp := 0.1
	out, err := client.Generate(context.Background(), goeval.Prompt{Instruction: "judge", Input: map[string]string{"response": "answer"}, Schema: map[string]any{"type": "object"}, Temperature: &temp})
	if err != nil || out != `{"value":1}` {
		t.Fatalf("%q %v", out, err)
	}
	vectors, err := client.Embed(context.Background(), []string{"a", "b"})
	if err != nil || len(vectors) != 2 || vectors[0][0] != 1 || vectors[1][1] != 1 {
		t.Fatalf("%v %v", vectors, err)
	}
}

func TestLogProbabilityCapability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["logprobs"] != true || body["top_logprobs"] != float64(3) {
			t.Errorf("wrong logprob request: %v", body)
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"score\":8,\"reason\":\"Accurate\"}"},"logprobs":{"content":[{"token":"8","logprob":-0.5,"top_logprobs":[{"token":"8","logprob":-0.5},{"token":"7","logprob":-1.5}]}]},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	c, err := New(Config{BaseURL: srv.URL, Model: "judge"})
	if err != nil {
		t.Fatal(err)
	}
	var model goeval.LogProbLLM = c
	out, err := model.GenerateWithLogProbs(context.Background(), goeval.Prompt{Text: "Judge accuracy.", TopLogProbs: 3})
	if err != nil || len(out.LogProbs) != 1 || len(out.LogProbs[0].TopLogProbs) != 2 || out.LogProbs[0].Token != "8" {
		t.Fatal(out, err)
	}
	if _, err := model.GenerateWithLogProbs(context.Background(), goeval.Prompt{TopLogProbs: 21}); err == nil {
		t.Fatal("invalid top logprobs accepted")
	}
}
func TestResponseFailures(t *testing.T) {
	for _, body := range []string{`{"choices":[]}`, `{"choices":[{"message":{"content":null}}]}`, `{"choices":[{"message":{"content":"x","refusal":"blocked"}}]}`, `{"choices":[{"message":{"content":"x"},"finish_reason":"length"}]}`, `not json`} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
			defer srv.Close()
			c, _ := New(Config{BaseURL: srv.URL, Model: "model"})
			if _, err := c.Generate(context.Background(), goeval.Prompt{}); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
	for _, body := range []string{`{"data":[]}`, `{"data":[{"index":0,"embedding":[]}]}`, `{"data":[{"index":2,"embedding":[1]}]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
		c, _ := New(Config{BaseURL: srv.URL, EmbeddingModel: "embed"})
		_, err := c.Embed(context.Background(), []string{"a"})
		srv.Close()
		if err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
func TestHTTPErrorAndCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		io.WriteString(w, `secret rate limited`)
	}))
	defer srv.Close()
	c, _ := New(Config{BaseURL: srv.URL, Model: "model", APIKey: "secret"})
	_, err := c.Generate(context.Background(), goeval.Prompt{})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || !httpErr.Temporary() || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Generate(ctx, goeval.Prompt{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer slow.Close()
	c, _ = New(Config{BaseURL: slow.URL, Model: "model"})
	ctx, done := context.WithTimeout(context.Background(), time.Millisecond)
	defer done()
	_, err = c.Generate(ctx, goeval.Prompt{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestConfigurationValidation(t *testing.T) {
	for _, base := range []string{"ftp://localhost", "https://example.com/v1?api-key=x", "/relative", "https://user:pass@example.com"} {
		if _, err := New(Config{BaseURL: base}); err == nil {
			t.Fatalf("accepted %s", base)
		}
	}
	c, _ := New(Config{})
	if _, err := c.Generate(context.Background(), goeval.Prompt{}); err == nil {
		t.Fatal("missing chat model accepted")
	}
	if _, err := c.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatal("missing embedding model accepted")
	}
}
