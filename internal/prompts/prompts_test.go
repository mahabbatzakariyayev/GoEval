package prompts

import (
	"encoding/json"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestPinnedBundle(t *testing.T) {
	if bundle.Revision != goeval.UpstreamRevision {
		t.Fatal("prompt/code baseline mismatch")
	}
	if len(bundle.Prompts) != 30 {
		t.Fatal("upstream prompt inventory changed")
	}
	for key, raw := range bundle.Prompts {
		var d Definition
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		if d.Source == "" {
			t.Fatalf("missing source for %s", key)
		}
		if strings.HasPrefix(key, "nv_metrics.") {
			continue
		}
		Instruction(key)
	}
}
func TestClaimExamplesAndRawTemplates(t *testing.T) {
	low := InstructionVariant("factual_correctness.ClaimDecompositionPrompt", "low_atomicity_low_coverage")
	high := InstructionVariant("factual_correctness.ClaimDecompositionPrompt", "high_atomicity_high_coverage")
	if low == high || !strings.Contains(high, "Charles Babbage") {
		t.Fatal("claim decomposition examples missing")
	}
	if !strings.Contains(Template("nv_metrics.AnswerAccuracy", "template_accuracy1"), "{sentence_inference}") {
		t.Fatal("upstream NVIDIA template changed")
	}
}
