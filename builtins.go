package goeval

import (
	"context"
	"fmt"
	"math"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	"github.com/mahabbatzakariyayev/GoEval/judge"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

type judgeConfig struct {
	options  judge.Options
	expected bool
	logProbs bool
}

// JudgeOption adjusts a model-based metric's quality gate or explanation.
type JudgeOption func(*judgeConfig) error

// Threshold sets the minimum passing score in [0,1]. Judges default to 0.5.
func Threshold(minimum float64) JudgeOption {
	return func(cfg *judgeConfig) error {
		if math.IsNaN(minimum) || minimum < 0 || minimum > 1 {
			return fmt.Errorf("goeval: threshold must be in [0,1]")
		}
		cfg.options.Threshold = judge.Float(minimum)
		return nil
	}
}

// Strict requires a perfect score and enables the metric's strict scoring mode.
func Strict() JudgeOption {
	return func(cfg *judgeConfig) error { cfg.options.StrictMode = true; return nil }
}

// WithoutReason skips optional explanation requests and omits reason text.
func WithoutReason() JudgeOption {
	return func(cfg *judgeConfig) error { cfg.options.IncludeReason = judge.Bool(false); return nil }
}

// AgainstExpected includes Expected when building a custom Judge prompt.
func AgainstExpected() JudgeOption {
	return func(cfg *judgeConfig) error { cfg.expected = true; return nil }
}

// LogProbs enables probability weighting for custom Judge scorers. The default
// avoids requiring optional log-probability support from compatible endpoints.
func LogProbs() JudgeOption { return func(cfg *judgeConfig) error { cfg.logProbs = true; return nil } }

func judgeOptions(options []JudgeOption) (judgeConfig, error) {
	cfg := judgeConfig{}
	for _, option := range options {
		if option == nil {
			return cfg, fmt.Errorf("goeval: nil judge option")
		}
		if err := option(&cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

type builtinMetric struct {
	metric                            Metric
	name                              string
	model                             LLM
	embedder                          Embedder
	requiresModel, requiresEmbeddings bool
	err                               error
}

func (m builtinMetric) Name() string { return m.name }
func (m builtinMetric) RequiredFields() []Field {
	if check.Nil(m.metric) {
		return nil
	}
	return m.metric.RequiredFields()
}
func (m builtinMetric) Validate() error {
	if m.err != nil {
		return m.err
	}
	if m.requiresModel && check.Nil(m.model) {
		return fmt.Errorf("goeval: %s requires a model", m.name)
	}
	if m.requiresEmbeddings && check.Nil(m.embedder) {
		return fmt.Errorf("goeval: %s requires an embedder", m.name)
	}
	return ValidateMetrics([]Metric{m.metric})
}
func (m builtinMetric) ValidateSample(s Sample) error {
	if err := s.Validate(m.RequiredFields()...); err != nil {
		return err
	}
	if validator, ok := m.metric.(interface{ ValidateSample(Sample) error }); ok {
		return validator.ValidateSample(s)
	}
	return nil
}
func (m builtinMetric) Score(ctx context.Context, s Sample) (Score, error) {
	if err := check.Context(ctx); err != nil {
		return Score{}, err
	}
	if err := m.Validate(); err != nil {
		return Score{}, err
	}
	return m.metric.Score(ctx, s)
}

func modelMetric(name string, model LLM, options []JudgeOption, factory func(LLM, judge.Options) judge.Metric) Metric {
	cfg, err := judgeOptions(options)
	if err == nil && (cfg.expected || cfg.logProbs) {
		err = fmt.Errorf("goeval: AgainstExpected and LogProbs apply only to custom Judge metrics")
	}
	return builtinMetric{metric: factory(model, cfg.options), name: name, model: model, requiresModel: true, err: err}
}
func gated(name string, metric Metric, threshold float64) Metric {
	gate, err := WithThreshold(metric, threshold)
	return builtinMetric{metric: gate, name: name, err: err}
}

// ExactMatch passes when Output equals Expected, including whitespace.
func ExactMatch() Metric { return gated("exact_match", scorers.ExactMatch(), 1) }

// NormalizedMatch trims surrounding whitespace before comparing outputs.
func NormalizedMatch() Metric {
	return builtinMetric{metric: judge.ExactMatchMetric(judge.Options{}), name: "normalized_match"}
}

// Contains passes when Output contains Expected as a substring.
func Contains() Metric { return gated("contains", scorers.StringPresence(), 1) }

// StringSimilarity uses normalized Levenshtein similarity with threshold 0.5.
func StringSimilarity() Metric {
	return gated("string_similarity", scorers.NonLLMStringSimilarity(scorers.Levenshtein), .5)
}

// SemanticSimilarity uses cosine similarity of Output and Expected embeddings.
func SemanticSimilarity(embedder Embedder) Metric {
	m := gated("semantic_similarity", scorers.SemanticSimilarity(embedder), .5).(builtinMetric)
	m.embedder = embedder
	m.requiresEmbeddings = true
	return m
}

// Faithfulness judges output claims against truths extracted from Context.
func Faithfulness(model LLM, options ...JudgeOption) Metric {
	return modelMetric("faithfulness", model, options, judge.FaithfulnessMetric)
}

// AnswerRelevance judges whether output statements address Input.
func AnswerRelevance(model LLM, options ...JudgeOption) Metric {
	return modelMetric("answer_relevance", model, options, judge.AnswerRelevancyMetric)
}

// ContextPrecision measures ranked retrieval relevance using Expected.
func ContextPrecision(model LLM, options ...JudgeOption) Metric {
	return modelMetric("context_precision", model, options, judge.ContextualPrecisionMetric)
}

// ContextRecall measures how much of Expected is supported by Context.
func ContextRecall(model LLM, options ...JudgeOption) Metric {
	return modelMetric("context_recall", model, options, judge.ContextualRecallMetric)
}

// ContextRelevance judges how much of Context is relevant to Input.
func ContextRelevance(model LLM, options ...JudgeOption) Metric {
	return modelMetric("context_relevance", model, options, judge.ContextualRelevancyMetric)
}

// Hallucination measures agreement with Sources; 1 means fully aligned.
func Hallucination(model LLM, options ...JudgeOption) Metric {
	return modelMetric("hallucination", model, options, judge.HallucinationMetric)
}

// Bias scores freedom from biased opinions; 1 means no detected bias.
func Bias(model LLM, options ...JudgeOption) Metric {
	return modelMetric("bias", model, options, judge.BiasMetric)
}

// Toxicity scores freedom from toxic opinions; 1 means no detected toxicity.
func Toxicity(model LLM, options ...JudgeOption) Metric {
	return modelMetric("toxicity", model, options, judge.ToxicityMetric)
}

// Summarization measures alignment and coverage. Input is the original text;
// Output is the summary. The score is the minimum of the two components.
func Summarization(model LLM, options ...JudgeOption) Metric {
	return modelMetric("summarization", model, options, func(m LLM, o judge.Options) judge.Metric {
		return judge.SummarizationMetric(m, judge.SummarizationOptions{Options: o})
	})
}

// TaskCompletion judges completion from the input, output and called tools.
func TaskCompletion(model LLM, options ...JudgeOption) Metric {
	return modelMetric("task_completion", model, options, func(m LLM, o judge.Options) judge.Metric {
		return judge.TaskCompletionMetric(m, judge.TaskCompletionOptions{Options: o})
	})
}

// ArgumentCorrectness judges whether tool arguments match the user's request.
func ArgumentCorrectness(model LLM, options ...JudgeOption) Metric {
	return modelMetric("argument_correctness", model, options, judge.ArgumentCorrectnessMetric)
}

// ToolCorrectness compares actual and expected tool names without model calls.
// Use the advanced metric package for argument/output matching or ordered modes.
func ToolCorrectness() Metric {
	return builtinMetric{name: "tool_correctness", metric: judge.ToolCorrectnessMetric(nil, judge.ToolCorrectnessOptions{})}
}

// PromptAlignment judges adherence to the supplied instructions.
func PromptAlignment(model LLM, instructions []string, options ...JudgeOption) Metric {
	m := modelMetric("prompt_alignment", model, options, func(m LLM, o judge.Options) judge.Metric {
		return judge.PromptAlignmentMetric(m, instructions, o)
	}).(builtinMetric)
	if len(instructions) == 0 {
		m.err = fmt.Errorf("goeval: prompt alignment needs instructions")
	}
	return m
}

// Judge creates a custom G-Eval criterion using Input and Output. AgainstExpected
// also includes the reference answer. Steps are generated once and reused.
func Judge(model LLM, name, criteria string, options ...JudgeOption) Metric {
	cfg, err := judgeOptions(options)
	params := []judge.SingleTurnParam{judge.Input, judge.ActualOutput}
	if cfg.expected {
		params = append(params, judge.ExpectedOutput)
	}
	metric, buildErr := judge.GEval(model, judge.GEvalOptions{Options: cfg.options, Name: name, Criteria: criteria, EvaluationParams: params, UseLogProbs: judge.Bool(cfg.logProbs)})
	if err == nil {
		err = buildErr
	}
	return builtinMetric{metric: metric, name: name, model: model, requiresModel: true, err: err}
}
