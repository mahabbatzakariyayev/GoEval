package judge

import (
	"context"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestDAGTaskAndBinaryBranch(t *testing.T) {
	for _, verdict := range []bool{true, false} {
		t.Run(goeval.PythonString(verdict), func(t *testing.T) {
			response := `{"verdict":false,"reason":"Missing section."}`
			want := 0.0
			if verdict {
				response = `{"verdict":true,"reason":"Both sections present."}`
				want = 1
			}
			llm := scripted(t,
				stage{map[string]any{"text": "Actual Output:\nanswer\n", "instructions": "Extract sections."}, `{"output":["intro","conclusion"]}`},
				stage{map[string]any{"text": "Sections:\n['intro', 'conclusion']\n\n"}, response},
				stage{map[string]any{"score": want}, `{"reason":"Decision path complete."}`})
			root := &TaskNode{Instructions: "Extract sections.", OutputLabel: "Sections", EvaluationParams: []SingleTurnParam{ActualOutput}, Children: []Node{&BinaryJudgementNode{Criteria: "Are both sections present?", Children: []*VerdictNode{{Verdict: true, Score: Int(10)}, {Verdict: false, Score: Int(0)}}}}}
			m, err := DAGMetric(llm, DAGOptions{Name: "structure", RootNodes: []Node{root}})
			if err != nil {
				t.Fatal(err)
			}
			root.Instructions = "Mutated after construction" // The graph owns a copy.
			measure(t, m, want)
		})
	}
}

func TestDAGExclusiveBranchesRejoin(t *testing.T) {
	for _, verdict := range []string{"true", "false"} {
		t.Run(verdict, func(t *testing.T) {
			shared := &TaskNode{Instructions: "Continue.", OutputLabel: "Continuation", EvaluationParams: []SingleTurnParam{ActualOutput}, Children: []Node{&NonBinaryJudgementNode{Criteria: "Choose quality.", Children: []*VerdictNode{{Verdict: "good", Score: Int(9)}, {Verdict: "bad", Score: Int(1)}}}}}
			root := &BinaryJudgementNode{Criteria: "First branch.", EvaluationParams: []SingleTurnParam{Input}, Children: []*VerdictNode{{Verdict: true, Child: shared}, {Verdict: false, Child: shared}}}
			m, err := DAGMetric(scripted(t,
				stage{nil, `{"verdict":` + verdict + `,"reason":"Chosen."}`},
				stage{map[string]any{"instructions": "Continue."}, `{"output":"continued"}`},
				stage{map[string]any{"text": "Continuation:\ncontinued\n", "options": []string{"good", "bad"}}, `{"verdict":"good","reason":"Good."}`}), DAGOptions{Options: Options{IncludeReason: Bool(false)}, Name: "join", RootNodes: []Node{root}})
			if err != nil {
				t.Fatal(err)
			}
			measure(t, m, .9)
		})
	}
}

func TestDAGMultipleTaskParentsJoinOnce(t *testing.T) {
	join := &BinaryJudgementNode{Criteria: "Both outputs available?", Children: []*VerdictNode{{Verdict: true, Score: Int(10)}, {Verdict: false, Score: Int(0)}}}
	root1 := &TaskNode{Instructions: "Extract one.", OutputLabel: "First", EvaluationParams: []SingleTurnParam{Input}, Children: []Node{join}}
	root2 := &TaskNode{Instructions: "Extract two.", OutputLabel: "Second", EvaluationParams: []SingleTurnParam{ActualOutput}, Children: []Node{join}}
	m, err := DAGMetric(scripted(t,
		stage{map[string]any{"instructions": "Extract one."}, `{"output":"one"}`},
		stage{map[string]any{"instructions": "Extract two."}, `{"output":"two"}`},
		stage{map[string]any{"text": "First:\none\n\nSecond:\ntwo\n\n"}, `{"verdict":true,"reason":"Both available."}`}), DAGOptions{Options: Options{IncludeReason: Bool(false)}, Name: "parents", RootNodes: []Node{root1, root2}})
	if err != nil {
		t.Fatal(err)
	}
	measure(t, m, 1)
}

func TestDAGChildMetricAndStrictThreshold(t *testing.T) {
	root := &BinaryJudgementNode{Criteria: "Branch.", EvaluationParams: []SingleTurnParam{ActualOutput}, Children: []*VerdictNode{{Verdict: true, Metric: ExactMatchMetric(Options{})}, {Verdict: false, Score: Int(0)}}}
	m, err := DAGMetric(scripted(t, stage{nil, `{"verdict":true,"reason":"Compare reference."}`}), DAGOptions{Name: "child", RootNodes: []Node{root}})
	if err != nil {
		t.Fatal(err)
	}
	c := Case{ActualOutput: "answer", ExpectedOutput: "answer"}
	r, err := m.Measure(context.Background(), c)
	if err != nil || r.Score != 1 {
		t.Fatal(r, err)
	}
	// Judge DAG strict mode raises the threshold but retains the leaf score.
	strict, err := DAGMetric(nil, DAGOptions{Options: Options{StrictMode: true, IncludeReason: Bool(false)}, Name: "strict", RootNodes: []Node{&VerdictNode{Verdict: true, Score: Int(8)}}})
	if err != nil {
		t.Fatal(err)
	}
	r = measure(t, strict, .8)
	if r.Threshold != 1 || r.Success {
		t.Fatal(r)
	}
}

func TestDAGValidation(t *testing.T) {
	cycle := &TaskNode{}
	cycle.Children = []Node{cycle}
	nested := &TaskNode{}
	for _, roots := range [][]Node{
		nil, {nil}, {cycle},
		{&VerdictNode{Verdict: true}},
		{&VerdictNode{Verdict: true, Score: Int(11)}},
		{&VerdictNode{Verdict: true, Score: Int(0), Metric: ExactMatchMetric(Options{})}},
		{&BinaryJudgementNode{Children: []*VerdictNode{{Verdict: true, Score: Int(1)}}}},
		{&BinaryJudgementNode{Children: []*VerdictNode{{Verdict: true, Score: Int(1)}, {Verdict: true, Score: Int(2)}}}},
		{&NonBinaryJudgementNode{Children: []*VerdictNode{{Verdict: "same", Score: Int(1)}, {Verdict: "same", Score: Int(2)}}}},
		{&TaskNode{Children: []Node{&VerdictNode{Verdict: true, Score: Int(10)}}}},
		{&TaskNode{Children: []Node{nested}}, nested},
	} {
		if _, err := DAGMetric(nil, DAGOptions{Name: "bad", RootNodes: roots}); err == nil {
			t.Fatal("invalid graph accepted", roots)
		}
	}
}

func TestDAGConcurrentRows(t *testing.T) {
	model := goeval.LLMFunc(func(_ context.Context, _ goeval.Prompt) (string, error) {
		return `{"verdict":true,"reason":"Good."}`, nil
	})
	m, err := DAGMetric(model, DAGOptions{Options: Options{IncludeReason: Bool(false)}, Name: "concurrent", RootNodes: []Node{&BinaryJudgementNode{Criteria: "Good?", EvaluationParams: []SingleTurnParam{ActualOutput}, Children: []*VerdictNode{{Verdict: true, Score: Int(10)}, {Verdict: false, Score: Int(0)}}}}})
	if err != nil {
		t.Fatal(err)
	}
	d := make(Dataset, 40)
	for i := range d {
		d[i] = allFields()
	}
	r, err := Evaluate(context.Background(), d, []Metric{m}, goeval.RunConfig{MaxWorkers: 8})
	if err != nil || r.Means[m.Name()] != 1 {
		t.Fatal(r, err)
	}
}
