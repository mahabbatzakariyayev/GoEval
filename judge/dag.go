package judge

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	goeval "github.com/mahabbatzakariyayev/GoEval/internal/core"
)

// Node is a task, judgement, or verdict in a Judge decision graph.
// Graphs are validated and copied by DAGMetric; each measurement has its own
// outputs and traversal state, allowing a metric to be used concurrently.
type Node interface{ dagNode() }

type TaskNode struct {
	Instructions     string
	OutputLabel      string
	EvaluationParams []SingleTurnParam
	Children         []Node
	Label            string
}

func (*TaskNode) dagNode() {}

type BinaryJudgementNode struct {
	Criteria         string
	EvaluationParams []SingleTurnParam
	Children         []*VerdictNode
	Label            string
}

func (*BinaryJudgementNode) dagNode() {}

type NonBinaryJudgementNode struct {
	Criteria         string
	EvaluationParams []SingleTurnParam
	Children         []*VerdictNode
	Label            string
}

func (*NonBinaryJudgementNode) dagNode() {}

// VerdictNode chooses either a deterministic integer score from 0 to 10, a
// continuation node, or a child metric. Set exactly one of Score, Child, Metric.
// Verdict must be a bool for a binary parent or a string for a nonbinary parent.
type VerdictNode struct {
	Verdict any
	Score   *int
	Child   Node
	Metric  Metric
}

func (*VerdictNode) dagNode() {}

func Int(v int) *int { return &v }

type DAGOptions struct {
	Options
	Name      string
	RootNodes []Node
}

type decisionGraph struct {
	roots    []Node
	parents  map[Node][]Node
	indegree map[Node]int
	params   []SingleTurnParam
}

func edges(n Node) []Node {
	switch n := n.(type) {
	case *TaskNode:
		return n.Children
	case *BinaryJudgementNode:
		out := make([]Node, len(n.Children))
		for i, c := range n.Children {
			out[i] = c
		}
		return out
	case *NonBinaryJudgementNode:
		out := make([]Node, len(n.Children))
		for i, c := range n.Children {
			out[i] = c
		}
		return out
	case *VerdictNode:
		if n.Child != nil {
			return []Node{n.Child}
		}
	}
	return nil
}

func isJudgement(n Node) bool {
	switch n.(type) {
	case *BinaryJudgementNode, *NonBinaryJudgementNode:
		return true
	}
	return false
}
func nodeParams(n Node) []SingleTurnParam {
	switch n := n.(type) {
	case *TaskNode:
		return n.EvaluationParams
	case *BinaryJudgementNode:
		return n.EvaluationParams
	case *NonBinaryJudgementNode:
		return n.EvaluationParams
	}
	return nil
}

