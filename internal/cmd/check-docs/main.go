// Command check-docs compiles complete README and guide examples in separate Go modules.
// Model examples are built without executing model requests.
package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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
	files := []string{"README.md"}
	if err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".md" {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, relative)
		}
		return nil
	}); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "goeval-docs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	fence := regexp.MustCompile("(?s)```go\\r?\\n(.*?)\\r?\\n```")
	count := 0
	for _, file := range files {
		markdown, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return err
		}
		for blockNumber, block := range fence.FindAllSubmatch(markdown, -1) {
			source := strings.TrimSpace(string(block[1]))
			if !strings.HasPrefix(source, "package ") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.AllErrors)
			if err != nil {
				return err
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
			isTest := strings.HasSuffix(parsed.Name.Name, "_test")
			for _, declaration := range parsed.Decls {
				if function, ok := declaration.(*ast.FuncDecl); ok && strings.HasPrefix(function.Name.Name, "Test") {
					isTest = true
				}
			}
			if isTest {
				name = "example_test.go"
				args = []string{"test", "-c", "-o", filepath.Join(directory, "example.test"), "."}
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
				return fmt.Errorf("%s Go block %d: %w\n%s", file, blockNumber+1, err, output)
			}
		}
	}
	if count == 0 {
		return fmt.Errorf("documentation has no complete Go examples")
	}
	fmt.Printf("Validated %d complete README and guide examples from separate Go modules\n", count)
	return nil
}
