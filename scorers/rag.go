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

// ErrUndefined replaces Python NaN for empty claim sets or denominators. The
// evaluator records it as a row error and excludes it from the aggregate.
var ErrUndefined = goeval.ErrUndefined

func objectSchema(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for k := range properties {
		required = append(required, k)
	}
	sort.Strings(required)
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func stringSchema() map[string]any        { return map[string]any{"type": "string"} }
func arraySchema(item any) map[string]any { return map[string]any{"type": "array", "items": item} }
func binarySchema() map[string]any        { return map[string]any{"type": "integer", "enum": []int{0, 1}} }
func boolValue(v *int) (bool, error) {
	if v == nil || (*v != 0 && *v != 1) {
		return false, fmt.Errorf("metrics: expected binary verdict 0 or 1")
	}
	return *v == 1, nil
}

func decompose(ctx context.Context, llm goeval.LLM, question, text string) ([]string, error) {
	var out struct {
		Statements *[]string `json:"statements"`
	}
	err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{
		Instruction: prompts.Instruction("faithfulness.StatementGeneratorPrompt"),
		Input:       map[string]any{"question": question, "answer": text}, Schema: objectSchema(map[string]any{"statements": arraySchema(stringSchema())}),
	}, &out)
	if err != nil {
		return nil, err
	}
	if out.Statements == nil {
		return nil, fmt.Errorf("metrics: model omitted statements")
	}
	for _, s := range *out.Statements {
		if strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("metrics: empty generated statement")
		}
	}
	return *out.Statements, nil
}
func verify(ctx context.Context, llm goeval.LLM, premise string, claims []string) ([]bool, error) {
	if len(claims) == 0 {
		return []bool{}, nil
	}
	type item struct {
		Statement string `json:"statement"`
		Verdict   *int   `json:"verdict"`
		Reason    string `json:"reason"`
	}
	var out struct {
		Statements []item `json:"statements"`
	}
	err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{
		Instruction: prompts.Instruction("faithfulness.NLIStatementPrompt"),
		Input:       map[string]any{"context": premise, "statements": claims}, Schema: objectSchema(map[string]any{"statements": arraySchema(objectSchema(map[string]any{"statement": stringSchema(), "verdict": binarySchema(), "reason": stringSchema()}))}),
	}, &out)
	if err != nil {
		return nil, err
	}
	if len(out.Statements) != len(claims) {
		return nil, fmt.Errorf("metrics: expected %d verdicts, got %d", len(claims), len(out.Statements))
	}
	verdicts := make([]bool, len(claims))
	for i, item := range out.Statements {
		if item.Statement != claims[i] {
			return nil, fmt.Errorf("metrics: verdict statement %d does not match input", i)
		}
		v, err := boolValue(item.Verdict)
		if err != nil {
			return nil, err
		}
		verdicts[i] = v
	}
	return verdicts, nil
}
func trueCount(v []bool) int {
	n := 0
	for _, b := range v {
		if b {
			n++
		}
	}
	return n
}
func meanVerdicts(v []bool) (float64, error) {
	if len(v) == 0 {
		return 0, ErrUndefined
	}
	return float64(trueCount(v)) / float64(len(v)), nil
}

