package scorers

import (
	"context"
	"fmt"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
	"github.com/mahabbatzakariyayev/GoEval/internal/prompts"
)

// AnswerCorrectness uses upstream's TP/FP/FN classifier, which is deliberately
// distinct from FactualCorrectness's bidirectional NLI verification.
func answerFactuality(ctx context.Context, llm goeval.LLM, s goeval.Sample, beta *float64) (float64, error) {
	answer, err := decompose(ctx, llm, s.UserInput, s.Response)
	if err != nil {
		return 0, err
	}
	reference, err := decompose(ctx, llm, s.UserInput, s.Reference)
	if err != nil {
		return 0, err
	}
	if len(answer) == 0 && len(reference) == 0 {
		return 1, nil
	}
	type item struct {
		Statement string `json:"statement"`
		Reason    string `json:"reason"`
	}
	var out struct {
		TP *[]item `json:"TP"`
		FP *[]item `json:"FP"`
		FN *[]item `json:"FN"`
	}
	items := arraySchema(objectSchema(map[string]any{"statement": stringSchema(), "reason": stringSchema()}))
	err = goeval.GenerateJSON(ctx, llm, goeval.Prompt{Instruction: prompts.Instruction("answer_correctness.CorrectnessClassifier"), Input: map[string]any{"question": s.UserInput, "answer": answer, "ground_truth": reference}, Schema: objectSchema(map[string]any{"TP": items, "FP": items, "FN": items})}, &out)
	if err != nil {
		return 0, err
	}
	if out.TP == nil || out.FP == nil || out.FN == nil {
		return 0, fmt.Errorf("metrics: incomplete TP/FP/FN classification")
	}
	return factualValue(len(*out.TP), len(*out.FP), len(*out.FN), FactualOptions{Beta: beta})
}
