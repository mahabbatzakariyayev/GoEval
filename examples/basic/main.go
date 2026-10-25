package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	goeval "github.com/mahabbatzakariyayev/GoEval"
	"github.com/mahabbatzakariyayev/GoEval/scorers"
)

func main() {
	dataset := goeval.Dataset{{UserInput: "What is the capital of France?", Response: "Paris", Reference: "Paris"}, {UserInput: "What is the capital of Italy?", Response: "Milan", Reference: "Rome"}}
	result, err := goeval.Evaluate(context.Background(), dataset, []goeval.Metric{scorers.ExactMatch(), scorers.RougeScore(scorers.RougeOptions{})}, goeval.RunConfig{})
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		log.Fatal(err)
	}
}
