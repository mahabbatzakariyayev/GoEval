package goeval

import (
	"context"
	"io"

	"github.com/mahabbatzakariyayev/GoEval/internal/core"
)

// Shared types also support the lower-level dataset and metric packages.
type (
	Field             = core.Field
	Sample            = core.Sample
	SingleTurnSample  = core.SingleTurnSample
	MultiTurnSample   = core.MultiTurnSample
	Dataset           = core.Dataset
	EvaluationDataset = core.EvaluationDataset
	Message           = core.Message
	ToolCall          = core.ToolCall
	Metric            = core.Metric
	MetricFunc        = core.MetricFunc
	Score             = core.Score
	MetricResult      = core.MetricResult
	RunConfig         = core.RunConfig
	RowResult         = core.RowResult
	EvaluationResult  = core.EvaluationResult
	EvaluationError   = core.EvaluationError
	MissingFieldError = core.MissingFieldError
	PanicError        = core.PanicError
	ThresholdError    = core.ThresholdError
	TestingT          = core.TestingT
	Prompt            = core.Prompt
	LLM               = core.LLM
	LLMFunc           = core.LLMFunc
	Embedder          = core.Embedder
	EmbedderFunc      = core.EmbedderFunc
	LogProbLLM        = core.LogProbLLM
	Completion        = core.Completion
	TokenLogProb      = core.TokenLogProb
	TokenAlternative  = core.TokenAlternative
)

const (
	UserInput           = core.UserInput
	Response            = core.Response
	Reference           = core.Reference
	RetrievedContexts   = core.RetrievedContexts
	ReferenceContexts   = core.ReferenceContexts
	RetrievedContextIDs = core.RetrievedContextIDs
	ReferenceContextIDs = core.ReferenceContextIDs
	Messages            = core.Messages
	ReferenceToolCalls  = core.ReferenceToolCalls
	ReferenceTopics     = core.ReferenceTopics
	Rubrics             = core.Rubrics
	Version             = core.Version
	UpstreamRevision    = core.UpstreamRevision
	ScorerRevision      = core.ScorerRevision
	JudgeRevision       = core.JudgeRevision
)

// ErrUndefined represents a score with no defined value for the supplied input.
var ErrUndefined = core.ErrUndefined

// Evaluate runs the lower-level sample API. For friendly Case fields and a
// reusable evaluator, use New and Evaluator.Evaluate.
func Evaluate(ctx context.Context, data Dataset, metrics []Metric, cfg RunConfig) (*EvaluationResult, error) {
	return core.Evaluate(ctx, data, metrics, cfg)
}

// DefaultRunConfig returns the compatibility runner defaults. New uses one
// attempt and the caller's deadline unless configured otherwise.
func DefaultRunConfig() RunConfig { return core.DefaultRunConfig() }

// ValidateMetrics checks configuration without making model calls.
func ValidateMetrics(metrics []Metric) error { return core.ValidateMetrics(metrics) }

// WithThreshold adds a minimum passing score to any metric.
func WithThreshold(metric Metric, threshold float64) (Metric, error) {
	return core.WithThreshold(metric, threshold)
}

// Check evaluates one sample and reports execution or quality-gate failures.
func Check(ctx context.Context, s Sample, metrics []Metric, cfg RunConfig) (*EvaluationResult, error) {
	return core.Check(ctx, s, metrics, cfg)
}

// AssertTest reports failed sample checks through Go's testing interface.
func AssertTest(t TestingT, ctx context.Context, s Sample, metrics []Metric) {
	t.Helper()
	core.AssertTest(t, ctx, s, metrics)
}

// GenerateJSON requests and decodes a nonnull JSON result from a model.
func GenerateJSON(ctx context.Context, llm LLM, p Prompt, out any) error {
	return core.GenerateJSON(ctx, llm, p, out)
}

// ReadJSON reads a dataset using compatibility sample field names.
func ReadJSON(r io.Reader) (Dataset, error) { return core.ReadJSON(r) }

// ReadJSONL reads one compatibility sample per nonblank line.
func ReadJSONL(r io.Reader) (Dataset, error) { return core.ReadJSONL(r) }

// ReadCSV reads compatibility samples and optionally remaps headers.
func ReadCSV(r io.Reader, columns map[string]string) (Dataset, error) {
	return core.ReadCSV(r, columns)
}

// PythonString renders the representation used by source-compatible templates.
func PythonString(value any) string { return core.PythonString(value) }

// PythonRepr renders quoted source-compatible template values.
func PythonRepr(value any) string { return core.PythonRepr(value) }
