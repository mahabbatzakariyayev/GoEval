package docsite

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var htmlID = regexp.MustCompile(`\bid="([^"]+)"`)
var htmlLink = regexp.MustCompile(`\b(?:href|src)="([^"]+)"`)

func validateLinks(output string, pages []Page) error {
	anchors := map[string]map[string]bool{}
	contents := map[string][]byte{}
	for _, p := range pages {
		data, err := os.ReadFile(filepath.Join(output, p.Output))
		if err != nil {
			return err
		}
		contents[p.Output] = data
		anchors[p.Output] = map[string]bool{}
		for _, id := range htmlID.FindAllSubmatch(data, -1) {
			key := string(id[1])
			if anchors[p.Output][key] {
				return fmt.Errorf("duplicate HTML id %s in %s", key, p.Output)
			}
			anchors[p.Output][key] = true
		}
	}
	for file, data := range contents {
		for _, match := range htmlLink.FindAllSubmatch(data, -1) {
			u, err := url.Parse(string(match[1]))
			if err != nil {
				return err
			}
			if u.IsAbs() {
				continue
			}
			target := u.Path
			if target == "" {
				target = file
			}
			if !filepath.IsLocal(target) {
				return fmt.Errorf("invalid local link %s in %s", target, file)
			}
			if _, err := os.Stat(filepath.Join(output, filepath.FromSlash(target))); err != nil {
				return fmt.Errorf("missing %s linked from %s", target, file)
			}
			if u.Fragment != "" && strings.HasSuffix(target, ".html") && !anchors[target][u.Fragment] {
				return fmt.Errorf("missing anchor %s#%s linked from %s", target, u.Fragment, file)
			}
		}
	}
	return nil
}
