package judge

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func TestOptionsAreOwnedByConstructedMetric(t *testing.T) {
	threshold := .8
	include := false
	m := ExactMatchMetric(Options{Threshold: &threshold, IncludeReason: &include})
	threshold = math.NaN()
	include = true
	r := measure(t, m, 0)
	if r.Threshold != .8 || r.Reason != "" {
		t.Fatal(r)
	}
	limit := 1
	faithful := FaithfulnessMetric(scripted(t,
		stage{map[string]any{"limit": " the single most important FACTUAL, undisputed truth"}, `{"truths":[]}`},
		stage{nil, `{"claims":[]}`}), Options{TruthsExtractionLimit: &limit, IncludeReason: Bool(false)})
	limit = 2
	measure(t, faithful, 1)
}

func TestSampleConversionPreservesAbsenceAndUpdatedEmptyValues(t *testing.T) {
	d, err := ReadJSON(strings.NewReader(`[{"input":"q","actual_output":null}]`))
	if err != nil {
		t.Fatal(err)
	}
	s := d[0].ToSample()
	if err := s.Validate(goeval.Reference); err == nil {
		t.Fatal("conversion invented a reference")
	}
	if _, err := goeval.Evaluate(context.Background(), goeval.Dataset{s}, []goeval.Metric{scorers.ExactMatch()}, goeval.RunConfig{}); err == nil {
		t.Fatal("GoEval accepted missing Judge reference")
	}
	updated := s.WithResponse("")
	c, err := FromSample(updated)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(ActualOutput); err != nil {
		t.Fatal("new empty response not marked present", err)
	}
	if err := d[0].Validate(ActualOutput); err == nil {
		t.Fatal("original case mutated")
	}
	c, err = FromSample(goeval.Sample{Messages: []goeval.Message{{Role: "user", Content: "q"}}, ReferenceToolCalls: []goeval.ToolCall{}})
	if err != nil || c.ToolsCalled == nil || len(c.ToolsCalled) != 0 {
		t.Fatal(c, err)
	}
}

func TestSharedRunnerRetainsJudgmentDetails(t *testing.T) {
	m := AnswerRelevancyMetric(scripted(t, stage{nil, `{"statements":[]}`}), Options{IncludeReason: Bool(false)})
	r, err := Evaluate(context.Background(), Dataset{allFields()}, []Metric{m}, goeval.RunConfig{})
	if err != nil || r.Rows[0].Scores[m.Name()].Details["statements"] == nil {
		t.Fatal(r, err)
	}
	if !semanticEqual(json.Number("1e1000000000"), json.Number("10e999999999")) {
		t.Fatal("large equivalent numbers differ")
	}
	if _, err := WeightedScore(8, []goeval.TokenLogProb{{Token: "8", TopLogProbs: []goeval.TokenAlternative{{Token: "8", LogProb: math.NaN()}}}}); err == nil {
		t.Fatal("invalid probabilities accepted")
	}
}

func TestJudgePreflightRunsBeforeAnyModelCalls(t *testing.T) {
	called := false
	llm := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { called = true; return `{"statements":[]}`, nil })
	_, err := goeval.Evaluate(context.Background(), Dataset{{Input: "q", ActualOutput: "a"}}.Samples(), []goeval.Metric{
		AnswerRelevancyMetric(llm, Options{IncludeReason: Bool(false)}),
		ToolCorrectnessMetric(nil, ToolCorrectnessOptions{}),
	}, goeval.RunConfig{})
	if err == nil || called {
		t.Fatal("a model was called before validating all inputs", err)
	}
}
