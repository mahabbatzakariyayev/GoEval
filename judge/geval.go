package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/internal/judgetemplates"
)

type Rubric struct {
	ScoreRange      [2]int `json:"score_range"`
	ExpectedOutcome string `json:"expected_outcome"`
}
type GEvalOptions struct {
	Options
	Name              string
	Criteria          string
	EvaluationSteps   []string
	EvaluationParams  []SingleTurnParam
	Rubric            []Rubric
	TopLogProbs       int   // Default 20.
	UseLogProbs       *bool // Default: use LogProbLLM when available.
	AdditionalContext string
}

var parameterNames = map[SingleTurnParam]string{Input: "Input", ActualOutput: "Actual Output", ExpectedOutput: "Expected Output", Context: "Context", RetrievalContext: "Retrieval Context", Metadata: "Metadata", Tags: "Tags", ToolsCalled: "Tools Called", ExpectedTools: "Expected Tools"}

// GEval generates criterion-specific evaluation steps, applies the original
// judge prompt, and normalizes the 0–10 (or rubric) score. It uses the last score
// token and its alternatives when the provider supports token log probabilities.
func GEval(llm goeval.LLM, cfg GEvalOptions) (Metric, error) {
	cfg.Options = cloneOptions(cfg.Options)
	cfg.UseLogProbs = check.CopyPtr(cfg.UseLogProbs)
	if strings.TrimSpace(cfg.Name) == "" || len(cfg.EvaluationParams) == 0 {
		return nil, fmt.Errorf("judge: G-Eval needs a name and evaluation parameters")
	}
	if cfg.Criteria == "" && len(cfg.EvaluationSteps) == 0 {
		return nil, fmt.Errorf("judge: G-Eval needs criteria or evaluation steps")
	}
	if cfg.Criteria != "" && strings.TrimSpace(cfg.Criteria) == "" {
		return nil, fmt.Errorf("judge: empty criteria")
	}
	if cfg.EvaluationSteps != nil && len(cfg.EvaluationSteps) == 0 {
		return nil, fmt.Errorf("judge: evaluation steps cannot be empty")
	}
	params := append([]SingleTurnParam(nil), cfg.EvaluationParams...)
	labels := make([]string, len(params))
	for i, p := range params {
		label, ok := parameterNames[p]
		if !ok {
			return nil, fmt.Errorf("judge: unsupported G-Eval parameter %q", p)
		}
		labels[i] = label
	}
	steps := append([]string(nil), cfg.EvaluationSteps...)
	for _, s := range steps {
		if strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("judge: empty evaluation step")
		}
	}
	rubrics := append([]Rubric(nil), cfg.Rubric...)
	sort.Slice(rubrics, func(i, j int) bool { return rubrics[i].ScoreRange[0] < rubrics[j].ScoreRange[0] })
	low, high := 0, 10
	rubricLines := []string{}
	for i, r := range rubrics {
		a, b := r.ScoreRange[0], r.ScoreRange[1]
		if a < 0 || b > 10 || a > b {
			return nil, fmt.Errorf("judge: invalid rubric range")
		}
		if i > 0 && rubrics[i-1].ScoreRange[1] >= a {
			return nil, fmt.Errorf("judge: overlapping rubric ranges")
		}
		label := strconv.Itoa(a)
		if a != b {
			label = fmt.Sprintf("%d-%d", a, b)
		}
		rubricLines = append(rubricLines, label+": "+r.ExpectedOutcome)
	}
	if len(rubrics) > 0 {
		low, high = rubrics[0].ScoreRange[0], rubrics[len(rubrics)-1].ScoreRange[1]
	}
	if low == high {
		return nil, fmt.Errorf("judge: G-Eval rubric must span more than one score")
	}
	if cfg.TopLogProbs == 0 {
		cfg.TopLogProbs = 20
	}
	if cfg.TopLogProbs < 1 || cfg.TopLogProbs > 20 {
		return nil, fmt.Errorf("judge: top logprobs must be in [1,20]")
	}
	var stepsMu sync.Mutex
	ensureSteps := func(ctx context.Context) ([]string, error) {
		stepsMu.Lock()
		defer stepsMu.Unlock()
		if len(steps) > 0 {
			return append([]string(nil), steps...), nil
		}
		generated, err := stringList(ctx, llm, "GEval.generate_evaluation_steps", "steps", map[string]any{"criteria": cfg.Criteria, "parameters": joinLabels(labels)})
		if err != nil {
			return nil, err
		}
		if len(generated) == 0 {
			return nil, fmt.Errorf("judge: no evaluation steps generated")
		}
		steps = append([]string(nil), generated...)
		return append([]string(nil), steps...), nil
	}
	m := newModelMetric(llm, "g_eval."+cfg.Name, cfg.Name+" (GEval)", params, cfg.Options, func(ctx context.Context, c Case, _ float64) (float64, string, map[string]any, error) {
		current, err := ensureSteps(ctx)
		if err != nil {
			return 0, "", nil, err
		}
		var numbered, content strings.Builder
		for i, s := range current {
			fmt.Fprintf(&numbered, "%d. %s\n", i+1, s)
		}
		for _, p := range params {
			fmt.Fprintf(&content, "%s:\n%s \n\n", parameterNames[p], goeval.PythonString(c.value(p)))
		}
		vars := map[string]any{"evaluation_steps": numbered.String(), "test_case_content": content.String(), "parameters": joinLabels(labels), "rubric": nil, "score_range": []int{low, high}, "_additional_context": cfg.AdditionalContext}
		if len(rubricLines) > 0 {
			vars["rubric"] = strings.Join(rubricLines, "\n")
		}
		key := "GEval.generate_evaluation_results"
		outLow, outHigh := float64(low), float64(high)
		if cfg.StrictMode {
			key = "GEval.generate_strict_evaluation_results"
			outLow, outHigh = 0, 1
		}
		text, err := judgetemplates.Render(key, vars)
		if err != nil {
			return 0, "", nil, err
		}
		p := goeval.Prompt{Text: text, Input: vars, TopLogProbs: cfg.TopLogProbs, Schema: schema(map[string]any{"score": map[string]any{"type": "number", "minimum": outLow, "maximum": outHigh}, "reason": str()})}
		var raw string
		var tokens []goeval.TokenLogProb
		if probabilistic, ok := llm.(goeval.LogProbLLM); ok && (cfg.UseLogProbs == nil || *cfg.UseLogProbs) {
			completion, err := probabilistic.GenerateWithLogProbs(ctx, p)
			if err != nil {
				return 0, "", nil, err
			}
			raw, tokens = completion.Text, completion.LogProbs
		} else {
			if check.Nil(llm) {
				return 0, "", nil, fmt.Errorf("judge: LLM is required")
			}
			raw, err = llm.Generate(ctx, p)
			if err != nil {
				return 0, "", nil, err
			}
		}
		var out struct {
			Score  *float64 `json:"score"`
			Reason *string  `json:"reason"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return 0, "", nil, fmt.Errorf("judge: invalid G-Eval JSON: %w", err)
		}
		if out.Score == nil || out.Reason == nil || *out.Score < outLow || *out.Score > outHigh || math.IsNaN(*out.Score) {
			return 0, "", nil, fmt.Errorf("judge: invalid G-Eval score or missing reason")
		}
		score := *out.Score
		weightedUsed := false
		if !cfg.StrictMode && len(tokens) > 0 {
			weighted, err := WeightedScore(score, tokens)
			// Upstream falls back to the raw score when token matching fails.
			if err == nil {
				score = weighted
				weightedUsed = true
			}
		}
		if cfg.StrictMode {
			if score != 0 && score != 1 {
				return 0, "", nil, fmt.Errorf("judge: strict G-Eval score must be 0 or 1")
			}
		} else {
			score = (score - float64(low)) / float64(high-low)
		}
		return score, *out.Reason, map[string]any{"evaluation_steps": current, "raw_score": *out.Score, "weighted": weightedUsed}, nil
	})
	return m, nil
}
func joinLabels(labels []string) string {
	switch len(labels) {
	case 0:
		return ""
	case 1:
		return labels[0]
	case 2:
		return labels[0] + " and " + labels[1]
	default:
		return strings.Join(labels[:len(labels)-1], ", ") + ", and " + labels[len(labels)-1]
	}
}

// WeightedScore ports Judge's last-score-token reducer and 1% probability
// filter. Tokens that contain signs, whitespace, or punctuation are excluded.
func WeightedScore(raw float64, tokens []goeval.TokenLogProb) (float64, error) {
	text := strconv.FormatFloat(raw, 'f', -1, 64)
	var selected *goeval.TokenLogProb
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].Token == text {
			selected = &tokens[i]
			break
		}
	}
	if selected == nil {
		return 0, fmt.Errorf("judge: score token %q missing from log probabilities", text)
	}
	sum, total := 0.0, 0.0
	for _, alt := range selected.TopLogProbs {
		if math.IsNaN(alt.LogProb) || math.IsInf(alt.LogProb, 1) || alt.LogProb > 0 {
			return 0, fmt.Errorf("judge: invalid token log probability")
		}
		if alt.LogProb < math.Log(0.01) {
			continue
		}
		decimal := alt.Token != ""
		for _, r := range alt.Token {
			if r < '0' || r > '9' {
				decimal = false
				break
			}
		}
		if !decimal {
			continue
		}
		value, err := strconv.ParseFloat(alt.Token, 64)
		if err != nil {
			return 0, err
		}
		prob := math.Exp(alt.LogProb)
		sum += value * prob
		total += prob
		if math.IsInf(sum, 0) {
			return 0, fmt.Errorf("judge: token score overflow")
		}
	}
	if total == 0 {
		return raw, nil
	}
	return sum / total, nil
}
