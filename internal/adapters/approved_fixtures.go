package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/sensors"
)

type ApprovedFixtures struct{}

type approvedFixture struct {
	SchemaVersion  string            `json:"schema_version,omitempty"`
	Name           string            `json:"name"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Workdir        string            `json:"workdir,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Stdin          string            `json:"stdin,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Expect         fixtureExpect     `json:"expect,omitempty"`
}

type fixtureExpect struct {
	ExitCode       *int     `json:"exit_code,omitempty"`
	Stdout         *string  `json:"stdout,omitempty"`
	Stderr         *string  `json:"stderr,omitempty"`
	StdoutContains []string `json:"stdout_contains,omitempty"`
	StderrContains []string `json:"stderr_contains,omitempty"`
}

type fixtureRun struct {
	exitCode int
	stdout   string
	stderr   string
}

func (ApprovedFixtures) Name() string { return "approved-fixtures" }

func (ApprovedFixtures) Dimension() sensors.Dimension { return sensors.DimBehavior }

func (ApprovedFixtures) Available(root string) bool {
	files, err := approvedFixtureFiles(root)
	return err == nil && len(files) > 0
}

func (a ApprovedFixtures) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: a.Name(),
		Dimension:  a.Dimension(),
		RawScore:   100,
	}

	files, err := approvedFixtureFiles(root)
	if err != nil {
		res.Error = err.Error()
		res.Duration = time.Since(start)
		return res
	}
	if len(files) == 0 {
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimBehavior,
			sensors.SeverityHigh,
			".harness/fixtures",
			0,
			"no-approved-fixtures",
			"behavior dimension is active but no approved fixture JSON files were found",
		))
		res.Duration = time.Since(start)
		return res
	}

	accept := os.Getenv("HARNESS_ACCEPT_FIXTURES") == "1"
	for _, path := range files {
		fixture, parseErr := readApprovedFixture(path)
		rel := slashRel(root, path)
		if parseErr != nil {
			res.Findings = append(res.Findings, finding(
				sensors.DimBehavior,
				sensors.SeverityHigh,
				rel,
				0,
				"fixture-parse-error",
				parseErr.Error(),
			))
			continue
		}
		run, runErr := runApprovedFixture(ctx, root, fixture)
		if runErr != nil {
			res.Findings = append(res.Findings, finding(
				sensors.DimBehavior,
				sensors.SeverityHigh,
				rel,
				0,
				"fixture-execution-error",
				runErr.Error(),
			))
			continue
		}
		if accept {
			if err := writeAcceptedFixture(path, fixture, run); err != nil {
				res.Findings = append(res.Findings, finding(
					sensors.DimBehavior,
					sensors.SeverityMedium,
					rel,
					0,
					"fixture-accept-failed",
					err.Error(),
				))
			}
			continue
		}
		res.Findings = append(res.Findings, compareFixture(rel, fixture, run)...)
	}

	res.RawScore = clampScore(100 - len(res.Findings)*20)
	res.Duration = time.Since(start)
	return res
}

func approvedFixtureFiles(root string) ([]string, error) {
	dir := filepath.Join(root, ".harness", "fixtures")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	return files, nil
}

func readApprovedFixture(path string) (approvedFixture, error) {
	var fixture approvedFixture
	b, err := os.ReadFile(path)
	if err != nil {
		return fixture, err
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		return fixture, err
	}
	if strings.TrimSpace(fixture.Command) == "" {
		return fixture, fmt.Errorf("fixture command is required")
	}
	if fixture.Name == "" {
		fixture.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return fixture, nil
}

func runApprovedFixture(parent context.Context, root string, fixture approvedFixture) (fixtureRun, error) {
	timeout := time.Duration(fixture.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, fixture.Command, fixture.Args...)
	cmd.Dir = root
	if fixture.Workdir != "" {
		cmd.Dir = filepath.Join(root, filepath.FromSlash(fixture.Workdir))
	}
	cmd.Env = os.Environ()
	for key, value := range fixture.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if fixture.Stdin != "" {
		cmd.Stdin = strings.NewReader(fixture.Stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return fixtureRun{}, fmt.Errorf("fixture %q timed out after %s", fixture.Name, timeout)
	}
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return fixtureRun{}, err
		}
	}
	return fixtureRun{
		exitCode: exitCode,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
	}, nil
}

func compareFixture(rel string, fixture approvedFixture, run fixtureRun) []sensors.Finding {
	if fixture.Expect.ExitCode == nil &&
		fixture.Expect.Stdout == nil &&
		fixture.Expect.Stderr == nil &&
		len(fixture.Expect.StdoutContains) == 0 &&
		len(fixture.Expect.StderrContains) == 0 {
		return []sensors.Finding{finding(
			sensors.DimBehavior,
			sensors.SeverityHigh,
			rel,
			0,
			"fixture-baseline-missing",
			"fixture has no approved expectations; review behavior and rerun QA with --accept-fixtures if correct",
		)}
	}

	expectedExit := 0
	if fixture.Expect.ExitCode != nil {
		expectedExit = *fixture.Expect.ExitCode
	}
	var findings []sensors.Finding
	if run.exitCode != expectedExit {
		findings = append(findings, fixtureRegression(rel,
			fmt.Sprintf("exit code %d does not match approved exit code %d", run.exitCode, expectedExit)))
	}
	if fixture.Expect.Stdout != nil && run.stdout != *fixture.Expect.Stdout {
		findings = append(findings, fixtureRegression(rel, "stdout does not match approved output"))
	}
	if fixture.Expect.Stderr != nil && run.stderr != *fixture.Expect.Stderr {
		findings = append(findings, fixtureRegression(rel, "stderr does not match approved output"))
	}
	for _, expected := range fixture.Expect.StdoutContains {
		if !strings.Contains(run.stdout, expected) {
			findings = append(findings, fixtureRegression(rel,
				fmt.Sprintf("stdout does not contain approved text %q", expected)))
		}
	}
	for _, expected := range fixture.Expect.StderrContains {
		if !strings.Contains(run.stderr, expected) {
			findings = append(findings, fixtureRegression(rel,
				fmt.Sprintf("stderr does not contain approved text %q", expected)))
		}
	}
	return findings
}

func fixtureRegression(rel, message string) sensors.Finding {
	return finding(
		sensors.DimBehavior,
		sensors.SeverityHigh,
		rel,
		0,
		"fixture-regression",
		message,
	)
}

func writeAcceptedFixture(path string, fixture approvedFixture, run fixtureRun) error {
	if fixture.SchemaVersion == "" {
		fixture.SchemaVersion = "1"
	}
	fixture.Expect.ExitCode = &run.exitCode
	fixture.Expect.Stdout = &run.stdout
	fixture.Expect.Stderr = &run.stderr
	fixture.Expect.StdoutContains = nil
	fixture.Expect.StderrContains = nil
	b, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}
