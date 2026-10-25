package check

import (
	"encoding/json"
	"testing"
)

func TestNumberKey(t *testing.T) {
	for input, want := range map[string]string{"1": "1", "1.0": "1", "-0.000": "0", "100": "1e2", "1e2": "1e2", "0.0100": "1e-2", "12e-1": "12e-1", "1e1000000000": "1e1000000000"} {
		got, err := NumberKey(json.Number(input))
		if err != nil || got != want {
			t.Fatal(input, got, err)
		}
	}
	for _, input := range []string{"null", "true", `"1"`, "1 2", "NaN", "1e99999999999999999999999"} {
		if _, err := NumberKey(json.Number(input)); err == nil {
			t.Fatal("invalid number accepted", input)
		}
	}
}
