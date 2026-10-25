package goeval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type reportJSON struct {
	*EvaluationResult
	Metrics []string `json:"metrics"`
	Error   string   `json:"error,omitempty"`
}

// MarshalJSON retains the requested metrics and run failure as well as scores.
func (r Report) MarshalJSON() ([]byte, error) {
	if r.EvaluationResult == nil {
		return []byte("null"), nil
	}
	value := reportJSON{EvaluationResult: r.EvaluationResult, Metrics: r.metrics}
	if r.runErr != nil {
		value.Error = r.runErr.Error()
	}
	return json.Marshal(value)
}

// UnmarshalJSON restores a report, including its quality-gate state. Recorded
// errors retain their messages; Go error types and causes cannot survive JSON.
func (r *Report) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*r = Report{}
		return nil
	}
	value := reportJSON{EvaluationResult: &EvaluationResult{}}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("goeval: trailing report data")
	}
	seen := make(map[string]bool, len(value.Metrics))
	for _, name := range value.Metrics {
		if name == "" || seen[name] {
			return fmt.Errorf("goeval: report contains an empty or duplicate metric name")
		}
		seen[name] = true
	}
	for _, row := range value.Rows {
		for name, score := range row.Scores {
			if err := score.Validate(); err != nil {
				return fmt.Errorf("goeval: invalid saved score %q: %w", name, err)
			}
		}
	}
	var runErr error
	if value.Error != "" {
		runErr = errors.New(value.Error)
	}
	*r = Report{EvaluationResult: value.EvaluationResult, metrics: value.Metrics, runErr: runErr}
	return nil
}

// Err returns the run failure, including cancellation and Check threshold
// failures, or any recorded row errors. Deserialized errors retain messages.
func (r *Report) Err() error {
	if r == nil {
		return nil
	}
	if r.runErr != nil {
		return r.runErr
	}
	return r.EvaluationResult.Err()
}
