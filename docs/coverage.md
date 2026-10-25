# Supported features

GoEval is a pre-release library. This page describes implemented behavior and
current limits; it does not promise that every evaluation workflow is supported.

| Area | Implemented | Verification and limits |
| --- | --- | --- |
| Friendly API | Cases, evaluator options, reusable evaluators, persistent reports, quality gates, Go test assertions | Offline defaults, HTTP pipeline/cache integration, typed errors, concurrent reuse, dataset/report round trips |
| Text scoring | Exact and normalized match, containment, Levenshtein, Hamming, Jaro, Jaro-Winkler, BLEU, ROUGE, chrF | Recorded numeric fixtures and Porter stemming cases; advanced tokenizer options are limited |
| Retrieval | Ranked precision, recall, entity recall, context relevance, ID sets, string matching | Reducer and staged prompt tests; additional recorded end-to-end cases remain useful |
| Answer quality | Faithfulness, relevance, correctness, factual correctness, noise sensitivity, semantic similarity | Multi-stage prompts and reducers; cosine can be negative; no claim of identical independent model judgments |
| Safety | Hallucination, bias, toxicity, aspect criteria | Root built-ins use higher-is-better scores; Sources is authoritative evidence for hallucination checks |
| Custom judges | Criteria, generated or supplied steps, rubric ranges, strict mode, optional probability weighting, decision graphs | Concurrency and branch validation tests; graphs have no persistence or remote upload |
| Agent evaluation | Tool selection, arguments, ordering, goals, topic adherence, task completion, prompt alignment | Deterministic tool fixtures and scripted judgments; no tracing SDK |
| Summaries and SQL | Two summary scoring pipelines; SQL explanation and equivalence judgments | SQL is judged rather than executed |
| Typed custom metrics | Numeric, discrete, ranking, rubric and aspect results | Interval/category/count validation; advanced prompt formatting and persistence are limited |
| Datasets | Friendly cases; lower-level single-turn samples and conversation arrays; JSON, JSONL, CSV | Absent fields, empty arrays, metadata and large numbers covered; conversation arrays do not imply conversational judges |
| Runner | Bounded concurrency, input ordering, cancellation, retries, per-attempt timeout, partial reports | Race tests and typed error checks; custom providers must honor contexts |
| Provider and cache | Compatible HTTP chat and embeddings, JSON schema/object modes, log probabilities, LRU and disk cache | Local HTTP servers and deterministic fixtures; live endpoints have not been validated |
| Experiments | Run application callbacks and evaluate successful outputs | Execution failures preserve causes and original row indices; no storage backend |
| Test data generation | Chunking, graph enrichment, scenarios, personas, single/multi-hop queries | Experimental; not a complete generation or optimization platform |

## Current limits

Multimodal image/audio evaluation, conversational judges and simulation,
classifier-backed metrics, dataframe comparison, schema correctness judges,
plan/step evaluation, arena ranking, optimization, tracing integrations,
remote persistence, token/cost accounting, and additional provider adapters are
not implemented comprehensively.

Go regular expressions use RE2 syntax. Backreferences and lookbehind are not
supported. Tool rendering, error representation, cache files, map ordering, and
some formatting/coercion rules follow Go conventions.

Model tests check prompt content, stages, input fields, and scoring reducers
using deterministic responses. Separate requests to a model can produce
different judgments. Provider compatibility should be checked with your chosen
endpoint and model before relying on a quality gate.

The source revision identifiers for embedded prompt snapshots remain recorded
in their JSON files and the exported ScorerRevision and JudgeRevision constants.
Copyright and adaptation notices are retained in [NOTICE](../NOTICE).
