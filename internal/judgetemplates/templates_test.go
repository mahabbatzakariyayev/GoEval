package judgetemplates

import (
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestSourceCompilationMatchesEveryPinnedPrompt(t *testing.T) {
	if bundle.Revision != goeval.JudgeRevision {
		t.Fatal("revision mismatch")
	}
	for key, p := range bundle.Templates {
		t.Run(key, func(t *testing.T) {
			compiled, err := CompileSource(p.Original)
			if err != nil || compiled != p.Compiled {
				t.Fatalf("conversion differs from pinned source: %v", err)
			}
		})
	}
}

func TestTemplateRendersSourceRepresentation(t *testing.T) {
	out, err := Render("AnswerRelevancyMetric.generate_verdicts", map[string]any{"input": "question", "statements": []string{"Paris", `a "quote"`}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out, "Input:\nquestion\n\nStatements:\n['Paris', 'a \"quote\"']\n\nJSON:") {
		t.Fatal(out)
	}
	if strings.Contains(out, "{%") || strings.Contains(out, "{{") {
		t.Fatal("unrendered expression")
	}
	if _, err := Render("AnswerRelevancyMetric.generate_verdicts", map[string]any{}); err == nil {
		t.Fatal("missing required variables accepted")
	}
	if _, err := CompileSource(`{% for item in list %}{{ item }}{% endfor %}`); err == nil {
		t.Fatal("unsupported language silently accepted")
	}
}
