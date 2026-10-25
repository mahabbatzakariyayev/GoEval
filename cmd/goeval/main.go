// Command goeval evaluates saved GoEval datasets and generates synthetic samples.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/providers/openai"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
	"github.com/mahabbatzakariyayev/GoEval/testset"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "goeval:", err)
		os.Exit(1)
	}
}

func registry(c *openai.Client) map[string]goeval.Metric {
	return map[string]goeval.Metric{
		"exact_match": scorers.ExactMatch(), "string_present": scorers.StringPresence(), "non_llm_string_similarity": scorers.NonLLMStringSimilarity(scorers.Levenshtein), "bleu_score": scorers.BleuScore(scorers.BLEUOptions{}), "rouge_score": scorers.RougeScore(scorers.RougeOptions{}), "chrf_score": scorers.ChrfScore(scorers.ChrFOptions{}),
		"faithfulness": scorers.Faithfulness(c), "answer_relevancy": scorers.AnswerRelevancy(c, c, 0), "answer_correctness": scorers.AnswerCorrectness(c, c, scorers.AnswerCorrectnessOptions{}), "factual_correctness": scorers.FactualCorrectness(c, scorers.FactualOptions{}), "semantic_similarity": scorers.SemanticSimilarity(c), "answer_similarity": scorers.AnswerSimilarity(c),
		"context_precision": scorers.ContextPrecision(c), "context_utilization": scorers.ContextUtilization(c), "context_recall": scorers.ContextRecall(c), "context_entity_recall": scorers.ContextEntityRecall(c), "id_based_context_precision": scorers.IDBasedContextPrecision(), "id_based_context_recall": scorers.IDBasedContextRecall(), "non_llm_context_precision_with_reference": scorers.NonLLMContextPrecisionWithReference(scorers.ContextMatchOptions{}), "non_llm_context_recall": scorers.NonLLMContextRecall(scorers.ContextMatchOptions{}),
		"tool_call_accuracy": scorers.ToolCallAccuracy(scorers.ToolAccuracyOptions{}), "tool_call_f1": scorers.ToolCallF1(), "agent_goal_accuracy": scorers.AgentGoalAccuracyWithReference(c), "topic_adherence": scorers.TopicAdherence(c, "f1"), "llm_sql_equivalence_with_reference": scorers.LLMSQLEquivalence(c), "summary_score": scorers.SummarizationScore(c, scorers.SummaryOptions{}), "noise_sensitivity": scorers.NoiseSensitivity(c, "relevant"),
		"nv_accuracy": scorers.AnswerAccuracy(c), "nv_context_relevance": scorers.ContextRelevance(c), "nv_response_groundedness": scorers.ResponseGroundedness(c), "harmfulness": scorers.Harmfulness(c), "maliciousness": scorers.Maliciousness(c), "coherence": scorers.Coherence(c), "correctness": scorers.Correctness(c), "conciseness": scorers.Conciseness(c),
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: goeval {evaluate|generate|metrics|version} [flags]")
	}
	if args[0] == "version" {
		_, err := fmt.Fprintf(stdout, "goeval %s\n", goeval.Version)
		return err
	}
	if args[0] == "metrics" {
		names := []string{}
		for name := range registry(nil) {
			names = append(names, name)
		}
		sort.Strings(names)
		_, err := fmt.Fprintln(stdout, strings.Join(names, "\n"))
		return err
	}
	if args[0] != "evaluate" && args[0] != "generate" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.SetOutput(stderr)
	input := f.String("input", "-", "input path, or - for stdin")
	output := f.String("output", "-", "output path, or - for stdout")
	format := f.String("format", "auto", "dataset input: auto, json, jsonl, csv")
	names := f.String("metrics", "exact_match", "comma-separated metric names")
	model := f.String("model", os.Getenv("OPENAI_MODEL"), "chat model")
	embedding := f.String("embedding-model", os.Getenv("OPENAI_EMBEDDING_MODEL"), "embedding model")
	base := f.String("base-url", os.Getenv("OPENAI_BASE_URL"), "OpenAI-compatible base URL")
	workers := f.Int("workers", 16, "concurrent rows")
	timeout := f.Duration("timeout", 180*time.Second, "timeout per operation")
	attempts := f.Int("attempts", 1, "total scoring attempts")
	failFast := f.Bool("fail-fast", false, "stop after first metric failure")
	strict := f.Bool("strict-json", false, "request provider JSON schema mode")
	size := f.Int("size", 10, "generated sample count")
	seed := f.Uint64("seed", 42, "generation scenario seed")
	multi := f.Bool("multi-hop", false, "generate multi-hop specific questions")
	enrich := f.Bool("enrich", false, "extract graph summaries/entities")
	chunk := f.Int("chunk-size", 512, "chunk size in words")
	overlap := f.Int("chunk-overlap", 0, "overlap in words")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *workers < 0 || *timeout < 0 || *attempts < 1 {
		return fmt.Errorf("invalid workers, timeout or attempts")
	}
	client, err := openai.New(openai.Config{APIKey: os.Getenv("OPENAI_API_KEY"), BaseURL: *base, Model: *model, EmbeddingModel: *embedding, StructuredOutput: *strict})
	if err != nil {
		return err
	}
	reader := stdin
	if *input != "-" {
		file, err := os.Open(*input)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}
	var value any
	var runErr error
	if args[0] == "evaluate" {
		var dataset goeval.Dataset
		kind := *format
		if kind == "auto" {
			kind = strings.TrimPrefix(filepath.Ext(*input), ".")
			if *input == "-" {
				kind = "jsonl"
			}
		}
		switch kind {
		case "json":
			dataset, err = goeval.ReadJSON(reader)
		case "jsonl", "ndjson":
			dataset, err = goeval.ReadJSONL(reader)
		case "csv":
			dataset, err = goeval.ReadCSV(reader, nil)
		default:
			return fmt.Errorf("unsupported dataset format %q", kind)
		}
		if err != nil {
			return err
		}
		all := registry(client)
		selected := []goeval.Metric{}
		for _, name := range strings.Split(*names, ",") {
			name = strings.TrimSpace(name)
			m, ok := all[name]
			if !ok {
				return fmt.Errorf("unknown metric %q (run goeval metrics)", name)
			}
			selected = append(selected, m)
		}
		result, err := goeval.Evaluate(ctx, dataset, selected, goeval.RunConfig{MaxWorkers: *workers, Timeout: *timeout, MaxRetries: *attempts, FailFast: *failFast})
		if result == nil {
			return err
		}
		value, runErr = result, err
		if runErr == nil {
			for _, row := range result.Rows {
				if len(row.Errors) > 0 {
					runErr = fmt.Errorf("evaluation contains failed scores; see row errors")
					break
				}
			}
		}
	} else {
		if *model == "" {
			return fmt.Errorf("generation requires -model or OPENAI_MODEL")
		}
		var docs []testset.Document
		dec := json.NewDecoder(reader)
		if err := dec.Decode(&docs); err != nil {
			return err
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return fmt.Errorf("trailing document content")
		}
		var embedder goeval.Embedder
		if *embedding != "" {
			embedder = client
		}
		dist := []testset.Distribution{{Type: testset.SingleHop, Weight: 1}}
		if *multi {
			dist = []testset.Distribution{{Type: testset.MultiHopSpecific, Weight: 1}}
		}
		generationCtx := ctx
		cancel := func() {}
		if *timeout > 0 {
			generationCtx, cancel = context.WithTimeout(ctx, *timeout)
		}
		defer cancel()
		result, err := (testset.Generator{LLM: client, Embedder: embedder}).Generate(generationCtx, docs, testset.Config{Size: *size, Seed: *seed, MaxWorkers: *workers, Distribution: dist, Graph: testset.GraphConfig{ChunkSize: *chunk, ChunkOverlap: *overlap, Enrich: *enrich}})
		if result == nil {
			return err
		}
		value, runErr = result.Dataset(), err
	}
	// Marshal before opening the destination so serialization failures cannot
	// truncate an existing output file.
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if *output == "-" {
		if _, err := stdout.Write(data); err != nil {
			return err
		}
	} else if err := os.WriteFile(*output, data, 0600); err != nil {
		return err
	}
	return runErr
}
