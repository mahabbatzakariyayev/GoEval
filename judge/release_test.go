package judge

import (
	"context"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func TestInvalidConfigurationsFailBeforeAnyRequests(t *testing.T) {
	calls := 0
	model := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) {
		calls++
		return `{"statements":[]}`, nil
	})
	invalid := []goeval.Metric{
		goeval.MetricFunc{MetricName: "nil_callback"},
		FaithfulnessMetric(nil, Options{}),
		PromptAlignmentMetric(model, nil, Options{}),
		PromptAlignmentMetric(model, []string{" "}, Options{}),
		SummarizationMetric(model, SummarizationOptions{N: -1}),
		SummarizationMetric(model, SummarizationOptions{AssessmentQuestions: []string{""}}),
		ToolCorrectnessMetric(model, ToolCorrectnessOptions{EvaluationParams: []ToolCallParam{"bad"}}),
		ToolCorrectnessMetric(nil, ToolCorrectnessOptions{AvailableTools: []ToolCall{{Name: "search"}}}),
		scorers.NumericMetric(model, "invalid_numeric", "grade", 1, 0),
	}
	for _, metric := range invalid {
		t.Run(metric.Name(), func(t *testing.T) {
			_, err := goeval.Evaluate(context.Background(), Dataset{allFields()}.Samples(), []goeval.Metric{
				AnswerRelevancyMetric(model, Options{IncludeReason: Bool(false)}), metric,
			}, goeval.RunConfig{})
			if err == nil || calls != 0 {
				t.Fatal("invalid configuration reached scoring", err, calls)
			}
		})
	}
	c := allFields()
	c.ToolsCalled = []ToolCall{{Name: "search"}}
	if _, err := goeval.Evaluate(context.Background(), Dataset{c}.Samples(), []goeval.Metric{
		AnswerRelevancyMetric(model, Options{}), ArgumentCorrectnessMetric(nil, Options{}),
	}, goeval.RunConfig{}); err == nil || calls != 0 {
		t.Fatal("tool judge needs a model", err, calls)
	}
}

func TestUpdatedToolDataOverridesSavedCase(t *testing.T) {
	original := Case{
		ToolsCalled: []ToolCall{{Name: "stale"}}, ExpectedTools: []ToolCall{{Name: "stale"}},
	}
	s := original.ToSample()
	s.Messages = []goeval.Message{{Role: "assistant", ToolCalls: []goeval.ToolCall{{Name: "search", Arguments: map[string]any{"q": "Paris"}}}}}
	s.ReferenceToolCalls = []goeval.ToolCall{{Name: "search"}}
	c, err := FromSample(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ToolsCalled) != 1 || c.ToolsCalled[0].Name != "search" || c.ExpectedTools[0].Name != "search" {
		t.Fatal("stale tool calls survived conversion", c)
	}
	if original.ToolsCalled[0].Name != "stale" {
		t.Fatal("original case mutated")
	}
	s.Messages, s.ReferenceToolCalls = []goeval.Message{}, []goeval.ToolCall{}
	c, err = FromSample(s)
	if err != nil || len(c.ToolsCalled) != 0 || len(c.ExpectedTools) != 0 {
		t.Fatal("empty updates retained stale calls", c, err)
	}
}

func TestPartialAndUnknownModelJudgmentsAreRejected(t *testing.T) {
	for _, output := range []string{
		`{"verdicts":[]}`,
		`{"verdicts":[{"verdict":"yes"},{"verdict":"unknown"}]}`,
	} {
		m := PromptAlignmentMetric(scripted(t, stage{nil, output}), []string{"be accurate", "be brief"}, Options{IncludeReason: Bool(false)})
		if _, err := m.Measure(context.Background(), allFields()); err == nil {
			t.Fatal("incomplete instruction judgments passed")
		}
	}
	m := HallucinationMetric(scripted(t, stage{nil, `{"verdicts":[]}`}), Options{IncludeReason: Bool(false)})
	if _, err := m.Measure(context.Background(), allFields()); err == nil {
		t.Fatal("missing source judgments passed")
	}
	m = ContextualPrecisionMetric(scripted(t, stage{nil, `{"verdicts":[{"verdict":"yes"}]}`}), Options{IncludeReason: Bool(false)})
	if _, err := m.Measure(context.Background(), allFields()); err == nil {
		t.Fatal("missing retrieval judgments passed")
	}
	m = AnswerRelevancyMetric(scripted(t, stage{nil, `{"statements":[null]}`}), Options{IncludeReason: Bool(false)})
	if _, err := m.Measure(context.Background(), allFields()); err == nil || !strings.Contains(err.Error(), "nonempty") {
		t.Fatal("null statement accepted", err)
	}
}
