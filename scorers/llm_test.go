package scorers

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/internal/prompts"
)

type step struct {
	prompt string
	input  any
	output string
}

func scripted(t *testing.T, steps []step) (goeval.LLM, func()) {
	t.Helper()
	var calls atomic.Int64
	llm := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		i := int(calls.Add(1)) - 1
		if i >= len(steps) {
			t.Errorf("unexpected model call %d: %s", i, p.Instruction)
			return "", errors.New("unexpected model call")
		}
		s := steps[i]
		if s.prompt != "" && !strings.HasPrefix(p.Instruction, prompts.Instruction(s.prompt)) {
			t.Errorf("step %d: wrong upstream prompt", i)
		}
		if s.input != nil {
			want, _ := json.Marshal(s.input)
			got, _ := json.Marshal(p.Input)
			if string(want) != string(got) {
				t.Errorf("step %d input: got %s want %s", i, got, want)
			}
		}
		return s.output, nil
	})
	return llm, func() {
		t.Helper()
		if int(calls.Load()) != len(steps) {
			t.Errorf("got %d model calls, want %d", calls.Load(), len(steps))
		}
	}
}
func assertValue(t *testing.T, m goeval.Metric, s goeval.Sample, want float64) {
	t.Helper()
	got, err := m.Score(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Value-want) > 1e-10 {
		t.Fatalf("%s: got %.14g want %.14g", m.Name(), got.Value, want)
	}
}
func TestFaithfulnessStages(t *testing.T) {
	llm, done := scripted(t, []step{
		{"faithfulness.StatementGeneratorPrompt", map[string]any{"question": "Q", "answer": "A"}, `{"statements":["supported","unsupported"]}`},
		{"faithfulness.NLIStatementPrompt", map[string]any{"context": "C1\nC2", "statements": []string{"supported", "unsupported"}}, `{"statements":[{"statement":"supported","reason":"yes","verdict":1},{"statement":"unsupported","reason":"no","verdict":0}]}`},
	})
	defer done()
	assertValue(t, Faithfulness(llm), goeval.Sample{UserInput: "Q", Response: "A", RetrievedContexts: []string{"C1", "C2"}}, 0.5)
}
func TestContextRecallUsesOneClassification(t *testing.T) {
	llm, done := scripted(t, []step{{"context_recall.ContextRecallClassificationPrompt", map[string]string{"question": "Q", "context": "C", "answer": "R"}, `{"classifications":[{"statement":"one","attributed":1,"reason":"yes"},{"statement":"two","attributed":0,"reason":"no"}]}`}})
	defer done()
	assertValue(t, ContextRecall(llm), goeval.Sample{UserInput: "Q", Reference: "R", RetrievedContexts: []string{"C"}}, 0.5)
}
func TestContextPrecisionPreservesRankingAndAnswerSource(t *testing.T) {
	for _, reference := range []bool{true, false} {
		t.Run(map[bool]string{true: "reference", false: "response"}[reference], func(t *testing.T) {
			steps := []step{}
			answer := "R"
			if !reference {
				answer = "A"
			}
			for i, c := range []string{"C1", "C2", "C3"} {
				out := `{"verdict":1,"reason":"useful"}`
				if i == 1 {
					out = `{"verdict":0,"reason":"irrelevant"}`
				}
				steps = append(steps, step{"context_precision.ContextPrecisionPrompt", map[string]any{"question": "Q", "answer": answer, "context": c}, out})
			}
			llm, done := scripted(t, steps)
			defer done()
			assertValue(t, contextPrecision(llm, reference), goeval.Sample{UserInput: "Q", Response: "A", Reference: "R", RetrievedContexts: []string{"C1", "C2", "C3"}}, (1+2.0/3)/(2+1e-10))
		})
	}
}
func TestAnswerCorrectnessUsesClassifier(t *testing.T) {
	llm, done := scripted(t, []step{
		{"faithfulness.StatementGeneratorPrompt", map[string]any{"question": "Q", "answer": "A"}, `{"statements":["A1","A2"]}`},
		{"faithfulness.StatementGeneratorPrompt", map[string]any{"question": "Q", "answer": "R"}, `{"statements":["R1","R2"]}`},
		{"answer_correctness.CorrectnessClassifier", map[string]any{"question": "Q", "answer": []string{"A1", "A2"}, "ground_truth": []string{"R1", "R2"}}, `{"TP":[{"statement":"A1","reason":"yes"}],"FP":[{"statement":"A2","reason":"no"}],"FN":[{"statement":"R2","reason":"missing"}]}`},
	})
	defer done()
	embedder := goeval.EmbedderFunc(func(_ context.Context, texts []string) ([][]float64, error) {
		if !reflect.DeepEqual(texts, []string{"A", "R"}) {
			t.Error(texts)
		}
		return [][]float64{{1, 0}, {1, 0}}, nil
	})
	assertValue(t, AnswerCorrectness(llm, embedder, AnswerCorrectnessOptions{}), goeval.Sample{UserInput: "Q", Response: "A", Reference: "R"}, 0.625)
}
func TestFactualCorrectnessUsesClaimDecompositionAndNLI(t *testing.T) {
	llm, done := scripted(t, []step{
		{"factual_correctness.ClaimDecompositionPrompt", map[string]string{"response": "A"}, `{"claims":["A1","A2"]}`},
		{"faithfulness.NLIStatementPrompt", map[string]any{"context": "R", "statements": []string{"A1", "A2"}}, `{"statements":[{"statement":"A1","verdict":1,"reason":"yes"},{"statement":"A2","verdict":0,"reason":"no"}]}`},
		{"factual_correctness.ClaimDecompositionPrompt", map[string]string{"response": "R"}, `{"claims":["R1","R2"]}`},
		{"faithfulness.NLIStatementPrompt", map[string]any{"context": "A", "statements": []string{"R1", "R2"}}, `{"statements":[{"statement":"R1","verdict":1,"reason":"yes"},{"statement":"R2","verdict":0,"reason":"no"}]}`},
	})
	defer done()
	assertValue(t, FactualCorrectness(llm, FactualOptions{}), goeval.Sample{Response: "A", Reference: "R"}, 0.5)
}
func TestAnswerRelevancyNoncommittalRule(t *testing.T) {
	for _, all := range []bool{true, false} {
		t.Run(map[bool]string{true: "all", false: "mixed"}[all], func(t *testing.T) {
			steps := []step{{"answer_relevance.ResponseRelevancePrompt", nil, `{"question":"Q1","noncommittal":1}`}, {"answer_relevance.ResponseRelevancePrompt", nil, `{"question":"Q2","noncommittal":1}`}}
			if !all {
				steps[1].output = `{"question":"Q2","noncommittal":0}`
			}
			llm, done := scripted(t, steps)
			defer done()
			embedder := goeval.EmbedderFunc(func(_ context.Context, texts []string) ([][]float64, error) {
				return [][]float64{{1, 0}, {1, 0}, {0, 1}}, nil
			})
			want := 0.5
			if all {
				want = 0
			}
			assertValue(t, AnswerRelevancy(llm, embedder, 2), goeval.Sample{UserInput: "Q", Response: "A"}, want)
		})
	}
}
func TestMalformedJudgmentsAreErrors(t *testing.T) {
	for _, raw := range []string{`{}`, `{"statements":["one"]} trailing`, `{"statements":null}`, `{"statements":[""]}`} {
		llm := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { return raw, nil })
		if _, err := Faithfulness(llm).Score(context.Background(), goeval.Sample{RetrievedContexts: []string{}}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	llm, done := scripted(t, []step{{"faithfulness.StatementGeneratorPrompt", nil, `{"statements":["one"]}`}, {"faithfulness.NLIStatementPrompt", nil, `{"statements":[{"statement":"one","reason":"missing verdict"}]}`}})
	defer done()
	if _, err := Faithfulness(llm).Score(context.Background(), goeval.Sample{RetrievedContexts: []string{}}); err == nil {
		t.Fatal("missing verdict became zero")
	}
}
func TestCustomJudges(t *testing.T) {
	llm := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { return `{"value":"pass","reason":"good"}`, nil })
	d := DiscreteMetric(llm, "check", "check quality", []string{"pass", "fail"})
	m, err := d.AsMetric(map[string]float64{"pass": 1, "fail": 0})
	if err != nil {
		t.Fatal(err)
	}
	assertValue(t, m, goeval.Sample{}, 1)
	if _, err := d.AsMetric(map[string]float64{"pass": 1}); err == nil {
		t.Fatal("incomplete mapping accepted")
	}
	bad := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { return `{"value":99,"reason":"bad"}`, nil })
	if _, err := NumericMetric(bad, "quality", "score quality", 0, 1).Score(context.Background(), goeval.Sample{}); err == nil {
		t.Fatal("out-of-range score accepted")
	}
	rank := RankingMetric(goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { return `{"value":["a","a"],"reason":"bad"}`, nil }), "rank", "rank items", []string{"a", "b"})
	if _, err := rank.Score(context.Background(), goeval.Sample{}); err == nil {
		t.Fatal("duplicate rank accepted")
	}
}
func TestNvidiaUsesTwoRawPromptsAndShortcuts(t *testing.T) {
	var calls atomic.Int64
	llm := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		if p.Text == "" || p.Temperature == nil || *p.Temperature != 0.1 {
			t.Error("missing upstream raw prompt parameters")
		}
		if calls.Add(1) == 1 {
			return "4", nil
		}
		return "2", nil
	})
	assertValue(t, AnswerAccuracy(llm), goeval.Sample{UserInput: "Q", Response: "A", Reference: "R"}, 0.75)
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
	assertValue(t, ResponseGroundedness(nil), goeval.Sample{Response: "supported", RetrievedContexts: []string{"This is supported text"}}, 1)
}

