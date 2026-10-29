package docsite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRepositoryDocumentation(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	if err := Build(root, output, "test-version"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, "search.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []searchEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) < 15 {
		t.Fatal("missing guides in search index", len(entries))
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(output, entry.URL))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "test-version") || !strings.Contains(string(data), `aria-current="page"`) {
			t.Fatalf("missing version or active navigation in %s", entry.URL)
		}
	}
	for _, name := range []string{"assets/style.css", "assets/app.js", "assets/favicon.svg", ".nojekyll", "LICENSE", "NOTICE"} {
		if _, err := os.Stat(filepath.Join(output, filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRendererEscapesContentAndPreservesCode(t *testing.T) {
	resolve := func(target string) (string, error) { return target, nil }
	markdown := "# Hello\n\n<script>alert(1)</script>\n\n## A heading\n\n## A heading\n\n" +
		"```go\nif a < b { return }\n```\n\n| Field | Value |\n| --- | --- |\n| `Output` | **Paris** |\n\n- One\n  continued\n- [Two](two.html)\n"
	out, headings, err := renderMarkdown(markdown, resolve)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<script>") || !strings.Contains(out, "&lt;script&gt;") || !strings.Contains(out, "a &lt; b") {
		t.Fatal("raw HTML or code was not escaped", out)
	}
	if len(headings) != 2 || headings[0].ID != "a-heading" || headings[1].ID != "a-heading-2" {
		t.Fatal(headings)
	}
	for _, expected := range []string{"<table>", "<strong>Paris</strong>", "<li>One continued</li>", `href="two.html"`, "copy-button"} {
		if !strings.Contains(out, expected) {
			t.Fatal("rendering lost", expected)
		}
	}
	if _, _, err := renderMarkdown("```go\nnot closed", resolve); err == nil {
		t.Fatal("unclosed code accepted")
	}
}

func TestBrokenAnchorsFailValidation(t *testing.T) {
	output := t.TempDir()
	file := filepath.Join(output, "index.html")
	if err := os.WriteFile(file, []byte(`<h1 id="intro">Intro</h1><a href="#missing">Broken</a>`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateLinks(output, []Page{{Output: "index.html"}}); err == nil {
		t.Fatal("missing anchor accepted")
	}
	if err := os.WriteFile(file, []byte(`<a href="missing.html">Broken</a>`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateLinks(output, []Page{{Output: "index.html"}}); err == nil {
		t.Fatal("missing page accepted")
	}
}
