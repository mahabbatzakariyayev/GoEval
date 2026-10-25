package judge

import (
	"context"
	"math"
	"strings"
	"sync/atomic"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

type logModel struct {
	goeval.LLM
	completion goeval.Completion
}

func (m logModel) GenerateWithLogProbs(_ context.Context, p goeval.Prompt) (goeval.Completion, error) {
	return m.completion, nil
}

func TestWeightedScore(t *testing.T) {
	tokens := []goeval.TokenLogProb{
		{Token: "8", TopLogProbs: []goeval.TokenAlternative{{Token: "1", LogProb: 0}}},
		{Token: "8", TopLogProbs: []goeval.TokenAlternative{
			{Token: "8", LogProb: math.Log(.6)}, {Token: "6", LogProb: math.Log(.3)},
			{Token: "0", LogProb: math.Log(.005)}, {Token: " 9", LogProb: math.Log(.095)},
		}},
	}
	want := (8*.6 + 6*.3) / .9
	got, err := WeightedScore(8, tokens)
	if err != nil || math.Abs(got-want) > 1e-12 {
		t.Fatalf("%g %v", got, err)
	}
	if _, err := WeightedScore(7, tokens); err == nil {
		t.Fatal("missing token accepted")
	}
	got, err = WeightedScore(8, []goeval.TokenLogProb{{Token: "8", TopLogProbs: []goeval.TokenAlternative{{Token: "x", LogProb: 0}}}})
	if err != nil || got != 8 {
		t.Fatal(got, err)
	}
}

func TestGEvalStepsAndNormalization(t *testing.T) {
	m, err := GEval(scripted(t,
		stage{map[string]any{"criteria": "Compare factual accuracy.", "parameters": "Actual Output and Expected Output"}, `{"steps":["Compare facts.","Penalize contradictions."]}`},
		stage{map[string]any{"evaluation_steps": "1. Compare facts.\n2. Penalize contradictions.\n"}, `{"score":8,"reason":"Mostly correct."}`},
		stage{nil, `{"score":6,"reason":"Partially correct."}`}), GEvalOptions{Name: "correctness", Criteria: "Compare factual accuracy.", EvaluationParams: []SingleTurnParam{ActualOutput, ExpectedOutput}})
	if err != nil {
		t.Fatal(err)
	}
	measure(t, m, .8)
	measure(t, m, .6) // Generated steps are reused; row state is not.
}

func TestGEvalLogProbWeightingAndFallback(t *testing.T) {
	for _, token := range []string{"8", "missing"} {
		t.Run(token, func(t *testing.T) {
			model := logModel{completion: goeval.Completion{Text: `{"score":8,"reason":"Accurate."}`, LogProbs: []goeval.TokenLogProb{{Token: token, TopLogProbs: []goeval.TokenAlternative{{Token: "8", LogProb: math.Log(.5)}, {Token: "6", LogProb: math.Log(.5)}}}}}}
			m, err := GEval(model, GEvalOptions{Name: "accuracy", EvaluationParams: []SingleTurnParam{ActualOutput}, EvaluationSteps: []string{"Check facts."}})
			if err != nil {
				t.Fatal(err)
			}
			want := .7
			if token == "missing" {
				want = .8
			}
			measure(t, m, want)
		})
	}
}

func TestGEvalRubricsAndStrict(t *testing.T) {
	m, err := GEval(scripted(t, stage{map[string]any{"score_range": []int{2, 8}}, `{"score":5,"reason":"Middle of rubric."}`}), GEvalOptions{Name: "quality", EvaluationParams: []SingleTurnParam{ActualOutput}, EvaluationSteps: []string{"Check quality."}, Rubric: []Rubric{{ScoreRange: [2]int{6, 8}, ExpectedOutcome: "Good"}, {ScoreRange: [2]int{2, 5}, ExpectedOutcome: "Needs work"}}})
	if err != nil {
		t.Fatal(err)
	}
	measure(t, m, .5)
	strict, err := GEval(scripted(t, stage{nil, `{"score":0,"reason":"Did not meet every requirement."}`}), GEvalOptions{Options: Options{StrictMode: true}, Name: "strict", EvaluationParams: []SingleTurnParam{ActualOutput}, EvaluationSteps: []string{"Check quality."}})
	if err != nil {
		t.Fatal(err)
	}
	r := measure(t, strict, 0)
	if r.Success || r.Threshold != 1 {
		t.Fatal(r)
	}
	for _, cfg := range []GEvalOptions{
		{}, {Name: "x", EvaluationParams: []SingleTurnParam{ActualOutput}},
		{Name: "x", Criteria: "check", EvaluationParams: []SingleTurnParam{"invalid"}},
		{Name: "x", Criteria: "check", EvaluationParams: []SingleTurnParam{ActualOutput}, Rubric: []Rubric{{ScoreRange: [2]int{0, 5}}, {ScoreRange: [2]int{5, 10}}}},
	} {
		if _, err := GEval(nil, cfg); err == nil {
			t.Fatal("invalid options accepted", cfg)
		}
	}
}

func TestGEvalConcurrentStepCache(t *testing.T) {
	var generated atomic.Int32
	model := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		if strings.Contains(p.Text, "criteria") && p.Input.(map[string]any)["criteria"] != nil {
			generated.Add(1)
			return `{"steps":["Check accuracy."]}`, nil
		}
		return `{"score":10,"reason":"Correct."}`, nil
	})
	m, err := GEval(model, GEvalOptions{Name: "accuracy", Criteria: "Check accuracy.", EvaluationParams: []SingleTurnParam{ActualOutput}})
	if err != nil {
		t.Fatal(err)
	}
	d := make(Dataset, 30)
	for i := range d {
		d[i] = allFields()
	}
	r, err := Evaluate(context.Background(), d, []Metric{m}, goeval.RunConfig{MaxWorkers: 8})
	if err != nil || r.Means[m.Name()] != 1 || generated.Load() != 1 {
		t.Fatalf("%+v %v generated=%d", r, err, generated.Load())
	}
}
