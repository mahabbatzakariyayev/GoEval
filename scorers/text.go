// Package metrics provides model-free, LLM, embedding, and agent evaluators.
package scorers

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

func textMetric(name string, f func(string, string) (float64, error)) goeval.Metric {
	return goeval.MetricFunc{MetricName: name, Fields: []goeval.Field{goeval.Response, goeval.Reference}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if err := ctx.Err(); err != nil {
			return goeval.Score{}, err
		}
		v, err := f(s.Response, s.Reference)
		return goeval.Score{Value: v}, err
	}}
}
func ExactMatch() goeval.Metric {
	return textMetric("exact_match", func(a, b string) (float64, error) {
		if a == b {
			return 1, nil
		}
		return 0, nil
	})
}
func StringPresence() goeval.Metric {
	return textMetric("string_present", func(a, b string) (float64, error) {
		if strings.Contains(a, b) {
			return 1, nil
		}
		return 0, nil
	})
}

type Distance string

const (
	Levenshtein Distance = "levenshtein"
	Hamming     Distance = "hamming"
	Jaro        Distance = "jaro"
	JaroWinkler Distance = "jaro_winkler"
)

func NonLLMStringSimilarity(distance Distance) goeval.Metric {
	return textMetric("non_llm_string_similarity", func(a, b string) (float64, error) { return StringSimilarity(a, b, distance) })
}

// StringSimilarity works on Unicode code points, matching Python string length.
func StringSimilarity(a, b string, distance Distance) (float64, error) {
	x, y := []rune(a), []rune(b)
	n := max(len(x), len(y))
	if distance == "" {
		distance = Levenshtein
	}
	if distance != Levenshtein && distance != Hamming && distance != Jaro && distance != JaroWinkler {
		return 0, fmt.Errorf("metrics: unknown distance %q", distance)
	}
	if a == b {
		return 1, nil
	}
	if n == 0 {
		return 1, nil
	}
	switch distance {
	case Levenshtein:
		row := make([]int, len(y)+1)
		for j := range row {
			row[j] = j
		}
		for i, c := range x {
			prev := row[0]
			row[0] = i + 1
			for j, d := range y {
				old := row[j+1]
				cost := 0
				if c != d {
					cost = 1
				}
				row[j+1] = min(row[j]+1, old+1, prev+cost)
				prev = old
			}
		}
		return 1 - float64(row[len(y)])/float64(n), nil
	case Hamming:
		d := n - min(len(x), len(y))
		for i := 0; i < min(len(x), len(y)); i++ {
			if x[i] != y[i] {
				d++
			}
		}
		return 1 - float64(d)/float64(n), nil
	case Jaro, JaroWinkler:
		if len(x) == 0 || len(y) == 0 {
			return 0, nil
		}
		window := max(n/2-1, 0)
		xm, ym := make([]bool, len(x)), make([]bool, len(y))
		matches := 0
		for i, c := range x {
			for j := max(0, i-window); j < min(len(y), i+window+1); j++ {
				if !ym[j] && c == y[j] {
					xm[i] = true
					ym[j] = true
					matches++
					break
				}
			}
		}
		if matches == 0 {
			return 0, nil
		}
		trans, k := 0, 0
		for i, c := range x {
			if !xm[i] {
				continue
			}
			for !ym[k] {
				k++
			}
			if c != y[k] {
				trans++
			}
			k++
		}
		m := float64(matches)
		v := (m/float64(len(x)) + m/float64(len(y)) + (m-float64(trans/2))/m) / 3
		if distance == JaroWinkler && v > 0.7 {
			prefix := 0
			for i := 0; i < min(4, len(x), len(y)); i++ {
				if x[i] != y[i] {
					break
				}
				prefix++
			}
			v += float64(prefix) * 0.1 * (1 - v)
		}
		return v, nil
	}
	return 0, fmt.Errorf("metrics: invalid distance")
}

func ngrams(tokens []string, n int) map[string]int {
	m := map[string]int{}
	for i := 0; i+n <= len(tokens); i++ {
		m[strings.Join(tokens[i:i+n], "\x00")]++
	}
	return m
}
func overlap(a, b map[string]int) int {
	v := 0
	for k, n := range a {
		v += min(n, b[k])
	}
	return v
}
func sumCounts(a map[string]int) int {
	v := 0
	for _, n := range a {
		v += n
	}
	return v
}
func fbeta(p, r, beta float64) float64 {
	if beta == 0 {
		return p
	}
	if p == 0 || r == 0 {
		return 0
	}
	if beta > 1 {
		inverse := 1 / beta
		b := inverse * inverse
		return (1 + b) * p * r / (p + b*r)
	}
	b := beta * beta
	return (1 + b) * p * r / (b*p + r)
}

