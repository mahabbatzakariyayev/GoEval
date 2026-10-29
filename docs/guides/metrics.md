# Choose your metrics

Choose metrics from the behavior you want to measure, then check that your cases
contain the required evidence. An answer can be relevant yet factually wrong,
or faithful to retrieved context that does not answer the question.

## Start with the application question

| Application question | Metric | Evidence |
| --- | --- | --- |
| Did the answer match an exact expected value? | `ExactMatch` | Output and Expected |
| Is surrounding whitespace acceptable? | `NormalizedMatch` | Output and Expected |
| Does the answer contain required text? | `Contains` | Output and Expected |
| Are the texts close character by character? | `StringSimilarity` | Output and Expected |
| Are the answer and reference semantically similar? | `SemanticSimilarity` | Output, Expected, an embedder |
| Are answer claims supported by retrieved passages? | `Faithfulness` | Input, Output, Context, a model |
| Does the answer address the question? | `AnswerRelevance` | Input, Output, a model |
| Are useful retrieved passages ranked highly? | `ContextPrecision` | Input, Output, Expected, Context, a model |
| Does retrieval support the reference answer? | `ContextRecall` | Input, Output, Expected, Context, a model |
| Are retrieved statements relevant? | `ContextRelevance` | Input, Output, Context, a model |
| Does the answer agree with authoritative sources? | `Hallucination` | Input, Output, Sources, a model |
| Are biased or toxic opinions absent? | `Bias`, `Toxicity` | Input, Output, a model |

## Combine complementary metrics

For a retrieval-augmented application, faithfulness measures support in Context;
answer relevance measures whether the response addresses Input. Add retrieval
precision or recall when you also have reference answers.

Model-based metrics may make several requests per case. Start with a small,
representative dataset and inspect per-case reasons before scaling the run.

## Set passing thresholds

Exact match, normalized match, and containment require a score of 1 by default.
String similarity, semantic similarity, root tool correctness, and model judges
default to 0.5. All root built-ins use higher-is-better decisions.

Model judges accept `goeval.Threshold(0.8)`. `Strict()` requires a perfect score
and enables the metric's strict scoring mode. `WithoutReason()` skips optional
explanation requests. Use `goeval.WithThreshold(metric, minimum)` for a custom
numeric gate or a different threshold on a deterministic metric.

Thresholds should reflect the quality you need and the observed behavior of your
cases. Cosine similarity can be negative; advanced rubric and numeric scorers can
use scales outside zero to one.

## Interpret safety scores

For root `Hallucination`, `Bias`, and `Toxicity`, a higher score is better.
A hallucination score of 1 means the answer aligns with supplied Sources.
Bias and toxicity measure freedom from the flagged opinions.

These judges report model judgments. They do not establish that an application
is safe in every situation. Review the underlying evidence alongside aggregate
scores.

## Use specialized scorers

The `scorers` package includes BLEU, ROUGE, chrF, alternative string distances,
retrieval IDs, factual correctness, SQL equivalence, summaries, rubrics, and
agent metrics. These implement `goeval.Metric`.

The `judge` package exposes model-stage options, direct `Measure` calls,
probability-weighted criteria, and decision graphs. Its metric result names
start with `judge.`, allowing you to combine them with root built-ins.

See [API reference](../reference.md) and [coverage](../coverage.md) before choosing
a specialized workflow.
