package scorers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestPythonParity(t *testing.T) {
	data, err := os.ReadFile("testdata/reference_scores.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Revision string `json:"upstream_revision"`
		Scores   []struct {
			Metric, Response, Reference, Type, Mode, Distance string
			Expected                                          *float64
		} `json:"scores"`
		Precision []struct {
			Verdicts []int
			Expected *float64
		} `json:"average_precision"`
		Porter []struct{ Word, Expected string } `json:"porter"`
		Nvidia []struct {
			Metric, Text string
			Expected     *float64
		} `json:"nvidia"`
		Tools []struct {
			Metric    string
			Flexible  bool
			Actual    []goeval.ToolCall
			Reference []goeval.ToolCall
			Expected  *float64
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if fixtures.Revision != "298b68274234c060deacab3cf5fb52aa3a20e885" {
		t.Fatal("unexpected upstream revision")
	}
	for i, tc := range fixtures.Scores {
		t.Run(fmt.Sprintf("%s/%d", tc.Metric, i), func(t *testing.T) {
			var m goeval.Metric
			switch tc.Metric {
			case "exact_match":
				m = ExactMatch()
			case "string_present":
				m = StringPresence()
			case "non_llm_string_similarity":
				m = NonLLMStringSimilarity(Distance(tc.Distance))
			case "bleu_score":
				m = BleuScore(BLEUOptions{})
			case "chrf_score":
				m = ChrfScore(ChrFOptions{})
			case "rouge_score":
				m = RougeScore(RougeOptions{Type: tc.Type, Mode: tc.Mode})
			default:
				t.Fatal(tc.Metric)
			}
			score, err := m.Score(context.Background(), goeval.Sample{Response: tc.Response, Reference: tc.Reference})
			if tc.Expected == nil {
				if err == nil {
					t.Fatal("expected undefined score")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(score.Value-*tc.Expected) > 1e-12 {
				t.Fatalf("Go %.16g != Python %.16g for response=%q reference=%q", score.Value, *tc.Expected, tc.Response, tc.Reference)
			}
		})
	}
	for _, tc := range fixtures.Precision {
		v := make([]bool, len(tc.Verdicts))
		for i, x := range tc.Verdicts {
			v[i] = x != 0
		}
		got := AveragePrecision(v)
		if math.Abs(got-*tc.Expected) > 1e-14 {
			t.Fatalf("AP %v: Go %g Python %g", tc.Verdicts, got, *tc.Expected)
		}
	}
	for _, tc := range fixtures.Porter {
		if got := porter(tc.Word); got != tc.Expected {
			t.Errorf("Porter(%q): Go %q Python %q", tc.Word, got, tc.Expected)
		}
	}
	for _, tc := range fixtures.Nvidia {
		llm := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { return tc.Text, nil })
		got, err := nvRating(context.Background(), llm, "test", tc.Metric == "AnswerAccuracy")
		if tc.Expected == nil {
			if err == nil {
				t.Errorf("expected undefined parser result for %q", tc.Text)
			}
		} else if err != nil || got != *tc.Expected {
			t.Errorf("%s %q: Go %g %v Python %g", tc.Metric, tc.Text, got, err, *tc.Expected)
		}
	}
	for i, tc := range fixtures.Tools {
		t.Run(fmt.Sprintf("%s/%d", tc.Metric, i), func(t *testing.T) {
			m := ToolCallF1()
			if tc.Metric == "tool_call_accuracy" {
				m = ToolCallAccuracy(ToolAccuracyOptions{FlexibleOrder: tc.Flexible})
			}
			sample := goeval.Sample{Messages: []goeval.Message{{Role: "assistant", ToolCalls: tc.Actual}}, ReferenceToolCalls: tc.Reference}
			assertValue(t, m, sample, *tc.Expected)
		})
	}
}