func Faithfulness(llm goeval.LLM) goeval.Metric {
	return goeval.MetricFunc{MetricName: "faithfulness", Fields: []goeval.Field{goeval.UserInput, goeval.Response, goeval.RetrievedContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		claims, err := decompose(ctx, llm, s.UserInput, s.Response)
		if err != nil {
			return goeval.Score{}, err
		}
		v, err := verify(ctx, llm, strings.Join(s.RetrievedContexts, "\n"), claims)
		if err != nil {
			return goeval.Score{}, err
		}
		score, err := meanVerdicts(v)
		return goeval.Score{Value: score, Reason: fmt.Sprintf("%d/%d claims supported", trueCount(v), len(v))}, err
	}}
}
func ContextRecall(llm goeval.LLM) goeval.Metric {
	return goeval.MetricFunc{MetricName: "context_recall", Fields: []goeval.Field{goeval.UserInput, goeval.Reference, goeval.RetrievedContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		var out struct {
			Classifications []struct {
				Statement  string `json:"statement"`
				Reason     string `json:"reason"`
				Attributed *int   `json:"attributed"`
			} `json:"classifications"`
		}
		err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("context_recall.ContextRecallClassificationPrompt"), Input: map[string]string{"question": s.UserInput, "context": strings.Join(s.RetrievedContexts, "\n"), "answer": s.Reference}, Schema: objectSchema(map[string]any{"classifications": arraySchema(objectSchema(map[string]any{"statement": stringSchema(), "reason": stringSchema(), "attributed": binarySchema()}))})}, &out)
		if err != nil {
			return goeval.Score{}, err
		}
		v := make([]bool, len(out.Classifications))
		for i, c := range out.Classifications {
			v[i], err = boolValue(c.Attributed)
			if err != nil {
				return goeval.Score{}, err
			}
		}
		score, err := meanVerdicts(v)
		return goeval.Score{Value: score}, err
	}}
}
func binaryJudge(ctx context.Context, llm goeval.LLM, instruction string, input any) (goeval.Score, error) {
	var out struct {
		Verdict *int   `json:"verdict"`
		Reason  string `json:"reason"`
	}
	err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: instruction, Input: input, Schema: objectSchema(map[string]any{"verdict": binarySchema(), "reason": stringSchema()})}, &out)
	if err != nil {
		return goeval.Score{}, err
	}
	v, err := boolValue(out.Verdict)
	score := 0.0
	if v {
		score = 1
	}
	return goeval.Score{Value: score, Reason: out.Reason}, err
}

// AveragePrecision measures rank-sensitive precision at relevant positions.
func AveragePrecision(relevant []bool) float64 {
	hits, total := 0, 0.0
	for i, ok := range relevant {
		if ok {
			hits++
			total += float64(hits) / float64(i+1)
		}
	}
	if hits == 0 {
		return 0
	}
	return total / (float64(hits) + 1e-10)
}
func contextPrecision(llm goeval.LLM, withReference bool) goeval.Metric {
	name, answerField := "context_precision", goeval.Reference
	if !withReference {
		name, answerField = "context_utilization", goeval.Response
	}
	return goeval.MetricFunc{MetricName: name, Fields: []goeval.Field{goeval.UserInput, answerField, goeval.RetrievedContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		answer := s.Reference
		if !withReference {
			answer = s.Response
		}
		relevant := make([]bool, len(s.RetrievedContexts))
		for i, passage := range s.RetrievedContexts {
			score, err := binaryJudge(ctx, llm, prompts.Instruction("context_precision.ContextPrecisionPrompt"), map[string]any{"question": s.UserInput, "answer": answer, "context": passage})
			if err != nil {
				return goeval.Score{}, err
			}
			relevant[i] = score.Value == 1
		}
		return goeval.Score{Value: AveragePrecision(relevant)}, nil
	}}
}
func ContextPrecision(llm goeval.LLM) goeval.Metric   { return contextPrecision(llm, true) }
func ContextUtilization(llm goeval.LLM) goeval.Metric { return contextPrecision(llm, false) }
func LLMContextPrecisionWithReference(llm goeval.LLM) goeval.Metric {
	return rename("llm_context_precision_with_reference", contextPrecision(llm, true))
}
func LLMContextPrecisionWithoutReference(llm goeval.LLM) goeval.Metric {
	return rename("llm_context_precision_without_reference", contextPrecision(llm, false))
}
func LLMContextRecall(llm goeval.LLM) goeval.Metric {
	return ContextRecall(llm)
}

func idMetric(recall bool) goeval.Metric {
	name := "id_based_context_precision"
	if recall {
		name = "id_based_context_recall"
	}
	return goeval.MetricFunc{MetricName: name, Fields: []goeval.Field{goeval.RetrievedContextIDs, goeval.ReferenceContextIDs}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if err := ctx.Err(); err != nil {
			return goeval.Score{}, err
		}
		retrieved, reference := map[string]bool{}, map[string]bool{}
		for _, id := range s.RetrievedContextIDs {
			retrieved[id] = true
		}
		for _, id := range s.ReferenceContextIDs {
			reference[id] = true
		}
		hits := 0
		for id := range retrieved {
			if reference[id] {
				hits++
			}
		}
		den := len(retrieved)
		if recall {
			den = len(reference)
		}
		if den == 0 {
			return goeval.Score{}, ErrUndefined
		}
		return goeval.Score{Value: float64(hits) / float64(den)}, nil
	}}
}
func IDBasedContextPrecision() goeval.Metric { return idMetric(false) }
func IDBasedContextRecall() goeval.Metric    { return idMetric(true) }

