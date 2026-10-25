package cache

import (
	"context"
	"errors"
	"math"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

type suppliedStore struct {
	data   []byte
	cancel context.CancelFunc
}

func (s suppliedStore) Get(string) ([]byte, bool, error) {
	if s.cancel != nil {
		s.cancel()
	}
	return s.data, true, nil
}
func (s suppliedStore) Set(string, []byte) error { return nil }

func TestCacheHitStillHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) {
		t.Fatal("cache hit called provider")
		return "", nil
	})
	if _, err := LLM(m, suppliedStore{data: []byte("cached"), cancel: cancel}, "ns").Generate(ctx, goeval.Prompt{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestInvalidEmbeddingsAreNeverCachedOrReplayed(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `[[]]`, `[[1],[1,2]]`} {
		m := goeval.EmbedderFunc(func(context.Context, []string) ([][]float64, error) {
			t.Fatal("unexpected provider call")
			return nil, nil
		})
		if _, err := Embeddings(m, suppliedStore{data: []byte(raw)}, "ns").Embed(context.Background(), []string{"x"}); err == nil {
			t.Fatal("invalid cache accepted", raw)
		}
	}
	for _, vectors := range [][][]float64{{nil}, {{1}, {1, 2}}, {{math.NaN()}}} {
		memory, _ := NewMemory(4)
		m := goeval.EmbedderFunc(func(context.Context, []string) ([][]float64, error) { return vectors, nil })
		if _, err := Embeddings(m, memory, "ns").Embed(context.Background(), []string{"x"}); err == nil {
			t.Fatal("invalid provider vectors accepted")
		}
		if memory.lru.Len() != 0 {
			t.Fatal("invalid output cached")
		}
	}
	var nilModel goeval.LLMFunc
	memory, _ := NewMemory(1)
	if _, err := LLM(nilModel, memory, "ns").Generate(context.Background(), goeval.Prompt{}); err == nil {
		t.Fatal("typed nil model accepted")
	}
}
