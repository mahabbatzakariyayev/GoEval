package scorers

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestStableStatisticsAndBetaExtremes(t *testing.T) {
	if got := fbeta(0, 1, 0); got != 0 {
		t.Fatal(got)
	}
	if got := fbeta(1, 0, 0); got != 1 {
		t.Fatal(got)
	}
	if got := fbeta(.4, .8, math.MaxFloat64); math.Abs(got-.8) > 1e-12 {
		t.Fatal(got)
	}
	for _, scale := range []float64{1, 1e-300, 1e300} {
		got, err := PearsonCorrelation([]float64{-scale, 0, scale}, []float64{scale, 0, -scale})
		if err != nil || math.Abs(got+1) > 1e-12 {
			t.Fatal(scale, got, err)
		}
	}
	threshold := .5
	m := NonLLMContextRecall(ContextMatchOptions{Threshold: &threshold})
	threshold = 1
	r, err := m.Score(context.Background(), goeval.Sample{RetrievedContexts: []string{"match"}, ReferenceContexts: []string{"match"}})
	if err != nil || r.Value != 1 {
		t.Fatal(r, err)
	}
}

func TestToolNumbersDoNotExpandHugeExponents(t *testing.T) {
	a, err := toolKey(goeval.ToolCall{Name: "number", Arguments: map[string]any{"n": json.Number("1e1000000000")}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := toolKey(goeval.ToolCall{Name: "number", Arguments: map[string]any{"n": json.Number("10e999999999")}})
	if err != nil || a != b {
		t.Fatal(a, b, err)
	}
}
