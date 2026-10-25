package scorers

import (
	"regexp"
	"strings"
)

// mteval-v13a tokenization as used by sacrebleu (Apache-2.0; see NOTICE).
var tokenizerRules = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`([\{-\~\[-\x60 -\&\(-\+\:-\@/])`), " $1 "},
	{regexp.MustCompile(`([^0-9])([\.,])`), "$1 $2 "},
	{regexp.MustCompile(`([\.,])([^0-9])`), " $1 $2"},
	{regexp.MustCompile(`([0-9])(-)`), "$1 $2 "},
}

func tokenize13a(s string) []string {
	s = strings.ReplaceAll(s, "<skipped>", "")
	s = strings.ReplaceAll(s, "-\n", "")
	s = strings.ReplaceAll(s, "\n", " ")
	for _, pair := range [][2]string{{"&quot;", "\""}, {"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"}} {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	s = " " + s + " "
	for _, rule := range tokenizerRules {
		s = rule.re.ReplaceAllString(s, rule.replacement)
	}
	return strings.Fields(s)
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func chrfWords(s string) []string {
	const punctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"
	out := []string{}
	for _, word := range strings.Fields(s) {
		r := []rune(word)
		if len(r) > 1 && strings.ContainsRune(punctuation, r[len(r)-1]) {
			out = append(out, string(r[:len(r)-1]), string(r[len(r)-1]))
		} else if len(r) > 1 && strings.ContainsRune(punctuation, r[0]) {
			out = append(out, string(r[0]), string(r[1:]))
		} else {
			out = append(out, word)
		}
	}
	return out
}