var rougeNonAlpha = regexp.MustCompile(`[^a-z0-9]+`)

func tokens(s string) []string {
	out := strings.Fields(rougeNonAlpha.ReplaceAllString(strings.ToLower(s), " "))
	for i, w := range out {
		if len(w) > 3 {
			out[i] = porter(w)
		}
	}
	return out
}

type RougeOptions struct {
	Type string
	Mode string
}

// RougeScore uses Python rouge_score tokenization and NLTK Porter stemming.
func RougeScore(opts RougeOptions) goeval.Metric {
	return textMetric("rouge_score", func(a, b string) (float64, error) {
		kind, mode := opts.Type, opts.Mode
		if kind == "" {
			kind = "rougeL"
		}
		if mode == "" {
			mode = "fmeasure"
		}
		x, y := tokens(a), tokens(b)
		matched := 0
		nx, ny := len(x), len(y)
		switch kind {
		case "rouge1", "rouge2":
			n := 1
			if kind == "rouge2" {
				n = 2
			}
			gx, gy := ngrams(x, n), ngrams(y, n)
			matched = overlap(gx, gy)
			nx, ny = sumCounts(gx), sumCounts(gy)
		case "rougeL":
			row := make([]int, len(y)+1)
			for _, c := range x {
				prev := 0
				for j, d := range y {
					old := row[j+1]
					if c == d {
						row[j+1] = prev + 1
					} else {
						row[j+1] = max(row[j+1], row[j])
					}
					prev = old
				}
			}
			matched = row[len(y)]
		default:
			return 0, fmt.Errorf("metrics: invalid ROUGE type %q", kind)
		}
		p, r := 0.0, 0.0
		if nx > 0 {
			p = float64(matched) / float64(nx)
		}
		if ny > 0 {
			r = float64(matched) / float64(ny)
		}
		switch mode {
		case "precision":
			return p, nil
		case "recall":
			return r, nil
		case "fmeasure":
			return fbeta(p, r, 1), nil
		default:
			return 0, fmt.Errorf("metrics: invalid ROUGE mode %q", mode)
		}
	})
}

type BLEUOptions struct {
	MaxOrder       int
	EffectiveOrder bool
	Lowercase      bool
}

// BleuScore uses clipped n-gram precision, exponential smoothing and brevity
// penalty, preserving GoEval's sentence splitting and sacrebleu 13a tokenization.
func BleuScore(opts BLEUOptions) goeval.Metric {
	return textMetric("bleu_score", func(a, b string) (float64, error) {
		n := opts.MaxOrder
		if n == 0 {
			n = 4
		}
		if n < 1 || n > 16 {
			return 0, fmt.Errorf("metrics: BLEU order must be between 1 and 16")
		}
		if opts.Lowercase {
			a, b = strings.ToLower(a), strings.ToLower(b)
		}
		x := tokenize13a(strings.Split(a, ". ")[0])
		references := strings.Split(b, ". ")
		refTokens := make([][]string, len(references))
		refLength := -1
		for i, r := range references {
			refTokens[i] = tokenize13a(r)
			n := len(refTokens[i])
			if refLength < 0 || absInt(n-len(x)) < absInt(refLength-len(x)) || (absInt(n-len(x)) == absInt(refLength-len(x)) && n < refLength) {
				refLength = n
			}
		}
		if len(x) == 0 {
			return 0, nil
		}
		logp, smooth, orders := 0.0, 1.0, 0
		for order := 1; order <= n; order++ {
			gx := ngrams(x, order)
			gy := map[string]int{}
			for _, ref := range refTokens {
				for k, count := range ngrams(ref, order) {
					gy[k] = max(gy[k], count)
				}
			}
			total := sumCounts(gx)
			if total == 0 {
				if opts.EffectiveOrder {
					break
				}
				return 0, nil
			}
			hit := overlap(gx, gy)
			p := 0.0
			if hit == 0 {
				smooth *= 2
				p = 1 / (smooth * float64(total))
			} else {
				p = float64(hit) / float64(total)
			}
			logp += math.Log(p)
			orders++
		}
		unigrams := map[string]int{}
		for _, ref := range refTokens {
			for k, count := range ngrams(ref, 1) {
				unigrams[k] = max(unigrams[k], count)
			}
		}
		if overlap(ngrams(x, 1), unigrams) == 0 || orders == 0 {
			return 0, nil
		}
		bp := 1.0
		if len(x) < refLength {
			bp = math.Exp(1 - float64(refLength)/float64(len(x)))
		}
		return bp * math.Exp(logp/float64(orders)), nil
	})
}

