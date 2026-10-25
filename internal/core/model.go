package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mahabbatzakariyayev/GoEval/internal/check"
)

// Prompt is a model request. Schema describes the requested JSON output.
type Prompt struct {
	Instruction string         `json:"instruction"`
	Input       any            `json:"input"`
	Schema      map[string]any `json:"schema,omitempty"`
	Text        string         `json:"text,omitempty"` // An upstream raw completion prompt, when set.
	Temperature *float64       `json:"temperature,omitempty"`
	LogProbs    bool           `json:"logprobs,omitempty"`
	TopLogProbs int            `json:"top_logprobs,omitempty"`
}

type TokenAlternative struct {
	Token   string  `json:"token"`
	LogProb float64 `json:"logprob"`
}
type TokenLogProb struct {
	Token       string             `json:"token"`
	LogProb     float64            `json:"logprob"`
	TopLogProbs []TokenAlternative `json:"top_logprobs"`
}
type Completion struct {
	Text     string         `json:"text"`
	LogProbs []TokenLogProb `json:"logprobs,omitempty"`
}

// LogProbLLM is an optional capability used by probability-weighted G-Eval.
// Ordinary custom LLMs remain valid and use Judge's raw-score fallback.
type LogProbLLM interface {
	LLM
	GenerateWithLogProbs(context.Context, Prompt) (Completion, error)
}

// LLM implementations must support concurrent calls and honor cancellation.
type LLM interface {
	Generate(context.Context, Prompt) (string, error)
}
type LLMFunc func(context.Context, Prompt) (string, error)

func (f LLMFunc) Generate(ctx context.Context, p Prompt) (string, error) { return f(ctx, p) }

// Embedder returns one finite, nonempty vector per input, in the same order.
type Embedder interface {
	Embed(context.Context, []string) ([][]float64, error)
}
type EmbedderFunc func(context.Context, []string) ([][]float64, error)

func (f EmbedderFunc) Embed(ctx context.Context, s []string) ([][]float64, error) { return f(ctx, s) }

// GenerateJSON deliberately rejects prose, truncated JSON, and extra values.
func GenerateJSON(ctx context.Context, llm LLM, p Prompt, out any) error {
	if err := check.Context(ctx); err != nil {
		return err
	}
	if check.Nil(llm) {
		return fmt.Errorf("goeval: LLM is required")
	}
	raw, err := llm.Generate(ctx, p)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace([]byte(raw)), []byte("null")) {
		return fmt.Errorf("goeval: model returned null instead of a JSON result")
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("goeval: invalid model JSON: %w", err)
	}
	return nil
}
