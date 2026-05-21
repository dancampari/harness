package sprint

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/charmbracelet/lipgloss"
)

// WriteJSON emits the QA result as indented JSON. Stable output: dimensions
// sorted, findings sorted by severity desc — so diffs across runs are
// meaningful for memory.db dedup.
func (q *QAResult) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(q.result)
}

// WriteTTY emits a styled report. Inspired by image 10 of the PBQ:
// a single block with verdict, dimension scores, top findings.
func (q *QAResult) WriteTTY(w io.Writer) error {
	r := q.result
	verdictStyle := lipgloss.NewStyle().Bold(true)
	if r.Verdict == "PASS" {
		verdictStyle = verdictStyle.Foreground(lipgloss.Color("10"))
	} else {
		verdictStyle = verdictStyle.Foreground(lipgloss.Color("9"))
	}
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	passStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	fmt.Fprintln(w, headerStyle.Render(fmt.Sprintf("┌─ harness sprint qa · sprint %03d ──────────────────────────────", q.sprintNumber)))
	fmt.Fprintln(w, "│")
	fmt.Fprintf(w, "│  Verdict: %s    Total Score: %d/100\n", verdictStyle.Render(r.Verdict), r.TotalScore)
	fmt.Fprintln(w, "│")
	fmt.Fprintln(w, "│  ┌─ Dimension      Score   Threshold   Passed   Findings  ──┐")

	names := make([]string, 0, len(r.Dimensions))
	for k := range r.Dimensions {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, n := range names {
		d := r.Dimensions[n]
		mark := passStyle.Render("✓")
		if !d.Passed {
			mark = failStyle.Render("✗")
		}
		fmt.Fprintf(w, "│  │  %-14s  %3d     %3d         %s        %3d       │\n",
			dimStyle.Render(n), d.Score, d.Threshold, mark, len(d.Findings))
	}
	fmt.Fprintln(w, "│  └────────────────────────────────────────────────────────┘")

	// Top findings: up to 5, sorted by severity desc.
	var all []findingForSort
	for _, d := range r.Dimensions {
		for _, f := range d.Findings {
			all = append(all, findingForSort{
				msg:  fmt.Sprintf("[%s] %s:%d  %s", f.Severity, f.File, f.Line, f.Message),
				rank: sevRank(string(f.Severity)),
			})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].rank > all[j].rank })
	if len(all) > 0 {
		fmt.Fprintln(w, "│")
		fmt.Fprintln(w, "│  Top findings:")
		for i, f := range all {
			if i >= 5 {
				break
			}
			fmt.Fprintf(w, "│    %s\n", f.msg)
		}
	}
	if r.ContractCheck.Status != "satisfied" {
		fmt.Fprintln(w, "│")
		fmt.Fprintf(w, "│  Contract: %s\n", r.ContractCheck.Status)
		for _, d := range r.ContractCheck.MissingDeliverables {
			fmt.Fprintf(w, "│    missing: %s\n", d)
		}
	}
	fmt.Fprintln(w, "│")
	fmt.Fprintf(w, "│  Evaluation: .harness/evaluations/sprint-%03d.md\n", q.sprintNumber)
	fmt.Fprintf(w, "│  Report: .harness/reports/sprint-%03d.json\n", q.sprintNumber)
	fmt.Fprintln(w, "└────────────────────────────────────────────────────────────────")
	return nil
}

// EvaluationPath returns the human-readable markdown report path.
func (q *QAResult) EvaluationPath() string {
	return q.evaluationPath
}

// ReportPath returns the machine-readable JSON report path.
func (q *QAResult) ReportPath() string {
	return q.reportPath
}

// Verdict returns "PASS" or "FAIL" so the CLI can map the value to a
// process exit code (needed by the pre-commit hook).
func (q *QAResult) Verdict() string {
	if q == nil || q.result == nil {
		return ""
	}
	return q.result.Verdict
}

type findingForSort struct {
	msg  string
	rank int
}

func sevRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}
