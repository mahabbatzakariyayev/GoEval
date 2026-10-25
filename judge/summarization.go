package judge

import (
	"context"
	"fmt"
	"strings"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

type SummarizationOptions struct {
	Options
	N                   int // Number of generated assessment questions; defaults to 5.
	AssessmentQuestions []string
}

type CoverageVerdict struct {
	OriginalVerdict string `json:"original_verdict"`
	SummaryVerdict  string `json:"summary_verdict"`
	Question        string `json:"question"`
}

// SummarizationMetric scores alignment and question coverage separately and
// returns their minimum, preserving Judge's penalty for borderline claims.
func SummarizationMetric(llm goeval.LLM, cfg SummarizationOptions) Metric {
	cfg.Options = cloneOptions(cfg.Options)
	questions := append([]string(nil), cfg.AssessmentQuestions...)
	m := newModelMetric(llm, "summarization", "Summarization", []SingleTurnParam{Input, ActualOutput}, cfg.Options, func(ctx context.Context, c Case, threshold float64) (float64, string, map[string]any, error) {
		n := cfg.N
		if n == 0 {
			n = 5
		}
		if n < 0 {
			return 0, "", nil, fmt.Errorf("judge: question count must be positive")
		}
		truths, err := stringList(ctx, llm, "FaithfulnessMetric.generate_truths", "truths", map[string]any{"retrieval_context": c.Input, "limit": truthsLimit(cfg.TruthsExtractionLimit), "multimodal_instruction": ""})
		if err != nil {
			return 0, "", nil, err
		}
		claims, err := stringList(ctx, llm, "FaithfulnessMetric.generate_claims", "claims", map[string]any{"actual_output": c.ActualOutput, "multimodal_instruction": ""})
		if err != nil {
			return 0, "", nil, err
		}
		// Generate per sample instead of mutating a shared metric's questions.
		current := append([]string(nil), questions...)
		if len(current) == 0 {
			current, err = stringList(ctx, llm, "SummarizationMetric.generate_questions", "questions", map[string]any{"text": c.Input, "n": n})
			if err != nil {
				return 0, "", nil, err
			}
		}
		original, err := stringList(ctx, llm, "SummarizationMetric.generate_answers", "answers", map[string]any{"text": c.Input, "questions": current})
		if err != nil {
			return 0, "", nil, err
		}
		summary, err := stringList(ctx, llm, "SummarizationMetric.generate_answers", "answers", map[string]any{"text": c.ActualOutput, "questions": current})
		if err != nil {
			return 0, "", nil, err
		}
		if len(original) != len(summary) || len(original) != len(current) {
			return 0, "", nil, fmt.Errorf("judge: assessment question and answer counts differ")
		}
		coverage := make([]CoverageVerdict, len(current))
		total, covered := 0, 0
		missing := []string{}
		for i, q := range current {
			original[i], summary[i] = normalizeVerdict(original[i], false), normalizeVerdict(summary[i], false)
			if original[i] == "" || summary[i] == "" {
				return 0, "", nil, fmt.Errorf("judge: summary assessment answers must be yes or no")
			}
			coverage[i] = CoverageVerdict{OriginalVerdict: original[i], SummaryVerdict: summary[i], Question: q}
			if strings.EqualFold(strings.TrimSpace(original[i]), "yes") {
				total++
				if strings.EqualFold(strings.TrimSpace(summary[i]), "yes") {
					covered++
				}
				if strings.EqualFold(strings.TrimSpace(summary[i]), "no") {
					missing = append(missing, q)
				}
			}
		}
		coverageScore := 0.0
		if total > 0 {
			coverageScore = float64(covered) / float64(total)
		}
		alignment := []Verdict{}
		if len(claims) > 0 {
			alignment, err = verdicts(ctx, llm, "SummarizationMetric.generate_alignment_verdicts", map[string]any{"summary_claims": claims, "original_text": strings.Join(truths, "\n\n")}, true, false)
			if err != nil {
				return 0, "", nil, err
			}
		}
		if err := validateVerdictCount(alignment, len(claims)); err != nil {
			return 0, "", nil, err
		}
		alignmentScore := strictScore(fraction(alignment, []string{"yes"}, 0), threshold, cfg.Options)
		coverageScore = strictScore(coverageScore, threshold, cfg.Options)
		score := min(alignmentScore, coverageScore)
		contradictions, redundancies := []any{}, []any{}
		for _, v := range alignment {
			if v.Verdict == "no" {
				contradictions = append(contradictions, reason(v))
			}
			if v.Verdict == "borderline" {
				redundancies = append(redundancies, reason(v))
			}
		}
		r, err := explanation(ctx, llm, "SummarizationMetric.generate_reason", map[string]any{"contradictions": contradictions, "redundancies": redundancies, "questions": missing, "score": fmt.Sprintf("%.2f", score)}, cfg.Options)
		return score, r, map[string]any{"truths": truths, "claims": claims, "assessment_questions": current, "coverage_verdicts": coverage, "alignment_verdicts": alignment, "score_breakdown": map[string]float64{"Alignment": alignmentScore, "Coverage": coverageScore}}, err
	})
	if cfg.N < 0 {
		m.configErr = fmt.Errorf("judge: question count must be positive")
	}
	for _, question := range questions {
		if strings.TrimSpace(question) == "" {
			m.configErr = fmt.Errorf("judge: assessment questions must be nonempty")
		}
	}
	return m
}

func truthsLimit(limit *int) string {
	if limit == nil {
		return " FACTUAL, undisputed truths"
	}
	n := max(*limit, 0)
	if n == 1 {
		return " the single most important FACTUAL, undisputed truth"
	}
	return fmt.Sprintf(" the %d most important FACTUAL, undisputed truths per document", n)
}
