package testset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval"
)

func TestChunkAndGraphInterchange(t *testing.T) {
	nodes, err := Chunk(Document{ID: "doc", Content: "one two three four five six seven"}, 4, 1)
	if err != nil || len(nodes) != 2 || nodes[0].Text != "one two three four" || nodes[1].Text != "four five six seven" {
		t.Fatalf("%+v %v", nodes, err)
	}
	if _, err := Chunk(Document{Content: "words"}, 4, 4); err == nil {
		t.Fatal("invalid overlap accepted")
	}
	graph, err := BuildGraph(context.Background(), []Document{{ID: "doc", Content: "one two three four five six seven"}}, nil, nil, GraphConfig{ChunkSize: 4, ChunkOverlap: 1})
	if err != nil || len(graph.Edges) != 1 {
		t.Fatalf("%+v %v", graph, err)
	}
	var b bytes.Buffer
	if err := graph.WriteJSON(&b); err != nil {
		t.Fatal(err)
	}
	got, err := ReadGraph(&b)
	if err != nil || !reflect.DeepEqual(got, graph) {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := BuildGraph(context.Background(), []Document{{ID: "same", Content: "a"}, {ID: "same", Content: "b"}}, nil, nil, GraphConfig{}); err == nil {
		t.Fatal("duplicate node IDs accepted")
	}
}
func TestSeededMultiHopGenerationAndProvenance(t *testing.T) {
	graph := &KnowledgeGraph{Nodes: []Node{{ID: "a", Text: "Paris is in France."}, {ID: "b", Text: "France is in Europe."}}, Edges: []Edge{{Source: "a", Target: "b", Weight: 1}}}
	llm := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		input := p.Input.(map[string]any)
		if len(input["contexts"].([]string)) != 2 {
			t.Error("multi-hop did not receive two contexts")
		}
		return `{"user_input":"Which continent contains Paris?","reference":"Europe."}`, nil
	})
	g := Generator{LLM: llm}
	cfg := Config{Size: 8, Seed: 42, MaxWorkers: 3, Distribution: []Distribution{{MultiHopSpecific, 1}}, Personas: []Persona{{"Student", "Learning geography"}}}
	a, err := g.GenerateFromGraph(context.Background(), graph, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := g.GenerateFromGraph(context.Background(), graph, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || len(a.Samples) != 8 {
		t.Fatal("seeded generation changed")
	}
	for _, s := range a.Samples {
		if s.Synthesizer != MultiHopSpecific || len(s.ReferenceContexts) != 2 || s.Persona != "Student" {
			t.Fatalf("missing provenance: %+v", s)
		}
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"synthesizer_name":"multi_hop_specific"`) {
		t.Fatal("embedded sample swallowed provenance")
	}
	var restored Testset
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Samples[0].Synthesizer != MultiHopSpecific {
		t.Fatal("lost provenance on read")
	}
	if len(a.Dataset()) != 8 {
		t.Fatal("dataset conversion failed")
	}
}
func TestGenerationErrorsAndCancellation(t *testing.T) {
	graph := &KnowledgeGraph{Nodes: []Node{{ID: "a", Text: "passage"}}}
	var calls atomic.Int64
	g := Generator{LLM: goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) {
		calls.Add(1)
		return "", errors.New("provider failed")
	})}
	if _, err := g.GenerateFromGraph(context.Background(), graph, Config{Size: 1, Distribution: []Distribution{{MultiHopAbstract, 1}}}); err == nil || calls.Load() != 0 {
		t.Fatal("invalid graph triggered model requests")
	}
	partial, err := g.GenerateFromGraph(context.Background(), graph, Config{Size: 2})
	if err == nil || len(partial.Samples) != 0 || calls.Load() != 2 {
		t.Fatalf("%+v %v %d", partial, err, calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = g.GenerateFromGraph(ctx, graph, Config{Size: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestGraphEmbeddingsAndEnrichment(t *testing.T) {
	llm := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) {
		return `{"summary":"geography","entities":["France"]}`, nil
	})
	embedder := goeval.EmbedderFunc(func(context.Context, []string) ([][]float64, error) { return [][]float64{{1, 0}, {1, 0}}, nil })
	graph, err := BuildGraph(context.Background(), []Document{{ID: "a", Content: "Paris France"}, {ID: "b", Content: "France Europe"}}, llm, embedder, GraphConfig{Enrich: true})
	if err != nil || len(graph.Edges) != 1 || graph.Nodes[0].Summary != "geography" {
		t.Fatalf("%+v %v", graph, err)
	}
	bad := goeval.EmbedderFunc(func(context.Context, []string) ([][]float64, error) { return [][]float64{{0, 0}, {0, 0}}, nil })
	if _, err := BuildGraph(context.Background(), []Document{{Content: "passage"}}, nil, bad, GraphConfig{}); err == nil {
		t.Fatal("invalid embeddings accepted")
	}
}
