// Package judge provides configurable multi-stage model judges, custom criteria,
// decision graphs, tool matching, and direct measurements. Its metrics implement
// GoEval's shared metric interface. Use the root goeval package for the simpler
// Case and Evaluator API. Metrics are safe to share across concurrent rows when
// their providers support concurrent requests. See NOTICE for adaptation notices.
package judge
