// Package docsite builds the static documentation website using the standard library.
package docsite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const repository = "https://github.com/mahabbatzakariyayev/GoEval"

type Page struct {
	Title, Group, Source, Output, Description string
}

type Heading struct {
	Text, ID string
	Level    int
}
type navigation struct {
	Title string
	Pages []Page
}
type view struct {
	Page
	Content          template.HTML
	Headings         []Heading
	Groups           []navigation
	Previous, Next   *Page
	Version, EditURL string
	Home             bool
}
type searchEntry struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Text        string `json:"text"`
}

// Build renders the page manifest and assets into an output directory.
func Build(root, output, version string) error {
	manifest, err := os.ReadFile(filepath.Join(root, "website", "pages.json"))
	if err != nil {
		return err
	}
	var pages []Page
	if err := json.Unmarshal(manifest, &pages); err != nil {
		return err
	}
	if len(pages) == 0 {
		return fmt.Errorf("documentation manifest is empty")
	}
	links := make(map[string]string, len(pages))
	outputs := make(map[string]bool, len(pages))
	groups := []navigation{}
	for _, p := range pages {
		if p.Title == "" || p.Group == "" || !filepath.IsLocal(p.Source) || path.Base(p.Output) != p.Output || !strings.HasSuffix(p.Output, ".html") || outputs[p.Output] || links[p.Source] != "" {
			return fmt.Errorf("invalid or duplicate documentation page: %+v", p)
		}
		links[p.Source], outputs[p.Output] = p.Output, true
		if len(groups) == 0 || groups[len(groups)-1].Title != p.Group {
			groups = append(groups, navigation{Title: p.Group})
		}
		groups[len(groups)-1].Pages = append(groups[len(groups)-1].Pages, p)
	}
	t, err := template.ParseFiles(filepath.Join(root, "website", "template.html"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(output, "assets"), 0755); err != nil {
		return err
	}
	index := make([]searchEntry, 0, len(pages))
	for i, p := range pages {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.Source)))
		if err != nil {
			return err
		}
		resolve := func(target string) (string, error) {
			u, err := url.Parse(target)
			if err != nil {
				return "", err
			}
			if u.IsAbs() {
				if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "mailto" {
					return "", fmt.Errorf("unsupported link scheme %q", u.Scheme)
				}
				return target, nil
			}
			if u.Host != "" {
				return "", fmt.Errorf("protocol-relative links are not supported")
			}
			if u.Path == "" {
				return target, nil
			}
			source := path.Clean(path.Join(path.Dir(p.Source), u.Path))
			if mapped := links[source]; mapped != "" {
				u.Path = mapped
				return u.String(), nil
			}
			if !filepath.IsLocal(source) {
				return "", fmt.Errorf("link leaves repository: %s", target)
			}
			info, err := os.Stat(filepath.Join(root, filepath.FromSlash(source)))
			if err != nil {
				return "", fmt.Errorf("broken link in %s: %s", p.Source, target)
			}
			kind := "/blob/main/"
			if info.IsDir() {
				kind = "/tree/main/"
			}
			u.Path = source
			return repository + kind + u.String(), nil
		}
		content, headings, err := renderMarkdown(string(data), resolve)
		if err != nil {
			return fmt.Errorf("%s: %w", p.Source, err)
		}
		v := view{Page: p, Content: template.HTML(content), Headings: headings, Groups: groups, Version: version, EditURL: repository + "/edit/main/" + p.Source, Home: p.Output == "index.html"}
		if i > 0 {
			v.Previous = &pages[i-1]
		}
		if i+1 < len(pages) {
			v.Next = &pages[i+1]
		}
		var rendered bytes.Buffer
		if err := t.Execute(&rendered, v); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(output, p.Output), rendered.Bytes(), 0644); err != nil {
			return err
		}
		index = append(index, searchEntry{Title: p.Title, Description: p.Description, URL: p.Output, Text: string(data)})
	}
	search, err := json.Marshal(index)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(output, "search.json"), search, 0644); err != nil {
		return err
	}
	for _, name := range []string{"style.css", "app.js", "favicon.svg"} {
		data, err := os.ReadFile(filepath.Join(root, "website", "assets", name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(output, "assets", name), data, 0644); err != nil {
			return err
		}
	}
	for _, name := range []string{"LICENSE", "NOTICE"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(output, name), data, 0644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(output, ".nojekyll"), nil, 0644); err != nil {
		return err
	}
	return validateLinks(output, pages)
}
