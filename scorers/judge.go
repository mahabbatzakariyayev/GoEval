package scorers

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/internal/prompts"
)

// AspectCritic preserves the pinned upstream scoring path: one generated verdict.
// Upstream stores strictness but currently passes one response to its reducer.
func AspectCritic(llm goeval.LLM, name, definition string, strictness int) goeval.Metric {
	return goeval.MetricFunc{MetricName: name, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if strictness < 0 || strings.TrimSpace(definition) == "" {
			return goeval.Score{}, fmt.Errorf("metrics: invalid aspect configuration")
		}
		return binaryJudge(ctx, llm, "Evaluate the Input based on the criterial defined. Use only 'Yes' (1) and 'No' (0) as verdict.\nCriteria Definition: "+definition, judgeInput(s))
	}}
}
func judgeInput(s goeval.Sample) map[string]any {
	if s.Messages != nil {
		return map[string]any{"user_input": s.PrettyRepr(), "reference": s.Reference}
	}
	return map[string]any{"user_input": s.UserInput, "response": s.Response, "reference": s.Reference, "retrieved_contexts": s.RetrievedContexts, "reference_contexts": s.ReferenceContexts}
}

// SimpleCriteriaScore follows GoEval's integer score/reason output. Optional
// limits are a Go extension; without limits any integer score is accepted.
func SimpleCriteriaScore(llm goeval.LLM, name, definition string, limits ...int) goeval.Metric {
	limits = append([]int(nil), limits...)
	return goeval.MetricFunc{MetricName: name, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if len(limits) != 0 && len(limits) != 2 {
			return goeval.Score{}, fmt.Errorf("metrics: supply either no score limits or min and max")
		}
		var out struct {
			Score  *int   `json:"score"`
			Reason string `json:"reason"`
		}
		err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: "Evaluate the input based on the criteria defined.\nCriteria Definition: " + definition, Input: judgeInput(s), Schema: objectSchema(map[string]any{"score": map[string]any{"type": "integer"}, "reason": stringSchema()})}, &out)
		if err != nil {
			return goeval.Score{}, err
		}
		if out.Score == nil {
			return goeval.Score{}, fmt.Errorf("metrics: missing criterion score")
		}
		if len(limits) == 2 && (*out.Score < limits[0] || *out.Score > limits[1]) {
			return goeval.Score{}, fmt.Errorf("metrics: criterion score outside limits")
		}
		return goeval.Score{Value: float64(*out.Score), Reason: out.Reason}, nil
	}}
}

type Judgment[T any] struct {
	Value  T      `json:"value"`
	Reason string `json:"reason,omitempty"`
}

