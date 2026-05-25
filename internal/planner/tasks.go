package planner

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// TaskPlan is the parsed form of a tasks.md file. TLC's tasks.md format
// is a numbered list of tasks where each entry carries metadata lines
// describing what files it touches, what tests cover it, and what other
// tasks it depends on. The harness reads these structured lines for
// deterministic gates: granularity, scope guardrail, and the
// diagram-definition cross-check.
//
// Format example (one task):
//
//	## Task 001 — Implement user-auth
//	- Where: src/auth/user.ts, src/auth/user.test.ts
//	- Tests: tests user-auth happy path
//	- Depends on: 000
//	- Parallel: false
//	- Cohesive: false
//
// All metadata lines are optional but their absence triggers warnings
// from the granularity / scope validators.
//
// Diagram is the parsed dependency graph embedded in the tasks.md file
// — either a Mermaid block, an ASCII arrow diagram, or both. Empty
// when no diagram is present; the diagram-definition validator skips
// without complaint in that case.
type TaskPlan struct {
	Tasks   []Task
	Diagram DependencyDiagram
}

// DependencyEdge is one directed `from → to` pair extracted from the
// dependency diagram. Both ends are normalised to the same form TLC
// uses for task IDs (numeric, with leading zeros stripped) so the
// cross-check can compare against the `Depends on:` field.
type DependencyEdge struct {
	From string
	To   string
}

// DependencyDiagram is the union of every edge declared in any diagram
// block within tasks.md. Sources track which markdown fence the edge
// came from for diagnostics (mermaid, ascii); callers usually only
// care about Edges.
type DependencyDiagram struct {
	Edges []DependencyEdge
}

// Task is one numbered entry in tasks.md.
type Task struct {
	Number        int
	Title         string
	Where         []string // files this task is allowed to touch
	Tests         string
	DependsOn     []string
	Parallel      bool
	Cohesive      bool // explicit opt-out of the granularity check
	RequirementID string
}

// ParseTasks reads and parses a tasks.md file. Tolerates missing files
// (returns an empty plan + nil error) so callers can gate on emptiness
// without having to stat the path first.
func ParseTasks(path string) (*TaskPlan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &TaskPlan{}, nil
		}
		return nil, err
	}
	return parseTasks(string(b)), nil
}

var (
	taskHeadingRe = regexp.MustCompile(`(?i)^##\s+Task\s+(\d+)\s*[—\-:]?\s*(.*)$`)
	taskFieldRe   = regexp.MustCompile(`^[-*]\s*(Where|Tests|Depends on|Parallel|Cohesive|REQ)\s*:\s*(.+)$`)
	reqIDRe       = regexp.MustCompile(`REQ-\d+`)
)

func parseTasks(raw string) *TaskPlan {
	plan := &TaskPlan{}
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var current *Task
	var inCodeFence bool
	var fenceLang string
	var fenceBuf []string
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track fenced code blocks so we can extract dependency
		// diagrams without confusing them for task metadata.
		if strings.HasPrefix(trimmed, "```") {
			if inCodeFence {
				plan.Diagram.Edges = append(plan.Diagram.Edges,
					extractDiagramEdges(fenceLang, fenceBuf)...)
				inCodeFence = false
				fenceLang = ""
				fenceBuf = nil
				continue
			}
			inCodeFence = true
			fenceLang = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "```")))
			fenceBuf = fenceBuf[:0]
			continue
		}
		if inCodeFence {
			fenceBuf = append(fenceBuf, line)
			continue
		}

		if m := taskHeadingRe.FindStringSubmatch(trimmed); m != nil {
			if current != nil {
				plan.Tasks = append(plan.Tasks, *current)
			}
			var n int
			_, _ = parseInt(m[1], &n)
			current = &Task{Number: n, Title: strings.TrimSpace(m[2])}
			continue
		}
		if current == nil {
			continue
		}
		fm := taskFieldRe.FindStringSubmatch(trimmed)
		if fm == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(fm[1]))
		value := strings.TrimSpace(fm[2])
		switch key {
		case "where":
			current.Where = splitCSV(value)
		case "tests":
			current.Tests = value
		case "depends on":
			current.DependsOn = splitCSV(value)
		case "parallel":
			current.Parallel = parseBool(value)
		case "cohesive":
			current.Cohesive = parseBool(value)
		case "req":
			if m := reqIDRe.FindString(value); m != "" {
				current.RequirementID = m
			}
		}
	}
	if current != nil {
		plan.Tasks = append(plan.Tasks, *current)
	}
	return plan
}

// extractDiagramEdges parses the body of a fenced code block and
// returns every `from → to` edge it can find. Two formats are
// recognised:
//
//   - Mermaid (` ```mermaid `): lines like `T1 --> T2` or `001 --> 002`.
//   - ASCII (“ ``` “ or `text`): lines containing arrows `→`, `->`,
//     `=>` between simple identifiers, e.g. `T1 → T2 → T4`.
//
// Identifiers are normalised by stripping a leading `T` (case-
// insensitive) and trimming leading zeros so `T001`, `001`, `1`, and
// `t1` all compare equal. Anything that does not look like an edge is
// silently skipped — the validator only reports edges it could parse.
func extractDiagramEdges(lang string, lines []string) []DependencyEdge {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "mermaid":
		return parseMermaidEdges(lines)
	default:
		return parseASCIIEdges(lines)
	}
}

var (
	mermaidEdgeRe   = regexp.MustCompile(`(?i)^\s*([A-Za-z]?\w+)\s*--+>\s*([A-Za-z]?\w+)\s*$`)
	identifierRe    = regexp.MustCompile(`(?i)[A-Za-z]?\d+`)
	asciiArrowSplit = regexp.MustCompile(`\s*(?:→|->|=>)\s*`)
)

func parseMermaidEdges(lines []string) []DependencyEdge {
	var out []DependencyEdge
	for _, line := range lines {
		m := mermaidEdgeRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		out = append(out, DependencyEdge{
			From: normaliseTaskID(m[1]),
			To:   normaliseTaskID(m[2]),
		})
	}
	return out
}

func parseASCIIEdges(lines []string) []DependencyEdge {
	var out []DependencyEdge
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := asciiArrowSplit.Split(trimmed, -1)
		if len(parts) < 2 {
			continue
		}
		var ids []string
		for _, p := range parts {
			id := identifierRe.FindString(p)
			if id == "" {
				continue
			}
			ids = append(ids, normaliseTaskID(id))
		}
		for i := 1; i < len(ids); i++ {
			out = append(out, DependencyEdge{From: ids[i-1], To: ids[i]})
		}
	}
	return out
}

// normaliseTaskID strips an optional `T`/`t` prefix and leading zeros
// so cross-checks against `Depends on:` lines compare on the canonical
// numeric form.
func normaliseTaskID(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" {
		return ""
	}
	if len(id) > 1 && (id[0] == 'T' || id[0] == 't') {
		id = id[1:]
	}
	id = strings.TrimLeft(id, "0")
	if id == "" {
		return "0"
	}
	return id
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "y", "1":
		return true
	}
	return false
}

func parseInt(s string, dst *int) (int, error) {
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, nil
		}
		*dst = *dst*10 + int(r-'0')
	}
	return *dst, nil
}
