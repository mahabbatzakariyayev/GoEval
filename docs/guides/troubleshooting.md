# Troubleshooting

Start by separating an execution failure from a failed quality gate. The
returned error, report.Err(), row Errors, and score reasons answer different
parts of that question.

## Evaluate returned nil error, but Passed is false

Scoring can complete successfully while a score is below its threshold.
Inspect Rows and each score's Success, Threshold, Value, and Reason.

Passed also requires a nonempty dataset and a gate on every metric.
Wrap score-only custom metrics with WithThreshold. An older rows/means-only
saved report has no record of all requested metrics and cannot establish that
every gate ran.

## A required field is missing

Check the selected metric's inputs. Faithfulness needs Context. Hallucination
needs Sources. Retrieval precision and recall need Expected as well as Context.
Use the [metric selection table](metrics.md).

JSON readers distinguish absent/null scalar fields from supplied empty strings.
An omitted Expected is different from an explicitly supplied empty reference.
A nil Context is different from an empty nonnil slice.

## A model judgment is rejected

The model must return the expected JSON result without surrounding prose.
Missing fields, unknown verdicts, empty extracted items, and incomplete judgment
counts are errors.

Confirm that your endpoint supports the selected response mode. StructuredOutput
requires JSON-schema support; ordinary mode requests a JSON object. Try a small
case and inspect the provider response in your own controlled test environment.

A missing or partial judgment is an execution failure, not evidence for a
passing score.

## A provider returns 401, 404, or 429

Check the API key, base URL including any API-root prefix, and configured model
name. The model must be available at the chosen endpoint.

A 429 response indicates rate limiting. Lower concurrency and use bounded retry
settings. Retry settings apply to metric attempts and can repeat several model
stages.

Use errors.As to inspect `*openai.HTTPError` from the providers/openai package.
The live error retains its HTTP status. Error response bodies redact the
configured API key before being returned.

## A run times out

WithTimeout applies to one metric attempt. A metric can involve multiple HTTP
calls. Use a context deadline for a total run limit.

Custom providers and scorers must cooperate with cancellation. A callback that
blocks indefinitely without checking its context can keep the run blocked.

## A custom metric does not make Passed true

A numeric score alone has no passing decision. Add a finite minimum threshold
with WithThreshold. Ensure the metric name is nonempty and unique.

The callback must return finite values. A nil MetricFunc callback and invalid
known metric configurations fail preflight before model calls.

## The optional CLI rejects a friendly dataset

The CLI uses the lower-level Dataset field names, including response and
reference. Friendly datasets use output and expected.

Use the library ReadCasesJSONL API and [dataset example](datasets.md), or convert
Cases to Samples before writing a lower-level dataset.

## The install command cannot find the package

Use the exact module path `github.com/mahabbatzakariyayev/GoEval`.
Before a source version is published, use a local replace directive pointing
to the GoEval checkout. See [getting started](getting-started.md).
