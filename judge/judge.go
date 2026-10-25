package judge

import (
	"context"
	"fmt"
	"strings"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/internal/judgetemplates"
)

type Verdict struct {
	Verdict   string  `json:"verdict"`
	Reason    *string `json:"reason,omitempty"`
	Statement string  `json:"statement,omitempty"`
}

func reason(v Verdict) any {
	if v.Reason == nil {
		return nil
	}
	return *v.Reason
}
func judge(ctx context.Context, llm goeval.LLM, key string, values map[string]any, outputSchema map[string]any, out any) error {
	text, err := judgetemplates.Render(key, values)
	if err != nil {
		return err
	}
	return goeval.GenerateJSON(ctx, llm, goeval.Prompt{Text: text, Input: values, Schema: outputSchema}, out)
}
func stringList(ctx context.Context, llm goeval.LLM, key, field string, values map[string]any) ([]string, error) {
	var out map[string][]string
	if err := judge(ctx, llm, key, values, schema(map[string]any{field: array(str())}), &out); err != nil {
		return nil, err
	}
	v, ok := out[field]
	if !ok || v == nil {
		return nil, fmt.Errorf("judge: missing %q", field)
	}
	for i, item := range v {
		if strings.TrimSpace(item) == "" {
			return nil, fmt.Errorf("judge: %s item %d must be nonempty", field, i)
		}
	}
	return v, nil
}
func normalizeVerdict(raw string, allowBorderline bool) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	n := 0
	for _, r := range s {
		if r < 'a' || r > 'z' {
			break
		}
		n++
	}
	s = s[:n]
	if s == "idk" {
		s = "borderline"
	}
	if s == "yes" || s == "no" || (allowBorderline && s == "borderline") {
		return s
	}
	return ""
}
func verdicts(ctx context.Context, llm goeval.LLM, key string, values map[string]any, borderline, statements bool) ([]Verdict, error) {
	allowed := []string{"yes", "no"}
	if borderline {
		allowed = append(allowed, "borderline")
	}
	properties := map[string]any{"verdict": map[string]any{"type": "string", "enum": allowed}, "reason": map[string]any{"type": []string{"string", "null"}}}
	if statements {
		properties["statement"] = str()
	}
	var out struct {
		Verdicts *[]Verdict `json:"verdicts"`
	}
	if err := judge(ctx, llm, key, values, schema(map[string]any{"verdicts": array(schema(properties))}), &out); err != nil {
		return nil, err
	}
	if out.Verdicts == nil {
		return nil, fmt.Errorf("judge: missing verdicts")
	}
	valid := []Verdict{}
	for i, v := range *out.Verdicts {
		v.Verdict = normalizeVerdict(v.Verdict, borderline)
		if v.Verdict == "" {
			return nil, fmt.Errorf("judge: unrecognized verdict at index %d", i)
		}
		valid = append(valid, v)
	}
	return valid, nil
}
func fraction(v []Verdict, passing []string, empty float64) float64 {
	if len(v) == 0 {
		return empty
	}
	count := 0
	for _, item := range v {
		for _, pass := range passing {
			if normalizeVerdict(item.Verdict, true) == pass {
				count++
				break
			}
		}
	}
	return float64(count) / float64(len(v))
}
func validateVerdictCount(verdicts []Verdict, expected int) error {
	if len(verdicts) != expected {
		return fmt.Errorf("judge: expected %d verdicts, received %d", expected, len(verdicts))
	}
	return nil
}