func rubricMetric(llm goeval.LLM, name string, rubrics map[string]string, instance bool) goeval.Metric {
	if rubrics == nil && !instance {
		rubrics = DefaultReferenceFreeRubrics()
	}
	copyRubrics := map[string]string{}
	for k, v := range rubrics {
		copyRubrics[k] = v
	}
	fields := []goeval.Field{}
	if instance {
		fields = append(fields, goeval.Rubrics)
	}
	return goeval.MetricFunc{MetricName: name, Fields: fields, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		r := copyRubrics
		if instance {
			r = s.Rubrics
		}
		if r == nil {
			return goeval.Score{}, fmt.Errorf("metrics: rubrics are required")
		}
		instruction := prompts.Instruction("domain_specific_rubrics.SingleTurnPrompt")
		input := judgeInput(s)
		if instance {
			instruction = prompts.Instruction("instance_specific_rubrics.SingleTurnPrompt")
			input["rubrics"] = r
			if s.Messages == nil {
				userInput := s.UserInput
				if s.RetrievedContexts != nil {
					userInput += " answer using context: " + strings.Join(s.RetrievedContexts, "\n")
				}
				input = map[string]any{"user_input": userInput, "response": s.Response, "reference": s.Reference, "retrieved_contexts": nil, "reference_contexts": nil, "rubrics": r}
			}
		} else {
			keys := make([]string, 0, len(r))
			for k := range r {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			lines := []string{}
			for _, k := range keys {
				lines = append(lines, k+": "+r[k])
			}
			instruction += "\n\nScoring Rubrics:\n" + strings.Join(lines, "\n") + "\n"
		}
		var out struct {
			Score    *int   `json:"score"`
			Feedback string `json:"feedback"`
		}
		err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: instruction, Input: input, Schema: objectSchema(map[string]any{"score": map[string]any{"type": "integer"}, "feedback": stringSchema()})}, &out)
		if err != nil {
			return goeval.Score{}, err
		}
		if out.Score == nil {
			return goeval.Score{}, fmt.Errorf("metrics: missing rubric score")
		}
		return goeval.Score{Value: float64(*out.Score), Reason: out.Feedback}, nil
	}}
}
func DomainSpecificRubrics(llm goeval.LLM, rubrics map[string]string) goeval.Metric {
	return rubricMetric(llm, "domain_specific_rubrics", rubrics, false)
}
func RubricsScore(llm goeval.LLM, rubrics map[string]string) goeval.Metric {
	return DomainSpecificRubrics(llm, rubrics)
}
func InstanceRubrics(llm goeval.LLM) goeval.Metric {
	return rubricMetric(llm, "instance_rubrics", nil, true)
}
func InstanceSpecificRubrics(llm goeval.LLM) goeval.Metric { return InstanceRubrics(llm) }
func DefaultReferenceFreeRubrics() map[string]string {
	return map[string]string{
		"score1_description": "The response is entirely incorrect and fails to address any aspect of the user input.",
		"score2_description": "The response contains partial accuracy but includes major errors or significant omissions that affect its relevance to the user input.",
		"score3_description": "The response is mostly accurate but lacks clarity, thoroughness, or minor details needed to fully address the user input.",
		"score4_description": "The response is accurate and clear, with only minor omissions or slight inaccuracies in addressing the user input.",
		"score5_description": "The response is completely accurate, clear, and thoroughly addresses the user input without any errors or omissions.",
	}
}

// LLMSQLEquivalence compares meaning using upstream's schema and prompt.
func LLMSQLEquivalence(llm goeval.LLM) goeval.Metric {
	return goeval.MetricFunc{MetricName: "llm_sql_equivalence_with_reference", Fields: []goeval.Field{goeval.Response, goeval.Reference, goeval.ReferenceContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		var out struct {
			Equivalence          *bool  `json:"equivalence"`
			ResponseExplanation  string `json:"response_query_explaination"`
			ReferenceExplanation string `json:"reference_query_explaination"`
		}
		err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("sql_semantic_equivalence.EquivalencePrompt"), Input: map[string]string{"response": s.Response, "reference": s.Reference, "database_schema": strings.Join(s.ReferenceContexts, "\n")}, Schema: objectSchema(map[string]any{"equivalence": map[string]any{"type": "boolean"}, "response_query_explaination": stringSchema(), "reference_query_explaination": stringSchema()})}, &out)
		if err != nil {
			return goeval.Score{}, err
		}
		if out.Equivalence == nil {
			return goeval.Score{}, fmt.Errorf("metrics: missing SQL equivalence")
		}
		v := 0.0
		if *out.Equivalence {
			v = 1
		}
		return goeval.Score{Value: v, Reason: out.ResponseExplanation + "; " + out.ReferenceExplanation}, nil
	}}
}
func SQLSemanticEquivalence(llm goeval.LLM) goeval.Metric { return LLMSQLEquivalence(llm) }

type SummaryOptions struct {
	DisableLengthPenalty bool
	Coefficient          *float64
}