type ContextMatchOptions struct {
	Threshold *float64
	Distance  Distance
}

func nonLLMContext(opts ContextMatchOptions, recall bool) goeval.Metric {
	opts.Threshold = check.CopyPtr(opts.Threshold)
	name := "non_llm_context_precision_with_reference"
	if recall {
		name = "non_llm_context_recall"
	}
	return goeval.MetricFunc{MetricName: name, Fields: []goeval.Field{goeval.RetrievedContexts, goeval.ReferenceContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		threshold := 0.5
		if opts.Threshold != nil {
			threshold = *opts.Threshold
		}
		if threshold < 0 || threshold > 1 || math.IsNaN(threshold) {
			return goeval.Score{}, fmt.Errorf("metrics: threshold must be in [0,1]")
		}
		a, b := s.RetrievedContexts, s.ReferenceContexts
		if recall {
			a, b = b, a
		}
		v := make([]bool, len(a))
		for i, x := range a {
			if err := ctx.Err(); err != nil {
				return goeval.Score{}, err
			}
			for _, y := range b {
				score, err := StringSimilarity(x, y, opts.Distance)
				if err != nil {
					return goeval.Score{}, err
				}
				if score > threshold {
					v[i] = true
					break
				}
			}
		}
		if recall {
			score, err := meanVerdicts(v)
			return goeval.Score{Value: score}, err
		}
		return goeval.Score{Value: AveragePrecision(v)}, nil
	}}
}
func NonLLMContextPrecisionWithReference(opts ContextMatchOptions) goeval.Metric {
	return nonLLMContext(opts, false)
}
func NonLLMContextRecall(opts ContextMatchOptions) goeval.Metric { return nonLLMContext(opts, true) }

type FactualOptions struct {
	Mode      string
	Beta      *float64
	Atomicity string // low (default) or high, as in Python.
	Coverage  string // low (default) or high.
}

func decomposeClaims(ctx context.Context, llm goeval.LLM, text string, opts FactualOptions) ([]string, error) {
	a, c := opts.Atomicity, opts.Coverage
	if a == "" {
		a = "low"
	}
	if c == "" {
		c = "low"
	}
	if (a != "low" && a != "high") || (c != "low" && c != "high") {
		return nil, fmt.Errorf("metrics: atomicity and coverage must be low or high")
	}
	var out struct {
		Claims *[]string `json:"claims"`
	}
	err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.InstructionVariant("factual_correctness.ClaimDecompositionPrompt", a+"_atomicity_"+c+"_coverage"), Input: map[string]string{"response": text}, Schema: objectSchema(map[string]any{"claims": arraySchema(stringSchema())})}, &out)
	if err != nil {
		return nil, err
	}
	if out.Claims == nil {
		return nil, fmt.Errorf("metrics: missing decomposed claims")
	}
	return *out.Claims, nil
}
func factualCounts(ctx context.Context, llm goeval.LLM, s goeval.Sample, opts FactualOptions) (int, int, int, error) {
	answer, err := decomposeClaims(ctx, llm, s.Response, opts)
	if err != nil {
		return 0, 0, 0, err
	}
	a, err := verify(ctx, llm, s.Reference, answer)
	if err != nil {
		return 0, 0, 0, err
	}
	b := []bool{}
	if opts.Mode != "precision" {
		reference, err := decomposeClaims(ctx, llm, s.Reference, opts)
		if err != nil {
			return 0, 0, 0, err
		}
		b, err = verify(ctx, llm, s.Response, reference)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	tp := trueCount(a)
	return tp, len(a) - tp, len(b) - trueCount(b), nil
}
func factualValue(tp, fp, fn int, opts FactualOptions) (float64, error) {
	mode := opts.Mode
	beta := 1.0
	if opts.Beta != nil {
		beta = *opts.Beta
	}
	if mode == "" {
		mode = "f1"
	}
	if beta < 0 || math.IsNaN(beta) || math.IsInf(beta, 0) {
		return 0, fmt.Errorf("metrics: invalid beta")
	}
	p, r := 0.0, 0.0
	if tp+fp > 0 {
		p = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		r = float64(tp) / float64(tp+fn)
	}
	switch mode {
	case "precision":
		return p, nil
	case "recall":
		return r, nil
	case "f1":
		return fbeta(p, r, beta), nil
	default:
		return 0, fmt.Errorf("metrics: invalid factual mode %q", mode)
	}
}
func FactualCorrectness(llm goeval.LLM, opts FactualOptions) goeval.Metric {
	opts.Beta = check.CopyPtr(opts.Beta)
	return goeval.MetricFunc{MetricName: "factual_correctness", Fields: []goeval.Field{goeval.Response, goeval.Reference}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if _, err := factualValue(0, 0, 0, opts); err != nil {
			return goeval.Score{}, err
		}
		tp, fp, fn, err := factualCounts(ctx, llm, s, opts)
		if err != nil {
			return goeval.Score{}, err
		}
		v, err := factualValue(tp, fp, fn, opts)
		if opts.Mode == "precision" {
			v = float64(tp) / (float64(tp+fp) + 1e-8)
		} else if opts.Mode == "recall" {
			v = float64(tp) / (float64(tp+fn) + 1e-8)
		}
		return goeval.Score{Value: math.RoundToEven(v*100) / 100, Reason: fmt.Sprintf("TP=%d FP=%d FN=%d", tp, fp, fn)}, err
	}}
}

