package core

import (
	"fmt"
	"strings"

	"github.com/mahabbatzakariyayev/GoEval/internal/check"
)

type preparedMetric struct {
	Metric
	name   string
	fields []Field
}

func (m preparedMetric) Name() string            { return m.name }
func (m preparedMetric) RequiredFields() []Field { return append([]Field(nil), m.fields...) }

func (m preparedMetric) ValidateSample(s Sample) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = &PanicError{Value: p}
		}
	}()
	if err := s.Validate(m.fields...); err != nil {
		return err
	}
	if validator, ok := m.Metric.(interface{ ValidateSample(Sample) error }); ok {
		return validator.ValidateSample(s)
	}
	return nil
}

func prepareMetric(m Metric) (out preparedMetric, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = &PanicError{Value: p}
		}
	}()
	if check.Nil(m) {
		return out, fmt.Errorf("goeval: nil metric")
	}
	if validator, ok := m.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return out, err
		}
	}
	out = preparedMetric{Metric: m, name: m.Name(), fields: m.RequiredFields()}
	if strings.TrimSpace(out.name) == "" {
		return out, fmt.Errorf("goeval: metric requires a name")
	}
	out.fields = append([]Field(nil), out.fields...)
	for _, field := range out.fields {
		switch field {
		case UserInput, Response, Reference, RetrievedContexts, ReferenceContexts, RetrievedContextIDs, ReferenceContextIDs, Messages, ReferenceToolCalls, ReferenceTopics, Rubrics:
		default:
			return out, fmt.Errorf("goeval: unknown required field %q for metric %q", field, out.name)
		}
	}
	return out, nil
}

// ValidateMetrics checks names and required-field declarations without calling
// models. Use it before running an application or another costly step.
func ValidateMetrics(metrics []Metric) error {
	_, err := prepareMetrics(metrics)
	return err
}
func prepareMetrics(metrics []Metric) ([]preparedMetric, error) {
	if len(metrics) == 0 {
		return nil, fmt.Errorf("goeval: at least one metric is required")
	}
	out := make([]preparedMetric, len(metrics))
	names := map[string]bool{}
	for i, m := range metrics {
		prepared, err := prepareMetric(m)
		if err != nil {
			return nil, err
		}
		if names[prepared.name] {
			return nil, fmt.Errorf("goeval: duplicate metric %q", prepared.name)
		}
		names[prepared.name] = true
		out[i] = prepared
	}
	return out, nil
}
