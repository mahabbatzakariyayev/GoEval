package goeval_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mahabbatzakariyayev/GoEval"
)

func TestSavedReportsPreserveQualityGates(t *testing.T) {
	for _, output := range []string{"Paris", "Rome"} {
		t.Run(output, func(t *testing.T) {
			e, err := goeval.New()
			if err != nil {
				t.Fatal(err)
			}
			original, err := e.Check(context.Background(), goeval.Case{Output: output, Expected: "Paris"})
			if original == nil {
				t.Fatal(err)
			}
			data, marshalErr := json.Marshal(original)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var saved goeval.Report
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Passed() != original.Passed() || (saved.Err() == nil) != (err == nil) {
				t.Fatalf("quality gate changed after saving: %s", data)
			}
			if saved.Means["exact_match"] != original.Means["exact_match"] {
				t.Fatal("saved mean changed")
			}
		})
	}
}

func TestSavedReportsPreserveFailuresAndLargeDetails(t *testing.T) {
	want := errors.New("provider unavailable")
	m := goeval.MetricFunc{MetricName: "custom", Evaluate: func(_ context.Context, s goeval.Sample) (goeval.Score, error) {
		if s.Response == "fail" {
			return goeval.Score{}, want
		}
		return goeval.Score{Value: 1, Details: map[string]any{"id": json.Number("9007199254740993")}}, nil
	}}
	gate, err := goeval.WithThreshold(m, 1)
	if err != nil {
		t.Fatal(err)
	}
	e, err := goeval.New(goeval.WithMetrics(gate))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Evaluate(context.Background(), []goeval.Case{{Output: "ok"}, {Output: "fail"}})
	if !errors.Is(err, want) || !errors.Is(r.Err(), want) {
		t.Fatal(err, r.Err())
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var saved goeval.Report
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Passed() || saved.Err() == nil || !strings.Contains(saved.Err().Error(), want.Error()) {
		t.Fatal("saved report lost execution failure", saved.Err())
	}
	if got := saved.Rows[0].Scores["custom"].Details["id"]; got != json.Number("9007199254740993") {
		t.Fatalf("large number changed: %v", got)
	}
}

func TestSavedReportsCannotPassIncompleteOrCancelledRuns(t *testing.T) {
	for _, data := range []string{
		`{"rows":[{"scores":{"one":{"value":1,"success":true,"threshold":1}}}],"metrics":["one","two"]}`,
		`{"rows":[{"scores":{"one":{"value":1,"success":true,"threshold":1}}}],"metrics":["one"],"error":"context canceled"}`,
		`{"rows":[{"scores":{"one":{"value":1,"success":true,"threshold":1}}}]}`,
		`null`,
	} {
		var r goeval.Report
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			t.Fatal(err)
		}
		if r.Passed() {
			t.Fatalf("incomplete report passed: %s", data)
		}
	}
	var r goeval.Report
	if err := json.Unmarshal([]byte(`{"metrics":["one","one"]}`), &r); err == nil {
		t.Fatal("duplicate metrics accepted")
	}
}

func TestCaseMetadataReachesCustomScorers(t *testing.T) {
	metadata := map[string]any{"customer": "team-a"}
	m := goeval.MetricFunc{MetricName: "metadata", Evaluate: func(_ context.Context, s goeval.Sample) (goeval.Score, error) {
		if s.Metadata["customer"] != "team-a" {
			return goeval.Score{}, errors.New("customer metadata missing")
		}
		return goeval.Score{Value: 1}, nil
	}}
	gate, err := goeval.WithThreshold(m, 1)
	if err != nil {
		t.Fatal(err)
	}
	e, err := goeval.New(goeval.WithMetrics(gate))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Evaluate(context.Background(), []goeval.Case{{Metadata: metadata}})
	if err != nil || !r.Passed() {
		t.Fatal(r, err)
	}
	if len(metadata) != 1 {
		t.Fatal("conversion modified caller metadata")
	}
}
