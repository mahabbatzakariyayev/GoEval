package judge

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

type stage struct {
	input  map[string]any
	output string
}

func scripted(t *testing.T, stages ...stage) goeval.LLM {
	t.Helper()
	var mu sync.Mutex
	i := 0
	t.Cleanup(func() {
		if i != len(stages) {
			t.Errorf("called %d of %d expected stages", i, len(stages))
		}
	})
	return goeval.LLMFunc(func(ctx context.Context, p goeval.Prompt) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if i >= len(stages) {
			t.Errorf("unexpected judge call: %v", p.Input)
			return "", errors.New("unexpected call")
		}
		s := stages[i]
		i++
		if p.Text == "" || p.Schema == nil {
			t.Error("missing original prompt/schema")
		}
		input, ok := p.Input.(map[string]any)
		if !ok {
			t.Fatalf("unstructured input: %T", p.Input)
		}
		for k, want := range s.input {
			gotJSON, _ := json.Marshal(input[k])
			wantJSON, _ := json.Marshal(want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("stage %d %s: got %s want %s", i, k, gotJSON, wantJSON)
			}
		}
		return s.output, nil
	})
}

func allFields() Case {
	return Case{Input: "question", ActualOutput: "answer", ExpectedOutput: "reference", Context: []string{"source"}, RetrievalContext: []string{"passage 1", "passage 2"}, ToolsCalled: []ToolCall{}, ExpectedTools: []ToolCall{}}
}

