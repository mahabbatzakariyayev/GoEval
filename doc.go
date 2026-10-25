// Package goeval evaluates AI answers, retrieval, summaries, and agent tools.
// Construct an Evaluator with New, choose built-in metrics with WithMetrics,
// and pass cases using Input, Output, Expected, and Context fields. All execution
// honors contexts, supports concurrent runs, and uses explicit model providers.
//
// The default evaluator uses ExactMatch and requires no model or credentials.
// Model-based judges share an LLM and support Threshold, Strict and WithoutReason
// options. Judge defines custom criteria. Provider SDK dependencies are optional.
//
// Advanced metric and compatibility APIs remain available in subpackages.
// See NOTICE for the origins of adapted implementations.
package goeval