func TestNoiseSensitivityAttributionAndExclusion(t *testing.T) {
	for _, mode := range []string{"relevant", "irrelevant"} {
		t.Run(mode, func(t *testing.T) {
			steps := []step{
				{"faithfulness.StatementGeneratorPrompt", nil, `{"statements":["G"]}`},
				{"faithfulness.StatementGeneratorPrompt", nil, `{"statements":["A1","A2","A3"]}`},
				{"faithfulness.NLIStatementPrompt", map[string]any{"context": "relevant", "statements": []string{"G"}}, `{"statements":[{"statement":"G","verdict":1,"reason":"yes"}]}`},
				{"faithfulness.NLIStatementPrompt", nil, `{"statements":[{"statement":"A1","verdict":1},{"statement":"A2","verdict":1},{"statement":"A3","verdict":0}]}`},
				{"faithfulness.NLIStatementPrompt", map[string]any{"context": "noise", "statements": []string{"G"}}, `{"statements":[{"statement":"G","verdict":0}]}`},
				{"faithfulness.NLIStatementPrompt", nil, `{"statements":[{"statement":"A1","verdict":0},{"statement":"A2","verdict":1},{"statement":"A3","verdict":1}]}`},
				{"faithfulness.NLIStatementPrompt", map[string]any{"context": "R", "statements": []string{"A1", "A2", "A3"}}, `{"statements":[{"statement":"A1","verdict":1},{"statement":"A2","verdict":0},{"statement":"A3","verdict":0}]}`},
			}
			llm, done := scripted(t, steps)
			defer done()
			assertValue(t, NoiseSensitivity(llm, mode), goeval.Sample{UserInput: "Q", Response: "A", Reference: "R", RetrievedContexts: []string{"relevant", "noise"}}, 1.0/3)
		})
	}
}
func TestEntityRecallAndSummaryStages(t *testing.T) {
	llm, done := scripted(t, []step{{"context_entities_recall.ExtractEntitiesPrompt", map[string]string{"text": "R"}, `{"entities":["Paris","France","Paris"]}`}, {"context_entities_recall.ExtractEntitiesPrompt", map[string]string{"text": "C"}, `{"entities":["Paris"]}`}})
	defer done()
	assertValue(t, ContextEntityRecall(llm), goeval.Sample{Reference: "R", RetrievedContexts: []string{"C"}}, 1/(2+1e-8))
	summary, finished := scripted(t, []step{{"summarization.ExtractKeyphrasePrompt", map[string]string{"text": "abcdefghij"}, `{"keyphrases":["fact"]}`}, {"summarization.GenerateQuestionsPrompt", map[string]any{"text": "abcdefghij", "keyphrases": []string{"fact"}}, `{"questions":["one?","two?"]}`}, {"summarization.GenerateAnswersPrompt", map[string]any{"summary": "abcde", "questions": []string{"one?", "two?"}}, `{"answers":["1","0"]}`}})
	defer finished()
	assertValue(t, SummarizationScore(summary, SummaryOptions{}), goeval.Sample{Response: "abcde", ReferenceContexts: []string{"abcdefghij"}}, 0.5)
}
func TestSQLRubricsAndAspectContracts(t *testing.T) {
	llm, done := scripted(t, []step{{"sql_semantic_equivalence.EquivalencePrompt", map[string]string{"reference": "SELECT id FROM users", "response": "SELECT users.id FROM users", "database_schema": "users(id)"}, `{"equivalence":true,"response_query_explaination":"same column","reference_query_explaination":"same column"}`}})
	defer done()
	assertValue(t, LLMSQLEquivalence(llm), goeval.Sample{Response: "SELECT users.id FROM users", Reference: "SELECT id FROM users", ReferenceContexts: []string{"users(id)"}}, 1)
	rubric, finished := scripted(t, []step{{"domain_specific_rubrics.SingleTurnPrompt", nil, `{"score":4,"feedback":"accurate"}`}})
	defer finished()
	assertValue(t, RubricsScore(rubric, nil), goeval.Sample{Response: "answer"}, 4)
	var calls atomic.Int64
	critic := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		calls.Add(1)
		if !strings.Contains(p.Instruction, "Criteria Definition: Is it correct?") {
			t.Error(p.Instruction)
		}
		return `{"verdict":1,"reason":"yes"}`, nil
	})
	assertValue(t, AspectCritic(critic, "correctness", "Is it correct?", 3), goeval.Sample{}, 1)
	if calls.Load() != 1 {
		t.Fatal("changed pinned upstream's single-judgment path")
	}
}
func TestRetrievalAndEmbeddingEdgeCases(t *testing.T) {
	s := goeval.Sample{RetrievedContextIDs: []string{"a", "a", "b"}, ReferenceContextIDs: []string{"a", "c"}}
	assertValue(t, IDBasedContextPrecision(), s, 0.5)
	assertValue(t, IDBasedContextRecall(), s, 0.5)
	s = goeval.Sample{RetrievedContexts: []string{"Paris", "nonsense", "Rome"}, ReferenceContexts: []string{"Paris", "Rome"}}
	assertValue(t, NonLLMContextPrecisionWithReference(ContextMatchOptions{}), s, AveragePrecision([]bool{true, false, true}))
	assertValue(t, NonLLMContextRecall(ContextMatchOptions{}), s, 1)
	for _, pair := range [][][]float64{{{}, {}}, {{1}, {1, 2}}, {{0, 0}, {1, 0}}, {{math.NaN()}, {1}}} {
		if _, err := Cosine(pair[0], pair[1]); err == nil {
			t.Fatalf("accepted invalid vectors %v", pair)
		}
	}
	if got, err := Cosine([]float64{1e300, 0}, []float64{1e300, 0}); err != nil || got != 1 {
		t.Fatalf("large cosine %g %v", got, err)
	}
	zero := 0.0
	v, err := factualValue(1, 1, 2, FactualOptions{Beta: &zero})
	if err != nil || v != 0.5 {
		t.Fatalf("beta zero: %g %v", v, err)
	}
	if _, err := IDBasedContextPrecision().Score(context.Background(), goeval.Sample{RetrievedContextIDs: []string{}, ReferenceContextIDs: []string{}}); !errors.Is(err, ErrUndefined) {
		t.Fatal(err)
	}
}
func TestCustomJudgesSupportPythonInputsAndTemplates(t *testing.T) {
	llm := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		if p.Instruction != "Check summary: Paris is the capital." {
			t.Errorf("unformatted prompt %q", p.Instruction)
		}
		return `{"value":"accurate","reason":"matches"}`, nil
	})
	d := DiscreteMetric(llm, "summary_accuracy", "Check summary: {summary}", []string{"accurate", "inaccurate"})
	out, err := d.ScoreInput(context.Background(), map[string]any{"summary": "Paris is the capital."})
	if err != nil || out.Value != "accurate" {
		t.Fatalf("%+v %v", out, err)
	}
	if _, err := d.ScoreInput(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing prompt input accepted")
	}
	numeric := NumericMetric(goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		if p.Instruction != "Rate helpfulness" {
			t.Error(p.Instruction)
		}
		return `{"value":1,"reason":"good"}`, nil
	}), "quality", "Rate {aspect}", 0, 1)
	value, err := numeric.ScoreInput(context.Background(), map[string]any{"aspect": "helpfulness"})
	if err != nil || value.Value != 1 {
		t.Fatalf("%+v %v", value, err)
	}
	ranking := RankingMetric(goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) {
		return `{"value":["second","first"],"reason":"relevance"}`, nil
	}), "rank", "Rank {results}", 2)
	ranked, err := ranking.ScoreInput(context.Background(), map[string]any{"results": []string{"first", "second"}})
	if err != nil || ranked.Value[0] != "second" {
		t.Fatalf("%+v %v", ranked, err)
	}
}
