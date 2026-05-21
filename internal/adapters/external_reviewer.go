package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/sensors"
)

// ExternalReviewer is the optional inferential-review adapter. It shells
// out to a user-configured CLI (typically an LLM-backed reviewer such as
// `claude code --agent harness-output-reviewer`) and parses a structured
// JSON response into Harness findings.
//
// The harness binary itself never imports any LLM SDK; this adapter
// stays a deterministic shell-out. Disabled by default; configured via:
//
//	# .harness/config.yaml
//	review:
//	  command: ["claude", "code", "--agent", "harness-output-reviewer"]
//	  timeout_seconds: 600
//	adapters:
//	  review: [external-reviewer]
//	thresholds:
//	  review: 70
//	weights:
//	  review: 10
//
// I/O contract with the configured command:
//
//   - stdin: JSON bundle {schema_version, contract_path, contract_md,
//     harness_dir, repo_root}
//   - stdout: JSON {schema_version, findings: [...]}
//   - exit 0: success (even when findings are non-empty)
//   - exit != 0: invocation error; harness records a missing-sensor-style
//     finding and the dimension fails
//
// See docs/INFERENTIAL_REVIEWER.md for the full contract and a sample
// reviewer script.
type ExternalReviewer struct{}

// Name implements sensors.Sensor.
func (ExternalReviewer) Name() string { return "external-reviewer" }

// Dimension implements sensors.Sensor.
func (ExternalReviewer) Dimension() sensors.Dimension { return sensors.DimReview }

// Available returns true when the project's config declares a review
// command. Tools-on-PATH discovery is deferred to Run so a misconfigured
// command produces a useful error instead of silently disabling QA.
func (ExternalReviewer) Available(root string) bool {
	cfgPath := filepath.Join(root, ".harness", "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return false
	}
	return len(cfg.Review.Command) > 0
}

// reviewInput is the JSON document piped to the reviewer command.
type reviewInput struct {
	SchemaVersion string `json:"schema_version"`
	HarnessDir    string `json:"harness_dir"`
	RepoRoot      string `json:"repo_root"`
	ContractPath  string `json:"contract_path,omitempty"`
	ContractMD    string `json:"contract_md,omitempty"`
}

// reviewOutput is the JSON document the reviewer command emits on stdout.
type reviewOutput struct {
	SchemaVersion string          `json:"schema_version"`
	Findings      []reviewFinding `json:"findings"`
}

type reviewFinding struct {
	RequirementID string `json:"requirement_id,omitempty"`
	Severity      string `json:"severity"`
	Rule          string `json:"rule"`
	File          string `json:"file,omitempty"`
	Line          int    `json:"line,omitempty"`
	Message       string `json:"message"`
	Suggestion    string `json:"suggestion,omitempty"`
}

// Run invokes the configured reviewer command. Findings emitted by the
// reviewer are converted into sensors.Finding records and enriched with
// fingerprints so they participate in trend analysis.
func (ExternalReviewer) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	harnessDir := filepath.Join(root, ".harness")
	cfg, err := config.Load(filepath.Join(harnessDir, "config.yaml"))
	if err != nil {
		return sensors.Result{
			SensorName:  ExternalReviewer{}.Name(),
			Dimension:   sensors.DimReview,
			RawScore:    0,
			Duration:    time.Since(start),
			ToolMissing: true,
			Error:       fmt.Sprintf("load config: %v", err),
		}
	}
	if len(cfg.Review.Command) == 0 {
		return sensors.Result{
			SensorName:  ExternalReviewer{}.Name(),
			Dimension:   sensors.DimReview,
			RawScore:    0,
			Duration:    time.Since(start),
			ToolMissing: true,
			Error:       "review.command is not set in .harness/config.yaml",
		}
	}

	contractPath, contractMD := activeContract(harnessDir)
	bundle := reviewInput{
		SchemaVersion: "1",
		HarnessDir:    harnessDir,
		RepoRoot:      root,
		ContractPath:  contractPath,
		ContractMD:    contractMD,
	}
	payload, err := json.Marshal(bundle)
	if err != nil {
		return sensors.Result{SensorName: ExternalReviewer{}.Name(), Dimension: sensors.DimReview,
			Duration: time.Since(start), Error: err.Error()}
	}

	timeout := time.Duration(cfg.Review.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, cfg.Review.Command[0], cfg.Review.Command[1:]...)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = root

	if err := cmd.Run(); err != nil {
		errMsg := err.Error()
		if stderr.Len() > 0 {
			errMsg = fmt.Sprintf("%s: %s", err.Error(), strings.TrimSpace(stderr.String()))
		}
		return sensors.Result{
			SensorName: ExternalReviewer{}.Name(),
			Dimension:  sensors.DimReview,
			RawScore:   0,
			Duration:   time.Since(start),
			Error:      errMsg,
		}
	}

	var out reviewOutput
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
		return sensors.Result{
			SensorName: ExternalReviewer{}.Name(),
			Dimension:  sensors.DimReview,
			RawScore:   0,
			Duration:   time.Since(start),
			Error:      fmt.Sprintf("parse reviewer output: %v", err),
		}
	}

	findings := make([]sensors.Finding, 0, len(out.Findings))
	for _, rf := range out.Findings {
		message := rf.Message
		if rf.RequirementID != "" {
			message = fmt.Sprintf("[%s] %s", rf.RequirementID, message)
		}
		f := sensors.Finding{
			Dimension: sensors.DimReview,
			Severity:  severityFromString(rf.Severity),
			File:      rf.File,
			Line:      rf.Line,
			Rule:      stringOr(rf.Rule, "external-reviewer"),
			Message:   message,
			Hint:      rf.Suggestion,
		}
		f.Fingerprint = sensors.Fingerprint(f.Dimension, f.File, f.Rule, f.Message)
		findings = append(findings, f)
	}

	// Score model: 100 when reviewer reports zero findings; subtract 10
	// per finding so a typical handful of issues still leaves a usable
	// score for the dimension. Severity is reflected in finding rank,
	// not score, to keep the formula transparent.
	score := 100 - len(findings)*10
	if score < 0 {
		score = 0
	}

	return sensors.Result{
		SensorName: ExternalReviewer{}.Name(),
		Dimension:  sensors.DimReview,
		RawScore:   score,
		Duration:   time.Since(start),
		Findings:   findings,
	}
}

// activeContract returns the latest sprint contract path and content,
// or empty values when there is no sprint contract yet (in which case
// the reviewer is expected to do general project review).
func activeContract(harnessDir string) (string, string) {
	dir := filepath.Join(harnessDir, "contracts")
	entries, err := readDirSafe(dir)
	if err != nil {
		return "", ""
	}
	latest := ""
	for _, name := range entries {
		if !strings.HasPrefix(name, "sprint-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		if name > latest {
			latest = name
		}
	}
	if latest == "" {
		return "", ""
	}
	path := filepath.Join(dir, latest)
	body, err := readFileSafe(path)
	if err != nil {
		return "", ""
	}
	return path, body
}

func severityFromString(s string) sensors.Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return sensors.SeverityCritical
	case "high":
		return sensors.SeverityHigh
	case "medium":
		return sensors.SeverityMedium
	case "low":
		return sensors.SeverityLow
	case "info":
		return sensors.SeverityInfo
	}
	return sensors.SeverityMedium
}

func stringOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