type ChrFOptions struct {
	CharOrder         int
	WordOrder         int
	Beta              float64
	IncludeWhitespace bool
	Lowercase         bool
}

func ChrfScore(opts ChrFOptions) goeval.Metric {
	return textMetric("chrf_score", func(a, b string) (float64, error) {
		n, beta := opts.CharOrder, opts.Beta
		if n == 0 {
			n = 6
		}
		if beta == 0 {
			beta = 2
		}
		if n < 1 || n > 32 || opts.WordOrder < 0 || opts.WordOrder > 8 || beta < 0 || math.IsNaN(beta) || math.IsInf(beta, 0) {
			return 0, fmt.Errorf("metrics: invalid chrF options")
		}
		if opts.Lowercase {
			a, b = strings.ToLower(a), strings.ToLower(b)
		}
		chars := func(s string) []string {
			out := []string{}
			for _, r := range s {
				if opts.IncludeWhitespace || !unicode.IsSpace(r) {
					out = append(out, string(r))
				}
			}
			return out
		}
		x, y := chars(a), chars(b)
		p, r, count := 0.0, 0.0, 0.0
		add := func(gx, gy map[string]int) {
			nx, ny := sumCounts(gx), sumCounts(gy)
			if nx > 0 && ny > 0 {
				hit := float64(overlap(gx, gy))
				p += hit / float64(nx)
				r += hit / float64(ny)
				count++
			}
		}
		for order := 1; order <= n; order++ {
			add(ngrams(x, order), ngrams(y, order))
		}
		for order := 1; order <= opts.WordOrder; order++ {
			add(ngrams(chrfWords(a), order), ngrams(chrfWords(b), order))
		}
		if count == 0 {
			return 0, nil
		}
		return fbeta(p/count, r/count, beta), nil
	})
}

// SemanticSimilarity is raw cosine similarity, including possible negative scores.
func SemanticSimilarity(embedder goeval.Embedder) goeval.Metric {
	return goeval.MetricFunc{MetricName: "semantic_similarity", Fields: []goeval.Field{goeval.Response, goeval.Reference}, Evaluate: func(ctx context.Context, s goeval.Sample) (goeval.Score, error) {
		if check.Nil(embedder) {
			return goeval.Score{}, fmt.Errorf("metrics: embedder is required")
		}
		v, err := embedder.Embed(ctx, []string{s.Response, s.Reference})
		if err != nil {
			return goeval.Score{}, err
		}
		if len(v) != 2 {
			return goeval.Score{}, fmt.Errorf("metrics: expected two embeddings")
		}
		score, err := Cosine(v[0], v[1])
		return goeval.Score{Value: score}, err
	}}
}
func AnswerSimilarity(e goeval.Embedder) goeval.Metric {
	m := SemanticSimilarity(e)
	return rename("answer_similarity", m)
}
func rename(name string, m goeval.Metric) goeval.Metric {
	return goeval.MetricFunc{MetricName: name, Fields: m.RequiredFields(), Evaluate: m.Score}
}
func Cosine(a, b []float64) (float64, error) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, fmt.Errorf("metrics: incompatible embedding dimensions")
	}
	// Scale first to avoid overflow for finite but very large coordinates.
	sa, sb := 0.0, 0.0
	for i, x := range a {
		y := b[i]
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			return 0, fmt.Errorf("metrics: non-finite embedding")
		}
		sa = math.Max(sa, math.Abs(x))
		sb = math.Max(sb, math.Abs(y))
	}
	if sa == 0 || sb == 0 {
		return 0, fmt.Errorf("metrics: zero-norm embedding")
	}
	dot, aa, bb := 0.0, 0.0, 0.0
	for i, x := range a {
		u, v := x/sa, b[i]/sb
		dot += u * v
		aa += u * u
		bb += v * v
	}
	return math.Max(-1, math.Min(1, dot/math.Sqrt(aa*bb))), nil
}
