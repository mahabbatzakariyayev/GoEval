package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestClientCopiesConfigurationAndRawPromptOverridesInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Temperature float64 `json:"temperature"`
			Messages    []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Temperature != .2 || len(body.Messages) != 1 || body.Messages[0].Content != "raw prompt" {
			t.Error(body)
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	temperature := .2
	c, err := New(Config{BaseURL: srv.URL, Model: "judge", Temperature: &temperature})
	if err != nil {
		t.Fatal(err)
	}
	temperature = .9
	if _, err := c.Generate(context.Background(), goeval.Prompt{Text: "raw prompt", Input: make(chan int)}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Embed(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	//lint:ignore SA1012 Exercise nil-context validation.
	if _, err := c.Generate(nil, goeval.Prompt{}); err == nil {
		t.Fatal("nil context accepted")
	}
}
