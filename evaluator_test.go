package goeval_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	goeval "github.com/mahabbatzakariyayev/GoEval"
)

func TestFriendlyDefaultEvaluator(t *testing.T) {
	e, err := goeval.New()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Evaluate(context.Background(), []goeval.Case{{Output: "Paris", Expected: "Paris"}})
	if err != nil || !r.Passed() || r.Means["exact_match"] != 1 {
		t.Fatal(r, err)
	}
	failed, err := e.Evaluate(context.Background(), []goeval.Case{{Output: "Paris", Expected: "Rome"}})
	if err != nil || failed.Passed() {
		t.Fatal(failed, err)
	}
	_, err = e.Check(context.Background(), goeval.Case{Output: "Paris", Expected: "Rome"})
	var gate *goeval.ThresholdError
	if !errors.As(err, &gate) || gate.Metric != "exact_match" {
		t.Fatal(err)
	}
	empty, err := e.Evaluate(context.Background(), nil)
	if err != nil || empty.Passed() {
		t.Fatal(empty, err)
	}
	if _, err := json.Marshal(r); err != nil {
		t.Fatal(err)
	}
}

func TestFriendlyMetricNamesAndSemantics(t *testing.T) {
	e, err := goeval.New(goeval.WithMetrics(goeval.ExactMatch(), goeval.NormalizedMatch(), goeval.Contains(), goeval.StringSimilarity()), goeval.WithConcurrency(2))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Evaluate(context.Background(), []goeval.Case{{Output: " Paris ", Expected: "Paris"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Means["exact_match"] != 0 || r.Means["normalized_match"] != 1 || r.Means["contains"] != 1 {
		t.Fatal(r.Means)
	}
	for name := range r.Means {
		if strings.Contains(name, "judge") {
			t.Fatal("compatibility name leaked", name)
		}
	}
}

func TestFriendlyJudgesUseBoundModelAndOptions(t *testing.T) {
	var calls atomic.Int32
	model := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		calls.Add(1)
		input := p.Input.(map[string]any)
		if _, ok := input["actual_output"]; ok {
			return `{"statements":["a","b"]}`, nil
		}
		return `{"verdicts":[{"verdict":"yes"},{"verdict":"no"}]}`, nil
	})
	e, err := goeval.New(goeval.WithMetrics(goeval.AnswerRelevance(model, goeval.Threshold(.8), goeval.WithoutReason())))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Evaluate(context.Background(), []goeval.Case{{Input: "q", Output: "a"}})
	if err != nil || r.Means["answer_relevance"] != .5 || r.Passed() || calls.Load() != 2 {
		t.Fatal(r, err, calls.Load())
	}
	if r.Rows[0].Scores["answer_relevance"].Details["verdicts"] == nil {
		t.Fatal("judgment evidence lost")
	}
}

func TestFriendlyCustomJudge(t *testing.T) {
	var calls atomic.Int32
	model := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		calls.Add(1)
		input := p.Input.(map[string]any)
		if input["criteria"] != nil {
			return `{"steps":["Check the answer against the reference."]}`, nil
		}
		content, _ := input["test_case_content"].(string)
		if !strings.Contains(content, "Expected Output:\nParis") {
			return "", fmt.Errorf("reference missing from custom judge")
		}
		return `{"score":9,"reason":"Correct."}`, nil
	})
	e, err := goeval.New(goeval.WithMetrics(goeval.Judge(model, "correctness", "Compare facts.", goeval.AgainstExpected(), goeval.Threshold(.8))))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r, err := e.Evaluate(context.Background(), []goeval.Case{{Input: "q", Output: "Paris", Expected: "Paris"}})
		if err != nil || !r.Passed() || r.Means["correctness"] != .9 {
			t.Fatal(r, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("evaluation steps were not reused", calls.Load())
	}
}

func TestFriendlyValidationAndExecutionErrors(t *testing.T) {
	for _, options := range [][]goeval.Option{
		{nil}, {goeval.WithMetrics()}, {goeval.WithMetrics(nil)},
		{goeval.WithConcurrency(-1)}, {goeval.WithTimeout(-time.Second)},
		{goeval.WithMetrics(goeval.Faithfulness(nil))},
		{goeval.WithMetrics(goeval.Judge(nil, "", ""))},
		{goeval.WithMetrics(goeval.AnswerRelevance(nil, goeval.Threshold(2)))},
	} {
		if _, err := goeval.New(options...); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
	want := errors.New("model error")
	m := goeval.MetricFunc{MetricName: "failure", Evaluate: func(context.Context, goeval.Sample) (goeval.Score, error) { return goeval.Score{}, want }}
	e, err := goeval.New(goeval.WithMetrics(m))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Evaluate(context.Background(), []goeval.Case{{}})
	if !errors.Is(err, want) || r == nil || r.Passed() {
		t.Fatal(r, err)
	}
}

func TestFriendlyCasesPreservePresenceAndMetadata(t *testing.T) {
	cases, err := goeval.ReadCases(strings.NewReader(`[{"input":"q","output":"a","context":[],"metadata":{"id":9007199254740993}}]`))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		s := cases[0].Sample()
		if err := s.Validate(goeval.Reference); err == nil {
			t.Fatal("missing expected answer invented")
		}
		if err := s.Validate(goeval.RetrievedContexts); err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := cases.WriteJSONL(&b); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), "9007199254740993") {
			t.Fatal(b.String())
		}
		cases, err = goeval.ReadCasesJSONL(strings.NewReader(b.String()))
		if err != nil {
			t.Fatal(err)
		}
	}
	e, err := goeval.New(goeval.WithMetrics(goeval.ToolCorrectness()))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Evaluate(context.Background(), []goeval.Case{{Input: "No action required"}})
	if err != nil || !r.Passed() {
		t.Fatal(r, err)
	}
}

func TestFriendlyEvaluatorConcurrentReuse(t *testing.T) {
	e, err := goeval.New(goeval.WithMetrics(goeval.NormalizedMatch()))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			r, err := e.Evaluate(context.Background(), []goeval.Case{{Output: " Paris ", Expected: "Paris"}})
			if err != nil || !r.Passed() {
				t.Fatal(r, err)
			}
		})
	}
}

func ExampleNew() {
	evaluator, err := goeval.New()
	if err != nil {
		panic(err)
	}
	report, err := evaluator.Evaluate(context.Background(), []goeval.Case{{Input: "Capital of France?", Output: "Paris", Expected: "Paris"}})
	if err != nil {
		panic(err)
	}
	fmt.Println(report.Passed())
	fmt.Println(report.Means["exact_match"])
	// Output:
	// true
	// 1
}
