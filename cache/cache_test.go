package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

type probabilisticModel struct{ calls atomic.Int32 }

func (m *probabilisticModel) Generate(context.Context, goeval.Prompt) (string, error) {
	m.calls.Add(1)
	return "plain", nil
}
func (m *probabilisticModel) GenerateWithLogProbs(context.Context, goeval.Prompt) (goeval.Completion, error) {
	m.calls.Add(1)
	return goeval.Completion{Text: "probabilistic", LogProbs: []goeval.TokenLogProb{{Token: "8", TopLogProbs: []goeval.TokenAlternative{{Token: "8", LogProb: -.5}}}}}, nil
}

func TestCachingPreservesLogProbCapability(t *testing.T) {
	memory, _ := NewMemory(8)
	model := &probabilisticModel{}
	cached := LLM(model, memory, "judge:model")
	probabilistic, ok := cached.(goeval.LogProbLLM)
	if !ok {
		t.Fatal("logprob capability hidden")
	}
	p := goeval.Prompt{Text: "judge"}
	out, err := probabilistic.GenerateWithLogProbs(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	out.LogProbs[0].TopLogProbs[0].Token = "mutated"
	for range 2 {
		out, err = probabilistic.GenerateWithLogProbs(context.Background(), p)
		if err != nil || out.Text != "probabilistic" || out.LogProbs[0].TopLogProbs[0].Token != "8" {
			t.Fatal(out, err)
		}
	}
	plain, err := cached.Generate(context.Background(), p)
	if err != nil || plain != "plain" || model.calls.Load() != 2 {
		t.Fatal(plain, err, model.calls.Load())
	}
	if _, ok := LLM(goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { return "", nil }), memory, "plain").(goeval.LogProbLLM); ok {
		t.Fatal("invented optional capability")
	}
}

func TestLRUEvictionAndCopies(t *testing.T) {
	m, _ := NewMemory(2)
	b := []byte("one")
	m.Set("a", b)
	b[0] = 'X'
	m.Set("b", []byte("two"))
	got, ok, _ := m.Get("a")
	if !ok || string(got) != "one" {
		t.Fatal(string(got))
	}
	got[0] = 'Y'
	m.Set("c", []byte("three"))
	if _, ok, _ := m.Get("b"); ok {
		t.Fatal("least recently used entry retained")
	}
	got, _, _ = m.Get("a")
	if string(got) != "one" {
		t.Fatal("cache data was mutated")
	}
}
func TestLLMAndDiskCache(t *testing.T) {
	store, err := NewDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	model := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { calls.Add(1); return `{"value":1}`, nil })
	wrapped := LLM(model, store, "provider/model/config")
	for i := 0; i < 2; i++ {
		if _, err := wrapped.Generate(context.Background(), goeval.Prompt{Instruction: "judge"}); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	other := LLM(model, store, "other-model")
	other.Generate(context.Background(), goeval.Prompt{Instruction: "judge"})
	if calls.Load() != 2 {
		t.Fatal("namespace collision")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := wrapped.Generate(ctx, goeval.Prompt{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestConcurrentStoreAndEmbeddingCopies(t *testing.T) {
	store, _ := NewMemory(10)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				store.Set("x", []byte("x"))
				store.Get("x")
			}
		}()
	}
	wg.Wait()
	var calls int
	model := goeval.EmbedderFunc(func(context.Context, []string) ([][]float64, error) { calls++; return [][]float64{{1, 2}}, nil })
	wrapped := Embeddings(model, store, "model")
	v, err := wrapped.Embed(context.Background(), []string{"x"})
	if err != nil {
		t.Fatal(err)
	}
	v[0][0] = 99
	v, err = wrapped.Embed(context.Background(), []string{"x"})
	if err != nil || v[0][0] != 1 || calls != 1 {
		t.Fatalf("%v %v %d", v, err, calls)
	}
}
