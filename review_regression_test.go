package goeval_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func TestNullFieldsRemainAbsentAndNumbersSurviveRoundTrip(t *testing.T) {
	d, err := goeval.ReadJSON(strings.NewReader(`[{"response":null,"reference":null,"metadata":{"id":9007199254740993}}]`))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		for _, field := range []goeval.Field{goeval.Response, goeval.Reference} {
			var missing *goeval.MissingFieldError
			if err := d[0].Validate(field); !errors.As(err, &missing) || missing.Field != field {
				t.Fatalf("field %s: %v", field, err)
			}
		}
		var b strings.Builder
		if err := d.WriteJSON(&b); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), "9007199254740993") {
			t.Fatal("metadata number lost precision", b.String())
		}
		d, err = goeval.ReadJSON(strings.NewReader(b.String()))
		if err != nil {
			t.Fatal(err)
		}
	}
	base := goeval.Sample{Response: "old", Reference: "reference"}
	removed := base.Without(goeval.Response)
	if err := removed.Validate(goeval.Response); err == nil {
		t.Fatal("Without retained a supplied response")
	}
	if err := removed.WithResponse("").Validate(goeval.Response); err != nil {
		t.Fatal(err)
	}
	if base.Response != "old" {
		t.Fatal("input mutated")
	}
}

func TestCSVAcceptsControlCharactersInScalarFields(t *testing.T) {
	value := "a\x00\x01\x7fb"
	d, err := goeval.ReadCSV(strings.NewReader("response,reference\n"+value+","+value+"\n"), nil)
	if err != nil || d[0].Response != value {
		t.Fatal(d, err)
	}
	var out strings.Builder
	if err := d.WriteCSV(&out); err != nil {
		t.Fatal(err)
	}
	copy, err := goeval.ReadCSV(strings.NewReader(out.String()), nil)
	if err != nil || copy[0].Reference != value {
		t.Fatal(copy, err)
	}
}

type invalidMetadataMetric struct{ fields bool }

func (m *invalidMetadataMetric) Name() string {
	if !m.fields {
		panic("name panic")
	}
	return "bad"
}
func (m *invalidMetadataMetric) RequiredFields() []goeval.Field { panic("fields panic") }
func (m *invalidMetadataMetric) Score(context.Context, goeval.Sample) (goeval.Score, error) {
	panic("must not score")
}

func TestEvaluationRejectsTypedNilAndMetadataPanics(t *testing.T) {
	var nilMetric *invalidMetadataMetric
	for _, m := range []goeval.Metric{nilMetric, &invalidMetadataMetric{}, &invalidMetadataMetric{fields: true}} {
		if _, err := goeval.Evaluate(context.Background(), nil, []goeval.Metric{m}, goeval.RunConfig{}); err == nil {
			t.Fatal("invalid metric accepted")
		}
	}
	if _, err := goeval.WithThreshold(nilMetric, 1); err == nil {
		t.Fatal("typed nil gate accepted")
	}
}

