package scorers

import "strings"

// porter implements the NLTK_EXTENSIONS variant used by Python rouge_score.
// Adapted from NLTK's Apache-2.0 PorterStemmer; see NOTICE.
func consonant(s string, i int) bool {
	switch s[i] {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	case 'y':
		return i == 0 || !consonant(s, i-1)
	}
	return true
}
func measure(s string) int {
	n := 0
	for i := 1; i < len(s); i++ {
		if !consonant(s, i-1) && consonant(s, i) {
			n++
		}
	}
	return n
}
func hasVowel(s string) bool {
	for i := range s {
		if !consonant(s, i) {
			return true
		}
	}
	return false
}
func doubleConsonant(s string) bool {
	n := len(s)
	return n >= 2 && s[n-1] == s[n-2] && consonant(s, n-1)
}
func cvc(s string) bool {
	n := len(s)
	return (n >= 3 && consonant(s, n-3) && !consonant(s, n-2) && consonant(s, n-1) && !strings.ContainsRune("wxy", rune(s[n-1]))) || (n == 2 && !consonant(s, 0) && consonant(s, 1))
}

type suffixRule struct{ suffix, replacement string }

func applySuffix(s string, rules []suffixRule, minMeasure int) string {
	for _, r := range rules {
		if strings.HasSuffix(s, r.suffix) {
			stem := strings.TrimSuffix(s, r.suffix)
			if measure(stem) > minMeasure {
				return stem + r.replacement
			}
			return s
		}
	}
	return s
}
func porterStep2(s string) string {
	if strings.HasSuffix(s, "alli") && measure(strings.TrimSuffix(s, "alli")) > 0 {
		return porterStep2(strings.TrimSuffix(s, "alli") + "al")
	}
	rules := []suffixRule{{"ational", "ate"}, {"tional", "tion"}, {"enci", "ence"}, {"anci", "ance"}, {"izer", "ize"}, {"bli", "ble"}, {"alli", "al"}, {"entli", "ent"}, {"eli", "e"}, {"ousli", "ous"}, {"ization", "ize"}, {"ation", "ate"}, {"ator", "ate"}, {"alism", "al"}, {"iveness", "ive"}, {"fulness", "ful"}, {"ousness", "ous"}, {"aliti", "al"}, {"iviti", "ive"}, {"biliti", "ble"}, {"fulli", "ful"}}
	for _, r := range rules {
		if strings.HasSuffix(s, r.suffix) {
			return applySuffix(s, []suffixRule{r}, 0)
		}
	}
	if strings.HasSuffix(s, "logi") && measure(s[:len(s)-3]) > 0 {
		return s[:len(s)-1]
	}
	return s
}
func porter(s string) string {
	irregular := map[string]string{"sky": "sky", "skies": "sky", "dying": "die", "lying": "lie", "tying": "tie", "news": "news", "innings": "inning", "inning": "inning", "outings": "outing", "outing": "outing", "cannings": "canning", "canning": "canning", "howe": "howe", "proceed": "proceed", "exceed": "exceed", "succeed": "succeed"}
	if v, ok := irregular[s]; ok {
		return v
	}
	if len(s) <= 2 {
		return s
	}
	if len(s) == 4 && strings.HasSuffix(s, "ies") {
		s = s[:len(s)-3] + "ie"
	} else {
		s = applySuffix(s, []suffixRule{{"sses", "ss"}, {"ies", "i"}, {"ss", "ss"}, {"s", ""}}, -1)
	}
	if strings.HasSuffix(s, "ied") {
		if len(s) == 4 {
			s = s[:len(s)-3] + "ie"
		} else {
			s = s[:len(s)-3] + "i"
		}
	} else if strings.HasSuffix(s, "eed") {
		s = applySuffix(s, []suffixRule{{"eed", "ee"}}, 0)
	} else {
		for _, suffix := range []string{"ed", "ing"} {
			if strings.HasSuffix(s, suffix) {
				stem := strings.TrimSuffix(s, suffix)
				if !hasVowel(stem) {
					continue
				}
				s = stem
				switch {
				case strings.HasSuffix(s, "at") || strings.HasSuffix(s, "bl") || strings.HasSuffix(s, "iz"):
					s += "e"
				case doubleConsonant(s):
					if !strings.ContainsRune("lsz", rune(s[len(s)-1])) {
						s = s[:len(s)-1]
					}
				case measure(s) == 1 && cvc(s):
					s += "e"
				}
				break
			}
		}
	}
	if strings.HasSuffix(s, "y") && len(s) > 2 && consonant(s, len(s)-2) {
		s = s[:len(s)-1] + "i"
	}
	s = porterStep2(s)
	s = applySuffix(s, []suffixRule{{"icate", "ic"}, {"ative", ""}, {"alize", "al"}, {"iciti", "ic"}, {"ical", "ic"}, {"ful", ""}, {"ness", ""}}, 0)
	for _, suffix := range []string{"al", "ance", "ence", "er", "ic", "able", "ible", "ant", "ement", "ment", "ent", "ion", "ou", "ism", "ate", "iti", "ous", "ive", "ize"} {
		if strings.HasSuffix(s, suffix) {
			stem := strings.TrimSuffix(s, suffix)
			if measure(stem) > 1 && (suffix != "ion" || (len(stem) > 0 && strings.ContainsRune("st", rune(stem[len(stem)-1])))) {
				s = stem
			}
			break
		}
	}
	if strings.HasSuffix(s, "e") {
		stem := s[:len(s)-1]
		m := measure(stem)
		if m > 1 || (m == 1 && !cvc(stem)) {
			s = stem
		}
	}
	if strings.HasSuffix(s, "ll") && measure(s[:len(s)-1]) > 1 {
		s = s[:len(s)-1]
	}
	return s
}
