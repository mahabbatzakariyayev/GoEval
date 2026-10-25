// Package check contains shared validation for public library boundaries.
package check

import (
	"context"
	"fmt"
	"math"
	"reflect"
)

// Nil also recognizes interfaces holding typed nil pointers or functions.
func Nil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func Context(ctx context.Context) error {
	if Nil(ctx) {
		return fmt.Errorf("goeval: nil context")
	}
	return ctx.Err()
}

// CopyPtr removes configuration aliases retained by constructors.
func CopyPtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func Embeddings(vectors [][]float64, count int) error {
	if len(vectors) != count {
		return fmt.Errorf("goeval: expected %d embeddings, got %d", count, len(vectors))
	}
	dim := 0
	for _, vector := range vectors {
		if len(vector) == 0 {
			return fmt.Errorf("goeval: empty embedding")
		}
		if dim == 0 {
			dim = len(vector)
		}
		if len(vector) != dim {
			return fmt.Errorf("goeval: inconsistent embedding dimensions")
		}
		for _, value := range vector {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("goeval: non-finite embedding")
			}
		}
	}
	return nil
}
