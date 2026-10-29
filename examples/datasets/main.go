package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/mahabbatzakariyayev/GoEval"
)

func main() {
	input := flag.String("input", "examples/data/cases.jsonl", "JSONL cases to evaluate")
	flag.Parse()
	file, err := os.Open(*input)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	cases, err := goeval.ReadCasesJSONL(file)
	if err != nil {
		log.Fatal(err)
	}
	evaluator, err := goeval.New(goeval.WithMetrics(goeval.ExactMatch(), goeval.NormalizedMatch()))
	if err != nil {
		log.Fatal(err)
	}
	report, runErr := evaluator.Evaluate(context.Background(), cases)
	if report != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			log.Fatal(err)
		}
	}
	if runErr != nil {
		log.Fatal(runErr)
	}
}
