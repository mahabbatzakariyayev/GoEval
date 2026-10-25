package prompt

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func TestTypedPromptRoundTripAndValidation(t *testing.T) {
	type output struct {
		Value int `json:"value"`
	}
	p := Prompt[string, output]{Name: "test", Instruction: "judge", Schema: map[string]any{"type": "object"}, Examples: []Example[string, output]{{"example", output{1}}}}
	var b bytes.Buffer
	if err := p.WriteJSON(&b); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadJSON[string, output](&b)
	if err != nil {
		t.Fatal(err)
	}
	llm := goeval.LLMFunc(func(_ context.Context, p goeval.Prompt) (string, error) {
		if !strings.Contains(p.Instruction, "example") {
			t.Error("few shot example missing")
		}
		return `{"value":2}`, nil
	})
	out, err := loaded.Generate(context.Background(), llm, "input", func(o output) error {
		if o.Value != 2 {
			return fmt.Errorf("bad value")
		}
		return nil
	})
	if err != nil || out.Value != 2 {
		t.Fatalf("%+v %v", out, err)
	}
	if _, err := loaded.Generate(context.Background(), llm, "input", func(output) error { return fmt.Errorf("rejected") }); err == nil {
		t.Fatal("validator ignored")
	}
}
func TestRenderRejectsMissingKeys(t *testing.T) {
	if _, err := Render("Answer {{.question}}", map[string]string{}); err == nil {
		t.Fatal("missing key accepted")
	}
	out, err := Render("Answer {{.question}}", map[string]string{"question": "why?"})
	if err != nil || out != "Answer why?" {
		t.Fatalf("%q %v", out, err)
	}
}
func TestPythonFormatCompatibility(t *testing.T) {
	input := map[string]any{"response": "Paris", "context": []string{"France", "Europe"}, "data": map[string]any{"answer": "Paris"}}
	got, err := Format(`Judge {response!r}; {context[1]}; {data.answer}; {{"value": 1}}`, input)
	if err != nil || got != `Judge 'Paris'; Europe; Paris; {"value": 1}` {
		t.Fatalf("%q %v", got, err)
	}
	for _, source := range []string{"{missing}", "{response", "response}", "{response:.2f}", "{context[8]}", "{response!x}"} {
		if _, err := Format(source, input); err == nil {
			t.Errorf("accepted invalid format %q", source)
		}
	}
}
