package check

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// NumberKey normalizes exact JSON decimal equality without expanding powers of
// ten. Expanding a small input such as 1e1000000000 into a big.Rat can allocate
// hundreds of megabytes. This representation uses space linear in input size.
func NumberKey(number json.Number) (string, error) {
	s := number.String()
	if !json.Valid([]byte(s)) || s == "" || strings.ContainsAny(s, " \t\r\n") {
		return "", fmt.Errorf("goeval: invalid JSON number")
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if s == "" || s[0] < '0' || s[0] > '9' {
		return "", fmt.Errorf("goeval: invalid JSON number")
	}
	exponent := int64(0)
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		var err error
		exponent, err = strconv.ParseInt(s[i+1:], 10, 64)
		if err != nil {
			return "", fmt.Errorf("goeval: number exponent is out of range")
		}
		s = s[:i]
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		fraction := int64(len(s) - i - 1)
		if exponent < math.MinInt64+fraction {
			return "", fmt.Errorf("goeval: number exponent is out of range")
		}
		exponent -= fraction
		s = s[:i] + s[i+1:]
	}
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0", nil
	}
	trimmed := strings.TrimRight(s, "0")
	zeros := int64(len(s) - len(trimmed))
	if exponent > math.MaxInt64-zeros {
		return "", fmt.Errorf("goeval: number exponent is out of range")
	}
	exponent += zeros
	s = trimmed
	if negative {
		s = "-" + s
	}
	if exponent != 0 {
		s += "e" + strconv.FormatInt(exponent, 10)
	}
	return s, nil
}