func TestLargeFiniteScoresHaveFiniteMeans(t *testing.T) {
	m := goeval.MetricFunc{MetricName: "large", Evaluate: func(_ context.Context, s goeval.Sample) (goeval.Score, error) {
		value := math.MaxFloat64
		if s.Response == "negative" {
			value = -value
		}
		return goeval.Score{Value: value}, nil
	}}
	for _, d := range []goeval.Dataset{{{Response: "positive"}, {Response: "negative"}}, {{Response: "positive"}, {Response: "positive"}}, {{Response: "negative"}, {Response: "positive"}}} {
		r, err := goeval.Evaluate(context.Background(), d, []goeval.Metric{m}, goeval.RunConfig{})
		if err != nil || math.IsInf(r.Means["large"], 0) || math.IsNaN(r.Means["large"]) {
			t.Fatal(r, err)
		}
		if _, err := json.Marshal(r); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecutionErrorsRemainInspectable(t *testing.T) {
	want := errors.New("provider unavailable")
	m := goeval.MetricFunc{MetricName: "failure", Evaluate: func(context.Context, goeval.Sample) (goeval.Score, error) { return goeval.Score{}, want }}
	r, err := goeval.Evaluate(context.Background(), goeval.Dataset{{}}, []goeval.Metric{m}, goeval.RunConfig{})
	if err != nil || !errors.Is(r.Err(), want) || !errors.Is(r.Rows[0].Err(), want) {
		t.Fatal(r, err)
	}
	if _, err := goeval.Check(context.Background(), goeval.Sample{}, []goeval.Metric{m}, goeval.RunConfig{}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	gate, _ := goeval.WithThreshold(scorers.ExactMatch(), 1)
	_, err = goeval.Check(context.Background(), goeval.Sample{Response: "a", Reference: "b"}, []goeval.Metric{gate}, goeval.RunConfig{})
	var threshold *goeval.ThresholdError
	if !errors.As(err, &threshold) || threshold.Score != 0 || threshold.Threshold != 1 {
		t.Fatal(err)
	}
	var calls atomic.Int32
	panics := goeval.MetricFunc{MetricName: "panic", Evaluate: func(context.Context, goeval.Sample) (goeval.Score, error) { calls.Add(1); panic(want) }}
	r, err = goeval.Evaluate(context.Background(), goeval.Dataset{{}}, []goeval.Metric{panics}, goeval.RunConfig{MaxRetries: 5})
	var recovered *goeval.PanicError
	if err != nil || calls.Load() != 1 || !errors.As(r.Err(), &recovered) || !errors.Is(r.Err(), want) {
		t.Fatal(r, err, calls.Load())
	}
}

func TestInvalidModelInputsReturnErrors(t *testing.T) {
	var nilModel goeval.LLMFunc
	var out map[string]any
	if err := goeval.GenerateJSON(context.Background(), nilModel, goeval.Prompt{}, &out); err == nil {
		t.Fatal("typed nil model accepted")
	}
	model := goeval.LLMFunc(func(context.Context, goeval.Prompt) (string, error) { return "null", nil })
	if err := goeval.GenerateJSON(context.Background(), model, goeval.Prompt{}, &out); err == nil {
		t.Fatal("null result accepted")
	}
	//lint:ignore SA1012 Exercise nil-context validation.
	if err := goeval.GenerateJSON(nil, model, goeval.Prompt{}, &out); err == nil {
		t.Fatal("nil context accepted")
	}
	bad := math.NaN()
	m := goeval.MetricFunc{MetricName: "invalid gate", Evaluate: func(context.Context, goeval.Sample) (goeval.Score, error) {
		return goeval.Score{Value: 1, Threshold: &bad}, nil
	}}
	r, err := goeval.Evaluate(context.Background(), goeval.Dataset{{}}, []goeval.Metric{m}, goeval.RunConfig{})
	if err != nil || r.Err() == nil || len(r.Rows[0].Scores) != 0 {
		t.Fatal(r, err)
	}
}

func FuzzSampleJSONRoundTrip(f *testing.F) {
	for _, seed := range []string{`{}`, `null`, `{"response":null}`, `{"response":"","retrieved_contexts":[]}`, `{"user_input":[{"type":"ai","tool_calls":[{"name":"find","args":{"id":9007199254740993}}]}]}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64*1024 {
			t.Skip()
		}
		var row goeval.Sample
		if json.Unmarshal([]byte(raw), &row) != nil {
			return
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		var copy goeval.Sample
		if err := json.Unmarshal(encoded, &copy); err != nil {
			t.Fatal(err)
		}
		for _, field := range []goeval.Field{goeval.UserInput, goeval.Response, goeval.Reference, goeval.RetrievedContexts, goeval.Messages, goeval.ReferenceToolCalls} {
			if (row.Validate(field) == nil) != (copy.Validate(field) == nil) {
				t.Fatalf("presence changed for %s: %s => %s", field, raw, encoded)
			}
		}
	})
}

func BenchmarkEvaluateOffline(b *testing.B) {
	d := make(goeval.Dataset, 100)
	for i := range d {
		d[i] = goeval.Sample{Response: "Paris", Reference: "Paris"}
	}
	ms := []goeval.Metric{scorers.ExactMatch()}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := goeval.Evaluate(context.Background(), d, ms, goeval.RunConfig{MaxWorkers: 4}); err != nil {
			b.Fatal(err)
		}
	}
}
