// Command compile-prompts compiles the original templates in a prompt snapshot.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mahabbatzakariyayev/GoEval/internal/judgetemplates"
)

func main() {
	path := flag.String("snapshot", "internal/judgetemplates/data.json", "prompt snapshot to compile")
	flag.Parse()
	if err := compile(*path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func compile(path string) error {
	type definition struct {
		Original string `json:"original"`
		Compiled string `json:"compiled"`
	}
	var snapshot struct {
		Revision  string                `json:"revision"`
		Templates map[string]definition `json:"templates"`
		Fragments map[string]string     `json:"fragments"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	if len(snapshot.Templates) == 0 {
		return fmt.Errorf("snapshot has no templates")
	}
	for key, value := range snapshot.Templates {
		compiled, err := judgetemplates.CompileSource(value.Original)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		value.Compiled = compiled
		snapshot.Templates[key] = value
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		return err
	}
	if err := os.WriteFile(path, out.Bytes(), 0644); err != nil {
		return err
	}
	fmt.Printf("Compiled %d templates\n", len(snapshot.Templates))
	return nil
}