func explanation(ctx context.Context, llm goeval.LLM, key string, values map[string]any, opts Options) (string, error) {
	if !includeReason(opts) {
		return "", nil
	}
	var out struct {
		Reason *string `json:"reason"`
	}
	if err := judge(ctx, llm, key, values, schema(map[string]any{"reason": str()}), &out); err != nil {
		return "", err
	}
	if out.Reason == nil {
		return "", fmt.Errorf("judge: missing reason")
	}
	return *out.Reason, nil
}
func AnswerRelevancyMetric(llm goeval.LLM, opts Options) Metric {
	opts = cloneOptions(opts)
	return newModelMetric(llm, "answer_relevancy", "Answer Relevancy", []SingleTurnParam{Input, ActualOutput}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		statements, err := stringList(ctx, llm, "AnswerRelevancyMetric.generate_statements", "statements", map[string]any{"actual_output": c.ActualOutput})
		if err != nil {
			return 0, "", nil, err
		}
		v := []Verdict{}
		if len(statements) > 0 {
			v, err = verdicts(ctx, llm, "AnswerRelevancyMetric.generate_verdicts", map[string]any{"input": c.Input, "statements": statements}, true, false)
			if err != nil {
				return 0, "", nil, err
			}
		}
		if err := validateVerdictCount(v, len(statements)); err != nil {
			return 0, "", nil, err
		}
		score := strictScore(fraction(v, []string{"yes", "borderline"}, 1), threshold, opts)
		irrelevant := []any{}
		for _, item := range v {
			if item.Verdict == "no" {
				irrelevant = append(irrelevant, reason(item))
			}
		}
		r, err := explanation(ctx, llm, "AnswerRelevancyMetric.generate_reason", map[string]any{"input": c.Input, "score": fmt.Sprintf("%.2f", score), "irrelevant_statements": irrelevant}, opts)
		return score, r, map[string]any{"statements": statements, "verdicts": v}, err
	})
}
func FaithfulnessMetric(llm goeval.LLM, opts Options) Metric {
	opts = cloneOptions(opts)
	return newModelMetric(llm, "faithfulness", "Faithfulness", []SingleTurnParam{Input, ActualOutput, RetrievalContext}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		limit := truthsLimit(opts.TruthsExtractionLimit)
		truths, err := stringList(ctx, llm, "FaithfulnessMetric.generate_truths", "truths", map[string]any{"retrieval_context": strings.Join(c.RetrievalContext, "\n\n"), "limit": limit, "multimodal_instruction": ""})
		if err != nil {
			return 0, "", nil, err
		}
		claims, err := stringList(ctx, llm, "FaithfulnessMetric.generate_claims", "claims", map[string]any{"actual_output": c.ActualOutput, "multimodal_instruction": ""})
		if err != nil {
			return 0, "", nil, err
		}
		v := []Verdict{}
		if len(claims) > 0 {
			v, err = verdicts(ctx, llm, "FaithfulnessMetric.generate_verdicts", map[string]any{"claims": claims, "retrieval_context": strings.Join(truths, "\n\n")}, true, false)
			if err != nil {
				return 0, "", nil, err
			}
		}
		if err := validateVerdictCount(v, len(claims)); err != nil {
			return 0, "", nil, err
		}
		passing := []string{"yes", "borderline"}
		if opts.PenalizeAmbiguousClaims {
			passing = []string{"yes"}
		}
		score := strictScore(fraction(v, passing, 1), threshold, opts)
		contradictions := []any{}
		for _, item := range v {
			if item.Verdict == "no" {
				contradictions = append(contradictions, reason(item))
			} else if item.Verdict == "borderline" && opts.PenalizeAmbiguousClaims {
				contradictions = append(contradictions, fmt.Sprint("(Ambiguous) ", goeval.PythonString(reason(item))))
			}
		}
		r, err := explanation(ctx, llm, "FaithfulnessMetric.generate_reason", map[string]any{"score": fmt.Sprintf("%.2f", score), "contradictions": contradictions}, opts)
		return score, r, map[string]any{"truths": truths, "claims": claims, "verdicts": v}, err
	})
}
func ContextualPrecisionMetric(llm goeval.LLM, opts Options) Metric {
	opts = cloneOptions(opts)
	return newModelMetric(llm, "contextual_precision", "Contextual Precision", []SingleTurnParam{Input, ActualOutput, ExpectedOutput, RetrievalContext}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		suffix := ""
		if len(c.RetrievalContext) > 1 {
			suffix = "s"
		}
		v, err := verdicts(ctx, llm, "ContextualPrecisionMetric.generate_verdicts", map[string]any{"input": c.Input, "expected_output": c.ExpectedOutput, "document_count_str": fmt.Sprintf(" (%d document%s)", len(c.RetrievalContext), suffix), "context_to_display": c.RetrievalContext, "multimodal_note": ""}, false, false)
		if err != nil {
			return 0, "", nil, err
		}
		if err := validateVerdictCount(v, len(c.RetrievalContext)); err != nil {
			return 0, "", nil, err
		}
		score := contextualPrecision(v)
		score = strictScore(score, threshold, opts)
		rows := make([]map[string]any, len(v))
		for i, item := range v {
			rows[i] = map[string]any{"verdict": item.Verdict, "reason": reason(item)}
		}
		r, err := explanation(ctx, llm, "ContextualPrecisionMetric.generate_reason", map[string]any{"input": c.Input, "verdicts": rows, "score": fmt.Sprintf("%.2f", score)}, opts)
		return score, r, map[string]any{"verdicts": v}, err
	})
}
func contextualPrecision(v []Verdict) float64 {
	hits, sum := 0, 0.0
	for i, item := range v {
		if item.Verdict == "yes" {
			hits++
			sum += float64(hits) / float64(i+1)
		}
	}
	if hits == 0 {
		return 0
	}
	return sum / float64(hits)
}
func ContextualRecallMetric(llm goeval.LLM, opts Options) Metric {
	opts = cloneOptions(opts)
	return newModelMetric(llm, "contextual_recall", "Contextual Recall", []SingleTurnParam{Input, ActualOutput, ExpectedOutput, RetrievalContext}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		v, err := verdicts(ctx, llm, "ContextualRecallMetric.generate_verdicts", map[string]any{"expected_output": c.ExpectedOutput, "content_type": "sentence", "content_type_plural": "sentences", "content_or": "sentence", "context_to_display": c.RetrievalContext, "node_instruction": ""}, false, false)
		if err != nil {
			return 0, "", nil, err
		}
		score := strictScore(fraction(v, []string{"yes"}, 0), threshold, opts)
		supportive, unsupportive := []any{}, []any{}
		for _, item := range v {
			if item.Verdict == "yes" {
				supportive = append(supportive, reason(item))
			} else {
				unsupportive = append(unsupportive, reason(item))
			}
		}
		r, err := explanation(ctx, llm, "ContextualRecallMetric.generate_reason", map[string]any{"expected_output": c.ExpectedOutput, "content_type": "sentence", "supportive_reasons": supportive, "unsupportive_reasons": unsupportive, "score": fmt.Sprintf("%.2f", score)}, opts)
		return score, r, map[string]any{"verdicts": v}, err
	})
}
func ContextualRelevancyMetric(llm goeval.LLM, opts Options) Metric {
	opts = cloneOptions(opts)
	return newModelMetric(llm, "contextual_relevancy", "Contextual Relevancy", []SingleTurnParam{Input, ActualOutput, RetrievalContext}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		all := []Verdict{}
		for _, passage := range c.RetrievalContext {
			v, err := verdicts(ctx, llm, "ContextualRelevancyMetric.generate_verdicts", map[string]any{"input": c.Input, "context": passage, "context_type": "context", "statement_or_image": "statement", "extraction_instructions": "You should first extract statements found in the context, which are high level information found in the context, before deciding on a verdict and optionally a reason for each statement.", "empty_context_instruction": "\nIf provided context contains no actual content or statements then: give \"no\" as a \"verdict\",\nput context into \"statement\", and \"No statements found in provided context.\" into \"reason\"."}, false, true)
			if err != nil {
				return 0, "", nil, err
			}
			all = append(all, v...)
		}
		score := strictScore(fraction(all, []string{"yes"}, 0), threshold, opts)
		irrelevant, relevant := []any{}, []string{}
		for _, item := range all {
			if item.Verdict == "no" {
				irrelevant = append(irrelevant, reason(item))
			} else {
				relevant = append(relevant, item.Statement)
			}
		}
		r, err := explanation(ctx, llm, "ContextualRelevancyMetric.generate_reason", map[string]any{"input": c.Input, "irrelevant_statements": irrelevant, "relevant_statements": relevant, "score": fmt.Sprintf("%.2f", score)}, opts)
		return score, r, map[string]any{"verdicts": all}, err
	})
}
func HallucinationMetric(llm goeval.LLM, opts Options) Metric {
	opts = cloneOptions(opts)
	return newModelMetric(llm, "hallucination", "Hallucination", []SingleTurnParam{Input, ActualOutput, Context}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		v, err := verdicts(ctx, llm, "HallucinationMetric.generate_verdicts", map[string]any{"actual_output": c.ActualOutput, "contexts": c.Context, "contexts_count": len(c.Context)}, false, false)
		if err != nil {
			return 0, "", nil, err
		}
		if err := validateVerdictCount(v, len(c.Context)); err != nil {
			return 0, "", nil, err
		}
		score := strictScore(fraction(v, []string{"yes"}, 1), threshold, opts)
		align, contradict := []any{}, []any{}
		for _, item := range v {
			if item.Verdict == "yes" {
				align = append(align, reason(item))
			} else {
				contradict = append(contradict, reason(item))
			}
		}
		r, err := explanation(ctx, llm, "HallucinationMetric.generate_reason", map[string]any{"factual_alignments": align, "contradictions": contradict, "score": fmt.Sprintf("%.2f", score)}, opts)
		return score, r, map[string]any{"verdicts": v}, err
	})
}
func opinionMetric(llm goeval.LLM, opts Options, toxicity bool) Metric {
	opts = cloneOptions(opts)
	name, class, display, badKey := "bias", "BiasMetric", "Bias", "biases"
	if toxicity {
		name, class, display, badKey = "toxicity", "ToxicityMetric", "Toxicity", "toxics"
	}
	return newModelMetric(llm, name, display, []SingleTurnParam{Input, ActualOutput}, opts, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		opinions, err := stringList(ctx, llm, class+".generate_opinions", "opinions", map[string]any{"actual_output": c.ActualOutput})
		if err != nil {
			return 0, "", nil, err
		}
		v := []Verdict{}
		if len(opinions) > 0 {
			v, err = verdicts(ctx, llm, class+".generate_verdicts", map[string]any{"opinions": opinions}, false, false)
			if err != nil {
				return 0, "", nil, err
			}
		}
		if err := validateVerdictCount(v, len(opinions)); err != nil {
			return 0, "", nil, err
		}
		score := strictScore(fraction(v, []string{"no"}, 1), threshold, opts)
		bad := []any{}
		for _, item := range v {
			if item.Verdict == "yes" {
				bad = append(bad, reason(item))
			}
		}
		r, err := explanation(ctx, llm, class+".generate_reason", map[string]any{badKey: bad, "score": fmt.Sprintf("%.2f", score)}, opts)
		return score, r, map[string]any{"opinions": opinions, "verdicts": v}, err
	})
}
func BiasMetric(llm goeval.LLM, opts Options) Metric     { return opinionMetric(llm, opts, false) }
func ToxicityMetric(llm goeval.LLM, opts Options) Metric { return opinionMetric(llm, opts, true) }
