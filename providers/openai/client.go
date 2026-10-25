// Package openai implements OpenAI-compatible chat completions and embeddings.
// BaseURL can target a compatible hosted or local inference server.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

type Config struct {
	APIKey           string
	BaseURL          string // Defaults to https://api.openai.com/v1.
	Model            string // Explicitly selected chat model; no implicit billable default.
	EmbeddingModel   string
	HTTPClient       *http.Client
	Temperature      *float64
	MaxTokens        int
	StructuredOutput bool // Use strict JSON schema instead of JSON object mode.
}
type Client struct {
	cfg  Config
	http *http.Client
	base string
}

var _ goeval.LLM = (*Client)(nil)
var _ goeval.LogProbLLM = (*Client)(nil)
var _ goeval.Embedder = (*Client)(nil)

func New(cfg Config) (*Client, error) {
	cfg.Temperature = check.CopyPtr(cfg.Temperature)
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, fmt.Errorf("openai: invalid base URL")
	}
	if cfg.MaxTokens < 0 {
		return nil, fmt.Errorf("openai: negative token limit")
	}
	if cfg.Temperature != nil && (math.IsNaN(*cfg.Temperature) || math.IsInf(*cfg.Temperature, 0)) {
		return nil, fmt.Errorf("openai: invalid temperature")
	}
	c := cfg.HTTPClient
	if c == nil {
		c = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Client{cfg: cfg, http: c, base: strings.TrimRight(cfg.BaseURL, "/")}, nil
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string   { return fmt.Sprintf("openai: HTTP %d: %s", e.StatusCode, e.Body) }
func (e *HTTPError) Temporary() bool { return e.StatusCode == 429 || e.StatusCode >= 500 }

func (c *Client) request(ctx context.Context, path string, payload, out any) error {
	if err := check.Context(ctx); err != nil {
		return err
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("openai: request: %w", err)
	}
	defer resp.Body.Close()
	const maxBody = 16 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxBody {
		return fmt.Errorf("openai: response exceeds 16 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(body)
		if c.cfg.APIKey != "" {
			msg = strings.ReplaceAll(msg, c.cfg.APIKey, "[redacted]")
		}
		if len(msg) > 2048 {
			msg = msg[:2048]
		}
		return &HTTPError{resp.StatusCode, msg}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("openai: decode response: %w", err)
	}
	return nil
}

func (c *Client) Generate(ctx context.Context, p goeval.Prompt) (string, error) {
	out, err := c.complete(ctx, p)
	return out.Text, err
}
func (c *Client) GenerateWithLogProbs(ctx context.Context, p goeval.Prompt) (goeval.Completion, error) {
	p.LogProbs = true
	return c.complete(ctx, p)
}
func (c *Client) complete(ctx context.Context, p goeval.Prompt) (goeval.Completion, error) {
	if err := check.Context(ctx); err != nil {
		return goeval.Completion{}, err
	}
	if c.cfg.Model == "" {
		return goeval.Completion{}, fmt.Errorf("openai: chat model is required")
	}
	var input []byte
	if p.Text == "" {
		var err error
		input, err = json.Marshal(p.Input)
		if err != nil {
			return goeval.Completion{}, err
		}
	}
	instruction := p.Instruction
	if p.Schema != nil {
		schema, err := json.Marshal(p.Schema)
		if err != nil {
			return goeval.Completion{}, err
		}
		instruction += "\nReturn only a JSON object matching this schema: " + string(schema)
	}
	payload := map[string]any{"model": c.cfg.Model, "messages": []map[string]string{{"role": "system", "content": instruction}, {"role": "user", "content": string(input)}}}
	if p.Text != "" {
		payload["messages"] = []map[string]string{{"role": "user", "content": p.Text}}
	}
	if p.Schema != nil {
		if c.cfg.StructuredOutput {
			payload["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "goeval_result", "strict": true, "schema": p.Schema}}
		} else {
			payload["response_format"] = map[string]any{"type": "json_object"}
		}
	}
	if c.cfg.Temperature != nil {
		payload["temperature"] = *c.cfg.Temperature
	}
	if p.Temperature != nil {
		payload["temperature"] = *p.Temperature
	}
	if c.cfg.MaxTokens > 0 {
		payload["max_completion_tokens"] = c.cfg.MaxTokens
	}
	if p.LogProbs {
		n := p.TopLogProbs
		if n == 0 {
			n = 20
		}
		if n < 1 || n > 20 {
			return goeval.Completion{}, fmt.Errorf("openai: top logprobs must be in [1,20]")
		}
		payload["logprobs"] = true
		payload["top_logprobs"] = n
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
				Refusal string  `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
			LogProbs     struct {
				Content []goeval.TokenLogProb `json:"content"`
			} `json:"logprobs"`
		} `json:"choices"`
	}
	if err := c.request(ctx, "/chat/completions", payload, &out); err != nil {
		return goeval.Completion{}, err
	}
	if len(out.Choices) == 0 {
		return goeval.Completion{}, fmt.Errorf("openai: empty choices")
	}
	choice := out.Choices[0]
	if choice.Message.Refusal != "" {
		return goeval.Completion{}, fmt.Errorf("openai: model refused the request")
	}
	if choice.FinishReason == "length" || choice.FinishReason == "content_filter" {
		return goeval.Completion{}, fmt.Errorf("openai: incomplete generation (%s)", choice.FinishReason)
	}
	if choice.Message.Content == nil || strings.TrimSpace(*choice.Message.Content) == "" {
		return goeval.Completion{}, fmt.Errorf("openai: empty model content")
	}
	return goeval.Completion{Text: *choice.Message.Content, LogProbs: choice.LogProbs.Content}, nil
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if err := check.Context(ctx); err != nil {
		return nil, err
	}
	if len(texts) == 0 {
		return [][]float64{}, nil
	}
	if c.cfg.EmbeddingModel == "" {
		return nil, fmt.Errorf("openai: embedding model is required")
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := c.request(ctx, "/embeddings", map[string]any{"model": c.cfg.EmbeddingModel, "input": texts, "encoding_format": "float"}, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("openai: expected %d embeddings, got %d", len(texts), len(out.Data))
	}
	vectors := make([][]float64, len(texts))
	dim := 0
	for _, item := range out.Data {
		if item.Index < 0 || item.Index >= len(texts) || vectors[item.Index] != nil {
			return nil, fmt.Errorf("openai: invalid embedding index %d", item.Index)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("openai: empty embedding")
		}
		if dim == 0 {
			dim = len(item.Embedding)
		}
		if len(item.Embedding) != dim {
			return nil, fmt.Errorf("openai: inconsistent embedding dimensions")
		}
		for _, x := range item.Embedding {
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return nil, fmt.Errorf("openai: non-finite embedding")
			}
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}
