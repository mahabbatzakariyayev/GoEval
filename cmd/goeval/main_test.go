package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval"
)

func TestOfflineCLI(t *testing.T) {
	var out, stderr bytes.Buffer
	err := run(context.Background(), []string{"evaluate", "-format", "json", "-metrics", "exact_match,rouge_score"}, strings.NewReader(`[{"response":"Paris","reference":"Paris"}]`), &out, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	var result goeval.EvaluationResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Means["exact_match"] != 1 || result.Means["rouge_score"] != 1 {
		t.Fatal(result.Means)
	}
}
func TestCLIReportsUnknownMetric(t *testing.T) {
	var out, stderr bytes.Buffer
	if err := run(context.Background(), []string{"evaluate", "-format", "json", "-metrics", "typo"}, strings.NewReader(`[]`), &out, &stderr); err == nil {
		t.Fatal("unknown metric accepted")
	}
	if out.Len() != 0 {
		t.Fatal("wrote invalid result")
	}
}
func TestGenerateCLIWithMockProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"message":{"content":"{\"user_input\":\"Where is Paris?\",\"reference\":\"France\"}"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
	var out, stderr bytes.Buffer
	err := run(context.Background(), []string{"generate", "-model", "mock", "-base-url", srv.URL, "-size", "2"}, strings.NewReader(`[{"id":"doc","content":"Paris is in France."}]`), &out, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	d, err := goeval.ReadJSON(&out)
	if err != nil || len(d) != 2 || d[0].UserInput != "Where is Paris?" || len(d[0].ReferenceContexts) != 1 {
		t.Fatalf("%+v %v", d, err)
	}
}