type AnswerCorrectnessOptions struct {
	Weights *[2]float64
	Beta    *float64
}

func AnswerCorrectness(llm goeval.LLM, embedder goeval.Embedder, opts AnswerCorrectnessOptions) goeval.Metric {
	opts.Beta = check.CopyPtr(opts.Beta)
	opts.Weights = check.CopyPtr(opts.Weights)
	return goeval.MetricFunc{MetricName: "answer_correctness", Fields: []goeval.Field{goeval.UserInput, goeval.Response, goeval.Reference}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		w := [2]float64{0.75, 0.25}
		if opts.Weights != nil {
			w = *opts.Weights
		}
		if w[0] < 0 || w[1] < 0 || w[0]+w[1] <= 0 || math.IsNaN(w[0]+w[1]) || math.IsInf(w[0]+w[1], 0) {
			return goeval.Score{}, fmt.Errorf("metrics: invalid correctness weights")
		}
		fact, err := answerFactuality(ctx, llm, s, opts.Beta)
		if err != nil {
			return goeval.Score{}, err
		}
		sim := 0.0
		if w[1] > 0 {
			v, err := SemanticSimilarity(embedder).Score(ctx, s)
			if err != nil {
				return goeval.Score{}, err
			}
			sim = v.Value
		}
		return goeval.Score{Value: (w[0]*fact + w[1]*sim) / (w[0] + w[1])}, nil
	}}
}
func AnswerRelevancy(llm goeval.LLM, embedder goeval.Embedder, strictness int) goeval.Metric {
	return goeval.MetricFunc{MetricName: "answer_relevancy", Fields: []goeval.Field{goeval.UserInput, goeval.Response}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		n := strictness
		if n == 0 {
			n = 3
		}
		if n < 1 || n > 100 {
			return goeval.Score{}, fmt.Errorf("metrics: strictness must be between 1 and 100")
		}
		if check.Nil(embedder) {
			return goeval.Score{}, fmt.Errorf("metrics: embedder is required")
		}
		questions := []string{s.UserInput}
		allNoncommittal := true
		allEmpty := true
		for i := 0; i < n; i++ {
			var out struct {
				Question     string `json:"question"`
				Noncommittal *int   `json:"noncommittal"`
			}
			err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("answer_relevance.ResponseRelevancePrompt"), Input: map[string]string{"response": s.Response}, Schema: objectSchema(map[string]any{"question": stringSchema(), "noncommittal": binarySchema()})}, &out)
			if err != nil {
				return goeval.Score{}, err
			}
			non, err := boolValue(out.Noncommittal)
			if err != nil {
				return goeval.Score{}, err
			}
			allNoncommittal = allNoncommittal && non
			allEmpty = allEmpty && out.Question == ""
			questions = append(questions, out.Question)
		}
		if allEmpty {
			return goeval.Score{}, ErrUndefined
		}
		vectors, err := embedder.Embed(ctx, questions)
		if err != nil {
			return goeval.Score{}, err
		}
		if len(vectors) != len(questions) {
			return goeval.Score{}, fmt.Errorf("metrics: embedding count mismatch")
		}
		sum := 0.0
		for _, v := range vectors[1:] {
			c, err := Cosine(vectors[0], v)
			if err != nil {
				return goeval.Score{}, err
			}
			sum += c
		}
		value := sum / float64(n)
		if allNoncommittal {
			value = 0
		}
		return goeval.Score{Value: value}, nil
	}}
}
func ResponseRelevancy(llm goeval.LLM, e goeval.Embedder, n int) goeval.Metric {
	return AnswerRelevancy(llm, e, n)
}

