# Caching and performance

Control request volume and execution time explicitly. An individual metric may
make several model requests, and model limits apply across concurrent cases.

## Set bounded concurrency

WithConcurrency controls the number of cases evaluated at once. The default is
16. Start lower when your endpoint has a small rate limit or your metrics use
multiple model stages.

Use the caller's context for an overall deadline and WithTimeout for each metric
attempt. Providers and custom metrics must honor cancellation; GoEval cannot
interrupt a callback that ignores its context.

## Configure retry attempts

The zero RunConfig uses one attempt. MaxRetries is the total number of attempts,
not the number of additional retries. Setting it to 2 allows at most two attempts
at scoring a metric.

RetryDelay sets a fixed delay. MaxWait enables capped exponential jitter when
RetryDelay is zero. Retries can repeat complete model pipelines and incur
additional requests.

DefaultRunConfig explicitly selects a different profile: 16 workers, ten total
attempts, 180 seconds per attempt, a 60-second retry cap, and seed 42. The seed
controls retry jitter; it does not make external model sampling deterministic.

## Cache model responses

Wrap a configured model with an explicit cache namespace:

```go
store, err := cache.NewMemory(256)
if err != nil {
    return err
}
cached := cache.LLM(client, store, "provider:model:temperature:v1")
metric := goeval.Faithfulness(cached)
```

Import cache from `github.com/mahabbatzakariyayev/GoEval/cache`. The fragment
assumes a configured client and a surrounding function. Include provider,
model, generation settings, and a version in the namespace. Change it when those
inputs change.

The memory cache is an LRU with a fixed entry count. It is safe for concurrent
access. Simultaneous cache misses can issue duplicate provider requests.

## Cache embeddings or use disk storage

Use `cache.Embeddings(embedder, store, namespace)` for embedding results.
Use `cache.NewDisk(directory)` for atomic disk caching.

The disk store has no automatic expiry or size limit. The directory can contain
evaluation inputs and model responses, so choose a location appropriate for
your data and manage its lifetime.

## Reuse evaluators

Reuse an Evaluator when its metrics and providers support concurrent calls.
Each run has its own results. Custom Judge instances also reuse generated
evaluation steps.

Do not mutate case slices, tool arguments, metadata, or nested configuration
containers during execution. See [detailed behavior](../usage.md#concurrency-and-retries).
