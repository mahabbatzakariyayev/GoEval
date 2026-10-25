package core

import "fmt"

// MissingFieldError identifies an absent dataset field.
type MissingFieldError struct{ Field Field }

func (e *MissingFieldError) Error() string {
	return fmt.Sprintf("goeval: required field %q is missing", e.Field)
}

// PanicError reports a panic recovered at a user-provided metric boundary.
// Panics are not retried by the evaluation runner.
type PanicError struct{ Value any }

func (e *PanicError) Error() string { return fmt.Sprintf("goeval: metric panic: %v", e.Value) }
func (e *PanicError) Unwrap() error { err, _ := e.Value.(error); return err }

// ThresholdError identifies a completed evaluation that failed its quality gate.
type ThresholdError struct {
	Metric    string
	Score     float64
	Threshold float64
	Reason    string
}

func (e *ThresholdError) Error() string {
	return fmt.Sprintf("%s: score %g below threshold %g: %s", e.Metric, e.Score, e.Threshold, e.Reason)
}