func ContextEntityRecall(llm goeval.LLM) goeval.Metric {
	return goeval.MetricFunc{MetricName: "context_entity_recall", Fields: []goeval.Field{goeval.Reference, goeval.RetrievedContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		extract := func(text string) (map[string]bool, error) {
			var out struct {
				Entities *[]string `json:"entities"`
			}
			err := goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("context_entities_recall.ExtractEntitiesPrompt"), Input: map[string]string{"text": text}, Schema: objectSchema(map[string]any{"entities": arraySchema(stringSchema())})}, &out)
			if err != nil {
				return nil, err
			}
			if out.Entities == nil {
				return nil, fmt.Errorf("metrics: missing entities")
			}
			m := map[string]bool{}
			for _, e := range *out.Entities {
				if strings.TrimSpace(e) == "" {
					return nil, fmt.Errorf("metrics: empty entity")
				}
				m[e] = true
			}
			return m, nil
		}
		ref, err := extract(s.Reference)
		if err != nil {
			return goeval.Score{}, err
		}
		got, err := extract(strings.Join(s.RetrievedContexts, "\n"))
		if err != nil {
			return goeval.Score{}, err
		}
		hits := 0
		for entity := range ref {
			if got[entity] {
				hits++
			}
		}
		return goeval.Score{Value: float64(hits) / (float64(len(ref)) + 1e-8)}, nil
	}}
}

// NoiseSensitivity is an error rate: lower is better.
func NoiseSensitivity(llm goeval.LLM, mode string) goeval.Metric {
	return goeval.MetricFunc{MetricName: "noise_sensitivity", Fields: []goeval.Field{goeval.UserInput, goeval.Response, goeval.Reference, goeval.RetrievedContexts}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if mode != "" && mode != "relevant" && mode != "irrelevant" {
			return goeval.Score{}, fmt.Errorf("metrics: invalid noise mode")
		}
		gt, err := decompose(ctx, llm, s.UserInput, s.Reference)
		if err != nil {
			return goeval.Score{}, err
		}
		ans, err := decompose(ctx, llm, s.UserInput, s.Response)
		if err != nil {
			return goeval.Score{}, err
		}
		if len(ans) == 0 || len(s.RetrievedContexts) == 0 {
			return goeval.Score{}, ErrUndefined
		}
		relevantFaithful, irrelevantFaithful := make([]bool, len(ans)), make([]bool, len(ans))
		for _, passage := range s.RetrievedContexts {
			g, err := verify(ctx, llm, passage, gt)
			if err != nil {
				return goeval.Score{}, err
			}
			a, err := verify(ctx, llm, passage, ans)
			if err != nil {
				return goeval.Score{}, err
			}
			relevant := trueCount(g) > 0
			for i, v := range a {
				if relevant {
					relevantFaithful[i] = relevantFaithful[i] || v
				} else {
					irrelevantFaithful[i] = irrelevantFaithful[i] || v
				}
			}
		}
		correct, err := verify(ctx, llm, s.Reference, ans)
		if err != nil {
			return goeval.Score{}, err
		}
		errors := 0
		for i, c := range correct {
			if c {
				continue
			}
			if mode == "irrelevant" {
				if irrelevantFaithful[i] && !relevantFaithful[i] {
					errors++
				}
			} else if relevantFaithful[i] {
				errors++
			}
		}
		return goeval.Score{Value: float64(errors) / float64(len(ans))}, nil
	}}
}