func buildGraph(roots []Node) (*decisionGraph, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("judge: DAG needs a root")
	}
	clones := map[Node]Node{}
	colors := map[Node]int{}
	var copyNode func(Node) (Node, error)
	copyNode = func(n Node) (Node, error) {
		if n == nil || reflect.ValueOf(n).IsNil() {
			return nil, fmt.Errorf("judge: nil DAG node")
		}
		if colors[n] == 1 {
			return nil, fmt.Errorf("judge: cycle detected in DAG")
		}
		if clone, ok := clones[n]; ok {
			return clone, nil
		}
		colors[n] = 1
		var clone Node
		switch n := n.(type) {
		case *TaskNode:
			v := *n
			v.EvaluationParams = append([]SingleTurnParam(nil), n.EvaluationParams...)
			v.Children = nil
			for _, c := range n.Children {
				if _, bad := c.(*VerdictNode); bad {
					return nil, fmt.Errorf("judge: task cannot directly parent a verdict")
				}
				cc, err := copyNode(c)
				if err != nil {
					return nil, err
				}
				v.Children = append(v.Children, cc)
			}
			clone = &v
		case *BinaryJudgementNode:
			if len(n.Children) != 2 {
				return nil, fmt.Errorf("judge: binary judgement needs exactly two verdicts")
			}
			v := *n
			v.EvaluationParams = append([]SingleTurnParam(nil), n.EvaluationParams...)
			v.Children = nil
			seen := map[bool]bool{}
			for _, c := range n.Children {
				if c == nil {
					return nil, fmt.Errorf("judge: nil verdict")
				}
				b, ok := c.Verdict.(bool)
				if !ok || seen[b] {
					return nil, fmt.Errorf("judge: binary verdicts must be true and false")
				}
				seen[b] = true
				cc, err := copyNode(c)
				if err != nil {
					return nil, err
				}
				v.Children = append(v.Children, cc.(*VerdictNode))
			}
			clone = &v
		case *NonBinaryJudgementNode:
			if len(n.Children) == 0 {
				return nil, fmt.Errorf("judge: nonbinary judgement needs verdicts")
			}
			v := *n
			v.EvaluationParams = append([]SingleTurnParam(nil), n.EvaluationParams...)
			v.Children = nil
			seen := map[string]bool{}
			for _, c := range n.Children {
				if c == nil {
					return nil, fmt.Errorf("judge: nil verdict")
				}
				s, ok := c.Verdict.(string)
				if !ok || seen[s] {
					return nil, fmt.Errorf("judge: nonbinary verdicts must be unique strings")
				}
				seen[s] = true
				cc, err := copyNode(c)
				if err != nil {
					return nil, err
				}
				v.Children = append(v.Children, cc.(*VerdictNode))
			}
			clone = &v
		case *VerdictNode:
			count := 0
			if n.Score != nil {
				count++
				if *n.Score < 0 || *n.Score > 10 {
					return nil, fmt.Errorf("judge: DAG score must be in [0,10]")
				}
			}
			if n.Child != nil {
				count++
			}
			if n.Metric != nil {
				count++
			}
			if count != 1 {
				return nil, fmt.Errorf("judge: verdict needs exactly one score, node, or metric")
			}
			v := *n
			if n.Score != nil {
				v.Score = Int(*n.Score)
			}
			if n.Child != nil {
				cc, err := copyNode(n.Child)
				if err != nil {
					return nil, err
				}
				v.Child = cc
			}
			clone = &v
		default:
			return nil, fmt.Errorf("judge: unsupported DAG node")
		}
		colors[n] = 2
		clones[n] = clone
		return clone, nil
	}
	g := &decisionGraph{parents: map[Node][]Node{}, indegree: map[Node]int{}}
	for _, root := range roots {
		if reflect.TypeOf(root) != reflect.TypeOf(roots[0]) {
			return nil, fmt.Errorf("judge: DAG roots must have the same node type")
		}
		if len(roots) > 1 && isJudgement(root) {
			return nil, fmt.Errorf("judge: a judgement must be the sole root")
		}
		n, err := copyNode(root)
		if err != nil {
			return nil, err
		}
		g.roots = append(g.roots, n)
	}
	visited := map[Node]bool{}
	params := map[SingleTurnParam]bool{}
	var visit func(Node) error
	visit = func(n Node) error {
		if visited[n] {
			return nil
		}
		visited[n] = true
		if _, ok := g.indegree[n]; !ok {
			g.indegree[n] = 0
		}
		for _, p := range nodeParams(n) {
			if _, ok := parameterNames[p]; !ok {
				return fmt.Errorf("judge: unknown DAG parameter %q", p)
			}
			if !params[p] {
				params[p] = true
				g.params = append(g.params, p)
			}
		}
		for _, c := range edges(n) {
			g.indegree[c]++
			g.parents[c] = append(g.parents[c], n)
			if err := visit(c); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range g.roots {
		if err := visit(root); err != nil {
			return nil, err
		}
	}
	rootSet := map[Node]bool{}
	for _, root := range g.roots {
		if rootSet[root] || g.indegree[root] > 0 {
			return nil, fmt.Errorf("judge: duplicate or reachable root")
		}
		rootSet[root] = true
	}
	// Mutually exclusive verdict siblings together supply one arrival to a
	// shared continuation. Losing branches never decrement its indegree.
	for n, parents := range g.parents {
		owners := map[Node]int{}
		for _, p := range parents {
			if _, ok := p.(*VerdictNode); !ok || rootSet[p] {
				continue
			}
			up := g.parents[p]
			if len(up) == 1 && isJudgement(up[0]) {
				owners[up[0]]++
			}
		}
		for _, count := range owners {
			if count > 1 {
				g.indegree[n] -= count - 1
			}
		}
	}
	return g, nil
}

// DAGMetric follows Judge's synchronous graph traversal: selected terminal
// verdicts supply scores (0–10 divided by 10); later terminal visits replace
// earlier scores. Graph branches do not implicitly average their results.
func DAGMetric(llm goeval.LLM, cfg DAGOptions) (Metric, error) {
	cfg.Options = cloneOptions(cfg.Options)
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("judge: DAG name is required")
	}
	g, err := buildGraph(cfg.RootNodes)
	if err != nil {
		return nil, err
	}
	m := newMetric("dag."+cfg.Name, cfg.Name+" (DAG)", g.params, cfg.Options, func(ctx context.Context, c Case, _ float64) (float64, string, map[string]any, error) {
		remaining := map[Node]int{}
		for n, d := range g.indegree {
			remaining[n] = d
		}
		outputs := map[Node]any{}
		choices := map[Node]any{}
		executed := map[Node]bool{}
		logs := []string{}
		finalScore := 0.0
		finalReason := ""
		hasScore := false
		resolve := func(n Node, sep string) string {
			var b strings.Builder
			for _, parent := range g.parents[n] {
				if task, ok := parent.(*TaskNode); ok {
					fmt.Fprintf(&b, "%s:\n%s%s", task.OutputLabel, goeval.PythonString(outputs[parent]), sep)
				}
			}
			for _, p := range nodeParams(n) {
				fmt.Fprintf(&b, "%s:\n%s\n", parameterNames[p], goeval.PythonString(c.value(p)))
			}
			return b.String()
		}
		var visit func(Node, int) error
		visit = func(n Node, depth int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			remaining[n]--
			if remaining[n] > 0 || executed[n] {
				return nil
			}
			executed[n] = true
			if v, ok := n.(*VerdictNode); ok {
				if parents := g.parents[n]; len(parents) > 0 && isJudgement(parents[0]) && !reflect.DeepEqual(choices[parents[0]], v.Verdict) {
					return nil
				}
				if v.Child != nil {
					return visit(v.Child, depth)
				}
				if v.Metric != nil {
					r, err := v.Metric.Measure(ctx, c)
					if err != nil {
						return err
					}
					finalScore, finalReason, hasScore = r.Score, r.Reason, true
					logs = append(logs, fmt.Sprintf("Verdict %s: %s score %g", goeval.PythonString(v.Verdict), v.Metric.DisplayName(), r.Score))
					return nil
				}
				finalScore, hasScore = float64(*v.Score)/10, true
				logs = append(logs, fmt.Sprintf("________________________\n| VerdictNode | Level == %d |\n**********************************\nVerdict: %s\nType: Deterministic", depth, goeval.PythonString(v.Verdict)))
				var err error
				finalReason, err = explanation(ctx, llm, "VerdictNode.generate_reason", map[string]any{"name": cfg.Name + " (DAG)", "score": finalScore, "verbose_steps": append([]string(nil), logs...)}, cfg.Options)
				return err
			}
			switch n := n.(type) {
			case *TaskNode:
				if len(n.EvaluationParams) == 0 && len(g.parents[n]) == 0 {
					return fmt.Errorf("judge: root task requires evaluation parameters")
				}
				var out struct {
					Output any `json:"output"`
				}
				outputSchema := map[string]any{"anyOf": []any{str(), array(str()), map[string]any{"type": "object", "additionalProperties": str()}}}
				if err := judge(ctx, llm, "TaskNode.generate_task_output", map[string]any{"instructions": n.Instructions, "text": resolve(n, "\n\n")}, schema(map[string]any{"output": outputSchema}), &out); err != nil {
					return err
				}
				if !validTaskOutput(out.Output) {
					return fmt.Errorf("judge: task output must be a string, string list, or string map")
				}
				outputs[n] = out.Output
				logs = append(logs, fmt.Sprintf("______________________\n| TaskNode | Level == %d |\n*******************************\nLabel: %s\n\nInstructions:\n%s\n\n%s:\n%s\n", depth, dagLabel(n.Label), n.Instructions, n.OutputLabel, goeval.PythonString(out.Output)))
			case *BinaryJudgementNode:
				var out struct {
					Verdict *bool   `json:"verdict"`
					Reason  *string `json:"reason"`
				}
				if err := judge(ctx, llm, "BinaryJudgement.generate_binary_verdict", map[string]any{"criteria": n.Criteria, "text": resolve(n, "\n\n")}, schema(map[string]any{"verdict": map[string]any{"type": "boolean"}, "reason": str()}), &out); err != nil {
					return err
				}
				if out.Verdict == nil || out.Reason == nil {
					return fmt.Errorf("judge: missing binary judgement")
				}
				choices[n] = *out.Verdict
				logs = append(logs, dagJudgementLog(true, depth, n.Label, n.Criteria, *out.Verdict, *out.Reason))
			case *NonBinaryJudgementNode:
				options := make([]string, len(n.Children))
				for i, v := range n.Children {
					options[i] = v.Verdict.(string)
				}
				var out struct {
					Verdict *string `json:"verdict"`
					Reason  *string `json:"reason"`
				}
				if err := judge(ctx, llm, "NonBinaryJudgement.generate_non_binary_verdict", map[string]any{"criteria": n.Criteria, "text": resolve(n, "\n"), "options": options}, schema(map[string]any{"verdict": map[string]any{"type": "string", "enum": options}, "reason": str()}), &out); err != nil {
					return err
				}
				if out.Verdict == nil || out.Reason == nil {
					return fmt.Errorf("judge: missing nonbinary judgement")
				}
				valid := false
				for _, o := range options {
					if o == *out.Verdict {
						valid = true
					}
				}
				if !valid {
					return fmt.Errorf("judge: unknown verdict %q", *out.Verdict)
				}
				choices[n] = *out.Verdict
				logs = append(logs, dagJudgementLog(false, depth, n.Label, n.Criteria, *out.Verdict, *out.Reason))
			}
			for _, child := range edges(n) {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		for _, root := range g.roots {
			if err := visit(root, 0); err != nil {
				return 0, "", nil, err
			}
		}
		if !hasScore {
			return 0, "", nil, fmt.Errorf("judge: DAG did not reach a scoring verdict")
		}
		return finalScore, finalReason, map[string]any{"steps": logs}, nil
	})
	m.model = llm
	for node := range g.indegree {
		switch node := node.(type) {
		case *TaskNode, *BinaryJudgementNode, *NonBinaryJudgementNode:
			m.requiresModel = true
		case *VerdictNode:
			if node.Metric != nil {
				if err := goeval.ValidateMetrics([]goeval.Metric{node.Metric}); err != nil {
					return nil, err
				}
			}
		}
	}
	return m, nil
}

func dagLabel(label string) string {
	if label == "" {
		return "None"
	}
	return label
}
func dagJudgementLog(binary bool, depth int, label, criteria string, verdict any, reason string) string {
	name, underscores, stars := "NonBinaryJudgementNode", 37, 53
	if binary {
		name, underscores, stars = "BinaryJudgementNode", 34, 48
	}
	return fmt.Sprintf("%s\n| %s | Level == %d |\n%s\nLabel: %s\n\nCriteria:\n%s\n\nVerdict: %s\nReason: %s\n", strings.Repeat("_", underscores), name, depth, strings.Repeat("*", stars), dagLabel(label), criteria, goeval.PythonString(verdict), reason)
}
func validTaskOutput(value any) bool {
	switch v := value.(type) {
	case string:
		return true
	case []any:
		for _, x := range v {
			if _, ok := x.(string); !ok {
				return false
			}
		}
		return true
	case map[string]any:
		for _, x := range v {
			if _, ok := x.(string); !ok {
				return false
			}
		}
		return true
	}
	return false
}
