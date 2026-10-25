// Command check-docs compiles complete README examples in separate Go modules.
// Model examples are built without executing model requests.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func main() {
	if err := check(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "goeval-docs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	blocks := regexp.MustCompile("(?s)```go\\r?\\n(.*?)\\r?\\n```").FindAllSubmatch(readme, -1)
	count := 0
	for _, block := range blocks {
		source := strings.TrimSpace(string(block[1]))
		if !strings.HasPrefix(source, "package ") {
			continue
		}
		count++
		directory := filepath.Join(temporary, fmt.Sprintf("example%d", count))
		if err := os.Mkdir(directory, 0700); err != nil {
			return err
		}
		module := fmt.Sprintf("module example.com/goeval-doc%d\n\ngo 1.23\n\nrequire github.com/mahabbatzakariyayev/GoEval v0.0.0\n\nreplace github.com/mahabbatzakariyayev/GoEval => %q\n", count, filepath.ToSlash(root))
		if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module), 0600); err != nil {
			return err
		}
		name := "main.go"
		args := []string{"build", "-o", filepath.Join(directory, "example.bin"), "."}
		if strings.HasPrefix(source, "package evaluation_test") {
			name = "example_test.go"
			args = []string{"test", "-race", "."}
		}
		if err := os.WriteFile(filepath.Join(directory, name), []byte(source), 0600); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		cmd := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), args...)
		cmd.Dir = directory
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("README example %d: %w\n%s", count, err, output)
		}
	}
	if count == 0 {
		return fmt.Errorf("README has no complete Go examples")
	}
	fmt.Printf("Validated %d complete README examples from separate Go modules\n", count)
	return nil
}