func measure(t *testing.T, m Metric, want float64, cases ...Case) Result {
	t.Helper()
	c := allFields()
	if len(cases) > 0 {
		c = cases[0]
	}
	r, err := m.Measure(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(r.Score-want) > 1e-12 {
		t.Fatalf("%s score %g want %g", m.Name(), r.Score, want)
	}
	return r
}

func TestAnswerRelevancyPipeline(t *testing.T) {
	llm := scripted(t,
		stage{map[string]any{"actual_output": "answer"}, `{"statements":["direct","ambiguous","off topic"]}`},
		stage{map[string]any{"input": "question", "statements": []string{"direct", "ambiguous", "off topic"}}, `{"verdicts":[{"verdict":"YES."},{"verdict":"idk","reason":"ambiguous"},{"verdict":"no","reason":"off topic"}]}`},
		stage{map[string]any{"score": "0.67", "irrelevant_statements": []string{"off topic"}}, `{"reason":"Two relevant or ambiguous statements."}`})
	r := measure(t, AnswerRelevancyMetric(llm, Options{}), 2.0/3)
	if !r.Success || r.Reason == "" {
		t.Fatal(r)
	}
}

func TestUnrecognizedJudgmentsCannotProducePassingScores(t *testing.T) {
	llm := scripted(t,
		stage{nil, `{"statements":["a claim"]}`},
		stage{nil, `{"verdicts":[{"verdict":"unknown"}]}`},
	)
	_, err := AnswerRelevancyMetric(llm, Options{IncludeReason: Bool(false)}).Measure(context.Background(), allFields())
	if err == nil || !strings.Contains(err.Error(), "unrecognized verdict") {
		t.Fatal("invalid judgment accepted", err)
	}
}

func TestEveryExtractedItemNeedsAJudgment(t *testing.T) {
	for _, output := range []string{`{"verdicts":[]}`, `{"verdicts":[{"verdict":"yes"}]}`} {
		t.Run(output, func(t *testing.T) {
			llm := scripted(t, stage{nil, `{"statements":["one","two"]}`}, stage{nil, output})
			_, err := AnswerRelevancyMetric(llm, Options{IncludeReason: Bool(false)}).Measure(context.Background(), allFields())
			if err == nil || !strings.Contains(err.Error(), "expected 2 verdicts") {
				t.Fatal("incomplete judgments accepted", err)
			}
		})
	}
	llm := scripted(t, stage{nil, `{"truths":["fact"]}`}, stage{nil, `{"claims":["one","two"]}`}, stage{nil, `{"verdicts":[]}`})
	if _, err := FaithfulnessMetric(llm, Options{IncludeReason: Bool(false)}).Measure(context.Background(), allFields()); err == nil {
		t.Fatal("unjudged claims passed")
	}
	llm = scripted(t, stage{nil, `{"opinions":["one","two"]}`}, stage{nil, `{"verdicts":[]}`})
	if _, err := ToxicityMetric(llm, Options{IncludeReason: Bool(false)}).Measure(context.Background(), allFields()); err == nil {
		t.Fatal("unjudged opinions passed")
	}
}

func TestFaithfulnessPipeline(t *testing.T) {
	for _, penalize := range []bool{false, true} {
		t.Run(map[bool]string{false: "allow_ambiguous", true: "penalize_ambiguous"}[penalize], func(t *testing.T) {
			llm := scripted(t,
				stage{map[string]any{"retrieval_context": "passage 1\n\npassage 2", "limit": " FACTUAL, undisputed truths"}, `{"truths":["fact 1","fact 2"]}`},
				stage{map[string]any{"actual_output": "answer"}, `{"claims":["claim 1","claim 2","claim 3"]}`},
				stage{map[string]any{"retrieval_context": "fact 1\n\nfact 2"}, `{"verdicts":[{"verdict":"yes"},{"verdict":"borderline"},{"verdict":"no"}]}`})
			want := 2.0 / 3
			if penalize {
				want = 1.0 / 3
			}
			measure(t, FaithfulnessMetric(llm, Options{IncludeReason: Bool(false), PenalizeAmbiguousClaims: penalize}), want)
		})
	}
}

func TestContextualMetrics(t *testing.T) {
	precisionCase := allFields()
	precisionCase.RetrievalContext = []string{"passage 1", "passage 2", "passage 3"}
	measure(t, ContextualPrecisionMetric(scripted(t,
		stage{map[string]any{"input": "question", "expected_output": "reference", "context_to_display": precisionCase.RetrievalContext}, `{"verdicts":[{"verdict":"yes"},{"verdict":"no"},{"verdict":"yes"}]}`},
		stage{map[string]any{"score": "0.83"}, `{"reason":"Relevant items at ranks one and three."}`}), Options{}), 5.0/6, precisionCase)
	measure(t, ContextualRecallMetric(scripted(t,
		stage{map[string]any{"expected_output": "reference"}, `{"verdicts":[{"verdict":"yes","reason":"supported"},{"verdict":"no","reason":"missing"}]}`},
		stage{map[string]any{"supportive_reasons": []string{"supported"}, "unsupportive_reasons": []string{"missing"}}, `{"reason":"Half supported."}`}), Options{}), 0.5)
	measure(t, ContextualRelevancyMetric(scripted(t,
		stage{map[string]any{"context": "passage 1"}, `{"verdicts":[{"statement":"fact","verdict":"yes"},{"statement":"noise","verdict":"no","reason":"irrelevant"}]}`},
		stage{map[string]any{"context": "passage 2"}, `{"verdicts":[{"statement":"fact 2","verdict":"yes"}]}`},
		stage{map[string]any{"relevant_statements": []string{"fact", "fact 2"}, "irrelevant_statements": []string{"irrelevant"}}, `{"reason":"Two of three relevant."}`}), Options{}), 2.0/3)
}

func TestSafetyMetricsHigherIsBetter(t *testing.T) {
	c := allFields()
	c.Context = []string{"source", "contradiction"}
	measure(t, HallucinationMetric(scripted(t,
		stage{map[string]any{"contexts": c.Context}, `{"verdicts":[{"verdict":"yes","reason":"matches"},{"verdict":"no","reason":"contradicts"}]}`},
		stage{map[string]any{"factual_alignments": []string{"matches"}, "contradictions": []string{"contradicts"}}, `{"reason":"Half faithful."}`}), Options{}), 0.5, c)
	for _, factory := range []func(goeval.LLM, Options) Metric{BiasMetric, ToxicityMetric} {
		m := factory(scripted(t,
			stage{nil, `{"opinions":["opinion 1","opinion 2"]}`},
			stage{nil, `{"verdicts":[{"verdict":"no"},{"verdict":"yes","reason":"unsafe"}]}`},
			stage{map[string]any{"score": "0.50"}, `{"reason":"One safe opinion."}`}), Options{})
		measure(t, m, 0.5)
	}
}

func TestEmptyAndStrictJudgments(t *testing.T) {
	for _, factory := range []func(goeval.LLM, Options) Metric{AnswerRelevancyMetric, BiasMetric, ToxicityMetric} {
		field := "opinions"
		if factory(nil, Options{}).Name() == "judge.answer_relevancy" {
			field = "statements"
		}
		measure(t, factory(scripted(t, stage{nil, `{"` + field + `":[]}`}), Options{IncludeReason: Bool(false)}), 1)
	}
	emptyContext := allFields()
	emptyContext.RetrievalContext = []string{}
	measure(t, ContextualPrecisionMetric(scripted(t, stage{nil, `{"verdicts":[]}`}), Options{IncludeReason: Bool(false)}), 0, emptyContext)
	measure(t, ContextualRecallMetric(scripted(t, stage{nil, `{"verdicts":[]}`}), Options{IncludeReason: Bool(false)}), 0)
	r := measure(t, AnswerRelevancyMetric(scripted(t,
		stage{nil, `{"statements":["a","b"]}`}, stage{nil, `{"verdicts":[{"verdict":"yes"},{"verdict":"no"}]}`},
		stage{map[string]any{"score": "0.00"}, `{"reason":"Strict failure."}`}), Options{StrictMode: true}), 0)
	if r.Success || r.Threshold != 1 {
		t.Fatal(r)
	}
}

func TestAgentJudges(t *testing.T) {
	measure(t, ArgumentCorrectnessMetric(nil, Options{}), 1) // No model call for an empty tool list.
	measure(t, TaskCompletionMetric(scripted(t,
		stage{map[string]any{"input": "question"}, `{"task":"inferred","outcome":"partial"}`},
		stage{map[string]any{"task": "override", "actual_outcome": "partial"}, `{"verdict":0.75,"reason":"Mostly complete."}`}), TaskCompletionOptions{Task: "override"}), 0.75)
	measure(t, PromptAlignmentMetric(scripted(t,
		stage{map[string]any{"prompt_instructions": []string{"be concise", "be accurate"}}, `{"verdicts":[{"verdict":"yes"},{"verdict":"no","reason":"incorrect"}]}`},
		stage{map[string]any{"unalignment_reasons": []string{"incorrect"}}, `{"reason":"Partially follows instructions."}`}), []string{"be concise", "be accurate"}, Options{}), 0.5)
	c := allFields()
	c.ToolsCalled = []ToolCall{{Name: "search", InputParameters: map[string]any{"q": "Paris"}}}
	m := ArgumentCorrectnessMetric(scripted(t, stage{nil, `{"verdicts":[{"verdict":"no","reason":"wrong query"}]}`}, stage{map[string]any{"incorrect_tool_calls_reasons": []string{"wrong query"}}, `{"reason":"Wrong arguments."}`}), Options{})
	if r, err := m.Measure(context.Background(), c); err != nil || r.Score != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestSummarizationPipeline(t *testing.T) {
	m := SummarizationMetric(scripted(t,
		stage{map[string]any{"retrieval_context": "question"}, `{"truths":["fact"]}`},
		stage{nil, `{"claims":["fact","fluff"]}`},
		stage{map[string]any{"n": 5, "text": "question"}, `{"questions":["q1","q2","q3"]}`},
		stage{map[string]any{"text": "question"}, `{"answers":["yes","yes","no"]}`},
		stage{map[string]any{"text": "answer"}, `{"answers":["yes","no","no"]}`},
		stage{map[string]any{"original_text": "fact"}, `{"verdicts":[{"verdict":"yes"},{"verdict":"borderline","reason":"fluff"}]}`},
		stage{map[string]any{"score": "0.50", "redundancies": []string{"fluff"}}, `{"reason":"Half coverage, half alignment."}`}), SummarizationOptions{})
	r := measure(t, m, 0.5)
	if r.Details["score_breakdown"].(map[string]float64)["Alignment"] != 0.5 {
		t.Fatal(r)
	}
}

func TestMalformedAndCancelledJudges(t *testing.T) {
	for _, output := range []string{`not json`, `{}`, `{"statements":null}`, `{"statements":[5]}`} {
		m := AnswerRelevancyMetric(scripted(t, stage{nil, output}), Options{IncludeReason: Bool(false)})
		if _, err := m.Measure(context.Background(), allFields()); err == nil {
			t.Fatalf("accepted %s", output)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExactMatchMetric(Options{}).Measure(ctx, allFields()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := ExactMatchMetric(Options{Threshold: Float(math.NaN())}).Measure(context.Background(), allFields()); err == nil {
		t.Fatal("accepted NaN threshold")
	}
}

func TestCombinedRunnerPreservesDistinctAlgorithms(t *testing.T) {
	c := Case{ActualOutput: " Paris ", ExpectedOutput: "Paris"}
	r, err := goeval.Evaluate(context.Background(), Dataset{c}.Samples(), []goeval.Metric{scorers.ExactMatch(), ExactMatchMetric(Options{})}, goeval.RunConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Means["exact_match"] != 0 || r.Means["judge.exact_match"] != 1 {
		t.Fatal(r)
	}
	s := r.Rows[0].Scores["judge.exact_match"]
	if s.Success == nil || !*s.Success || s.Threshold == nil || *s.Threshold != 1 {
		t.Fatal(s)
	}
	var roundTrip goeval.Dataset
	data, err := json.Marshal(Dataset{c}.Samples())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if _, err := goeval.Check(context.Background(), roundTrip[0], []goeval.Metric{ExactMatchMetric(Options{})}, goeval.RunConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestDeepDatasetInterchange(t *testing.T) {
	d, err := ReadJSONL(strings.NewReader(`{"input":"", "actualOutput":"", "expectedOutput":"", "retrievalContext":[], "toolsCalled":[{"name":"search","inputParameters":{"id":9007199254740993}}], "expectedTools":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := d[0].Validate(Input, ActualOutput, ExpectedOutput, RetrievalContext, ToolsCalled, ExpectedTools); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := d.WriteJSONL(&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "9007199254740993") {
		t.Fatal(b.String())
	}
	reloaded, err := ReadJSONL(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded[0].Validate(RetrievalContext, ExpectedTools); err != nil {
		t.Fatal(err)
	}
	missing, err := ReadJSON(strings.NewReader(`[{"input":"q","actual_output":"a"}]`))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := missing[0].Validate(ExpectedOutput); err == nil {
			t.Fatal("missing expected_output accepted")
		}
		var out strings.Builder
		if err := missing.WriteJSON(&out); err != nil {
			t.Fatal(err)
		}
		missing, err = ReadJSON(strings.NewReader(out.String()))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ReadJSON(strings.NewReader(`[{"actualOutput":"a","actual_output":"b"}]`)); err == nil {
		t.Fatal("ambiguous aliases accepted")
	}
	var ordinary goeval.Sample
	if err := json.Unmarshal([]byte(`{"user_input":"q","response":"a","metadata":{"source":"application"}}`), &ordinary); err != nil {
		t.Fatal(err)
	}
	converted, err := FromSample(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	if err := converted.Validate(ExpectedOutput); err == nil {
		t.Fatal("lost source presence when converting a shared sample")
	}
	if converted.Metadata["source"] != "application" {
		t.Fatal(converted.Metadata)
	}
	if _, err := ExactMatchMetric(Options{}).Score(context.Background(), ordinary); err == nil {
		t.Fatal("direct Score accepted absent reference")
	}
	if _, err := ExactMatchMetric(Options{}).Measure(context.Background(), Case{Multimodal: true}); err == nil {
		t.Fatal("unsupported multimodal data accepted")
	}
}