func SummarizationScore(llm goeval.LLM, opts SummaryOptions) goeval.Metric {
	opts.Coefficient = check.CopyPtr(opts.Coefficient)
	return goeval.MetricFunc{MetricName: "summary_score", Fields: []goeval.Field{goeval.ReferenceContexts, goeval.Response}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		coeff := 0.5
		if opts.Coefficient != nil {
			coeff = *opts.Coefficient
		}
		if coeff < 0 || coeff > 1 || math.IsNaN(coeff) {
			return goeval.Score{}, fmt.Errorf("metrics: invalid summary coefficient")
		}
		text := strings.Join(s.ReferenceContexts, "\n")
		if text == "" {
			return goeval.Score{}, ErrUndefined
		}
		var keys struct {
			Keyphrases *[]string `json:"keyphrases"`
		}
		err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("summarization.ExtractKeyphrasePrompt"), Input: map[string]string{"text": text}, Schema: objectSchema(map[string]any{"keyphrases": arraySchema(stringSchema())})}, &keys)
		if err != nil {
			return goeval.Score{}, err
		}
		if keys.Keyphrases == nil {
			return goeval.Score{}, fmt.Errorf("metrics: missing keyphrases")
		}
		var questions struct {
			Questions *[]string `json:"questions"`
		}
		err = goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("summarization.GenerateQuestionsPrompt"), Input: map[string]any{"text": text, "keyphrases": *keys.Keyphrases}, Schema: objectSchema(map[string]any{"questions": arraySchema(stringSchema())})}, &questions)
		if err != nil {
			return goeval.Score{}, err
		}
		if questions.Questions == nil || len(*questions.Questions) == 0 {
			return goeval.Score{}, ErrUndefined
		}
		var answers struct {
			Answers []string `json:"answers"`
		}
		err = goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("summarization.GenerateAnswersPrompt"), Input: map[string]any{"summary": s.Response, "questions": *questions.Questions}, Schema: objectSchema(map[string]any{"answers": arraySchema(stringSchema())})}, &answers)
		if err != nil {
			return goeval.Score{}, err
		}
		if len(answers.Answers) != len(*questions.Questions) {
			return goeval.Score{}, fmt.Errorf("metrics: summary answer count mismatch")
		}
		hits := 0
		for _, a := range answers.Answers {
			if strings.ToLower(a) == "1" {
				hits++
			}
		}
		qa := float64(hits) / float64(len(answers.Answers))
		concise := 0.0
		if !opts.DisableLengthPenalty {
			concise = 1 - float64(min(len([]rune(s.Response)), len([]rune(text))))/(float64(len([]rune(text)))+1e-10)
		}
		return goeval.Score{Value: qa*(1-coeff) + concise*coeff}, nil
	}}
}

// PearsonCorrelation measures agreement of numeric judge predictions and labels.
func PearsonCorrelation(a, b []float64) (float64, error) {
	if len(a) != len(b) || len(a) < 2 {
		return 0, fmt.Errorf("metrics: correlation needs equally sized samples of length >=2")
	}
	sa, sb := 0.0, 0.0
	for i, x := range a {
		y := b[i]
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			return 0, fmt.Errorf("metrics: non-finite label")
		}
		sa = math.Max(sa, math.Abs(x))
		sb = math.Max(sb, math.Abs(y))
	}
	if sa == 0 || sb == 0 {
		return 0, ErrUndefined
	}
	ma, mb := 0.0, 0.0
	for i, x := range a {
		ma += (x/sa - ma) / float64(i+1)
		mb += (b[i]/sb - mb) / float64(i+1)
	}
	xy, xx, yy := 0.0, 0.0, 0.0
	for i, x := range a {
		dx, dy := x/sa-ma, b[i]/sb-mb
		xy += dx * dy
		xx += dx * dx
		yy += dy * dy
	}
	if xx == 0 || yy == 0 {
		return 0, ErrUndefined
	}
	return math.Max(-1, math.Min(1, xy/(math.Sqrt(xx)*math.Sqrt(yy)))), nil
}
func CohenKappa(a, b []string) (float64, error) {
	if len(a) != len(b) || len(a) == 0 {
		return 0, fmt.Errorf("metrics: kappa needs equally sized nonempty samples")
	}
	ac, bc := map[string]int{}, map[string]int{}
	hits := 0
	for i, x := range a {
		ac[x]++
		bc[b[i]]++
		if x == b[i] {
			hits++
		}
	}
	n := float64(len(a))
	expected := 0.0
	for label, count := range ac {
		expected += float64(count) / n * float64(bc[label]) / n
	}
	if expected == 1 {
		return 0, ErrUndefined
	}
	return (float64(hits)/n - expected) / (1 - expected), nil
}

// SortedMetricNames is useful for stable reporting of metric registries.
func SortedMetricNames(ms []goeval.Metric) []string {
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.Name()
	}
	sort.Strings(names)
	return names
}
