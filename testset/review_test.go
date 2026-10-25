package testset

import (
	"context"
	"errors"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval"
)

func TestGenerationWorkerContainsProviderPanic(t *testing.T) {
	g := Generator{LLM: goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { panic("provider panic") })}
	r, err := g.GenerateFromGraph(context.Background(), &KnowledgeGraph{Nodes: []Node{{ID: "n", Text: "Text"}}}, Config{Size: 2, MaxWorkers: 2})
	if err == nil || !strings.Contains(err.Error(), "provider panic") || r == nil || len(r.Samples) != 0 {
		t.Fatal(r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.GenerateFromGraph(ctx, nil, Config{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	//lint:ignore SA1012 Exercise nil-context validation.
	if _, err := g.Generate(nil, nil, Config{}); err == nil {
		t.Fatal("nil context accepted")
	}
}
