package scorers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/internal/prompts"
)

func nvTemplate(key, name string, values map[string]string) string {
	t := prompts.Template("nv_metrics."+key, name)
	// Replace the original placeholders in one pass; inserted answers must not
	// themselves be interpreted as templates.
	pairs := []string{}
	for k, v := range values {
		pairs = append(pairs, "{"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(t)
}
func nvRating(ctx context.Context, llm goeval.LLM, text string, accuracy bool) (float64, error) {
	if check.Nil(llm) {
		return 0, fmt.Errorf("metrics: LLM is required")
	}
	temperature := 0.1
	for attempt := 0; attempt < 5; attempt++ {
		raw, err := llm.Generate(ctx, goeval.Prompt{Text: text, Temperature: &temperature})
		if err != nil {
			return 0, err
		}
		if accuracy {
			for i := 0; i < 5; i++ {
				if strings.Contains(raw, strconv.Itoa(i)) {
					return float64(i) / 4, nil
				}
			}
		} else {
			for _, i := range []int{2, 1, 0} {
				if strings.Contains(raw, strconv.Itoa(i)) {
					return float64(i) / 2, nil
				}
			}
		}
	}
	return 0, ErrUndefined
}
func nvPair(ctx context.Context, llm goeval.LLM, key, prefix string, first, second map[string]string, accuracy bool) (goeval.Score, error) {
	a, err := nvRating(ctx, llm, nvTemplate(key, prefix+"1", first), accuracy)
	if err != nil && !errors.Is(err, ErrUndefined) {
		return goeval.Score{}, err
	}
	firstUndefined := errors.Is(err, ErrUndefined)
	b, err := nvRating(ctx, llm, nvTemplate(key, prefix+"2", second), accuracy)
	if err != nil && !errors.Is(err, ErrUndefined) {
		return goeval.Score{}, err
	}
	// Python max(nan, valid) remains nan, while max(valid, nan) returns valid.
	// Preserve that asymmetric legacy fallback rather than inventing a mean.
	if firstUndefined {
		return goeval.Score{}, ErrUndefined
	}
	if errors.Is(err, ErrUndefined) {
		return goeval.Score{Value: a}, nil
	}
	return goeval.Score{Value: (a + b) / 2}, nil
}
func AnswerAccuracy(llm goeval.LLM) goeval.Metric {
	return goeval.MetricFunc{MetricName: "nv_accuracy", Fields: []goeval.Field{goeval.UserInput, goeval.Response, goeval.Reference}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		a := map[string]string{"query": s.UserInput, "answer0": "User Answer", "answer1": "Reference Answer", "sentence_inference": s.Response, "sentence_true": s.Reference}
		b := map[string]string{"query": s.UserInput, "answer0": "Reference Answer", "answer1": "User Answer", "sentence_inference": s.Reference, "sentence_true": s.Response}
		return nvPair(ctx, llm, "AnswerAccuracy", "template_accuracy", a, b, true)
	}}
}
func ContextRelevance(llm goeval.LLM) goeval.Metric {
	return goeval.MetricFunc{MetricName: "nv_context_relevance", Fields: []goeval.Field{goeval.UserInput, goeval.RetrievedContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		query, passage := strings.TrimSpace(s.UserInput), strings.TrimSpace(strings.Join(s.RetrievedContexts, "\n"))
		if query == "" || passage == "" || strings.Contains(query, passage) {
			return goeval.Score{Value: 0}, nil
		}
		v := map[string]string{"query": s.UserInput, "context": strings.Join(s.RetrievedContexts, "\n")}
		return nvPair(ctx, llm, "ContextRelevance", "template_relevance", v, v, false)
	}}
}
func ResponseGroundedness(llm goeval.LLM) goeval.Metric {
	return goeval.MetricFunc{MetricName: "nv_response_groundedness", Fields: []goeval.Field{goeval.Response, goeval.RetrievedContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		response, passage := strings.TrimSpace(s.Response), strings.TrimSpace(strings.Join(s.RetrievedContexts, "\n"))
		if response == "" || passage == "" {
			return goeval.Score{Value: 0}, nil
		}
		if strings.Contains(passage, response) {
			return goeval.Score{Value: 1}, nil
		}
		v := map[string]string{"response": s.Response, "context": strings.Join(s.RetrievedContexts, "\n")}
		return nvPair(ctx, llm, "ResponseGroundedness", "template_groundedness", v, v, false)
	}}
}
