package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/sensors"
)

// Playwright is the E2E sensor. This is critical for killing "Teste Fake"
// (problem 4 of the video): a curl /api returning 200 doesn't prove the
// button clicks, the form submits, the redirect lands, or the CSS aligns.
//
// Playwright runs a real browser, takes screenshots, and we compare them
// against baselines stored in .harness/screenshots/baseline/.
type Playwright struct{}

func (Playwright) Name() string                 { return "playwright" }
func (Playwright) Dimension() sensors.Dimension { return sensors.DimE2E }

func (Playwright) Available(root string) bool {
	if !detect.HasFile(root, "package.json") {
		return false
	}
	// Look for playwright config in any of its forms.
	configs := []string{"playwright.config.ts", "playwright.config.js",
		"playwright.config.mjs", "playwright.config.cjs"}
	for _, c := range configs {
		if detect.HasFile(root, c) {
			return hasNodeBin(root, "playwright")
		}
	}
	return false
}

// pwReport is the shape of Playwright's JSON reporter output. We only
// need a subset; the reporter emits a lot more (timings, attachments,
// stdio, etc.) that we ignore.
type pwReport struct {
	Stats struct {
		Expected   int `json:"expected"`
		Unexpected int `json:"unexpected"`
		Flaky      int `json:"flaky"`
		Skipped    int `json:"skipped"`
	} `json:"stats"`
	Suites []pwSuite `json:"suites"`
}

type pwSuite struct {
	Title  string    `json:"title"`
	File   string    `json:"file"`
	Specs  []pwSpec  `json:"specs"`
	Suites []pwSuite `json:"suites"`
}

type pwSpec struct {
	Title string   `json:"title"`
	File  string   `json:"file"`
	Line  int      `json:"line"`
	Ok    bool     `json:"ok"`
	Tests []pwTest `json:"tests"`
}

type pwTest struct {
	Status  string `json:"status"`
	Results []struct {
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
		Attachments []struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			ContentType string `json:"contentType"`
		} `json:"attachments,omitempty"`
	} `json:"results"`
}

func (p Playwright) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: p.Name(),
		Dimension:  p.Dimension(),
	}

	// Direct playwright to emit a JSON reporter into a temp file. We use
	// PLAYWRIGHT_JSON_OUTPUT_NAME for compatibility with --reporter=json
	// across versions.
	reportPath := filepath.Join(root, ".harness", "reports", "playwright.json")
	_ = os.MkdirAll(filepath.Dir(reportPath), 0o755)
	cmd := nodeToolCommand(ctx, root, "playwright", "test",
		"--reporter=json",
		"--output=.harness/playwright/results")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_JSON_OUTPUT_NAME="+reportPath,
		"CI=1", // forces headless, deterministic
	)
	out, err := cmd.Output()
	res.Duration = time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.ToolMissing = true
			res.Error = err.Error()
			return res
		}
	}

	// The reporter writes to stdout by default; try parsing that first.
	var report pwReport
	if jsonErr := json.Unmarshal(out, &report); jsonErr != nil {
		reportBytes, readErr := os.ReadFile(reportPath)
		if readErr != nil {
			res.Error = fmt.Sprintf("parse playwright output: %v; read %s: %v", jsonErr, slashRel(root, reportPath), readErr)
			return res
		}
		if fileErr := json.Unmarshal(reportBytes, &report); fileErr != nil {
			res.Error = fmt.Sprintf("parse playwright report %s: %v", slashRel(root, reportPath), fileErr)
			return res
		}
	}

	walkSuites(&res, root, report.Suites)
	processPlaywrightScreenshots(&res, root, report.Suites)

	total := report.Stats.Expected + report.Stats.Unexpected + report.Stats.Flaky
	if total == 0 {
		res.RawScore = 0
		res.Findings = append(res.Findings, sensors.Finding{
			Dimension: sensors.DimE2E,
			Severity:  sensors.SeverityHigh,
			Rule:      "no-e2e-tests",
			Message:   "no Playwright tests discovered — E2E coverage is zero",
		})
		return res
	}
	passing := report.Stats.Expected
	res.RawScore = int(float64(passing) / float64(total) * 100)
	if report.Stats.Unexpected > 0 && res.RawScore > 50 {
		res.RawScore = 50
	}
	if hasRule(res.Findings, "visual-regression") || hasRule(res.Findings, "screenshot-baseline-missing") {
		if res.RawScore > 50 {
			res.RawScore = 50
		}
	}
	return res
}

func walkSuites(res *sensors.Result, root string, suites []pwSuite) {
	for _, s := range suites {
		for _, spec := range s.Specs {
			for _, t := range spec.Tests {
				for _, r := range t.Results {
					if r.Status == "passed" {
						continue
					}
					msg := ""
					if r.Error != nil {
						msg = r.Error.Message
					}
					rel, _ := filepath.Rel(root, spec.File)
					f := sensors.Finding{
						Dimension: sensors.DimE2E,
						Severity:  sensors.SeverityCritical,
						File:      rel,
						Line:      spec.Line,
						Rule:      "e2e-failure",
						Message:   fmt.Sprintf("%s: %s", spec.Title, truncateMessage(msg, 200)),
					}
					f.Fingerprint = sensors.Fingerprint(f.Dimension, f.File, f.Rule, spec.Title)
					res.Findings = append(res.Findings, f)
				}
			}
		}
		walkSuites(res, root, s.Suites)
	}
}

func processPlaywrightScreenshots(res *sensors.Result, root string, suites []pwSuite) {
	accept := os.Getenv("HARNESS_ACCEPT_SCREENSHOTS") == "1"
	currentDir := filepath.Join(root, ".harness", "screenshots", "current")
	baselineDir := filepath.Join(root, ".harness", "screenshots", "baseline")
	diffDir := filepath.Join(root, ".harness", "screenshots", "diff")
	_ = os.MkdirAll(currentDir, 0o755)
	_ = os.MkdirAll(baselineDir, 0o755)
	_ = os.MkdirAll(diffDir, 0o755)

	for _, attachment := range screenshotAttachments(suites) {
		name := safeScreenshotName(attachment.name, attachment.path)
		current := filepath.Join(currentDir, name)
		baseline := filepath.Join(baselineDir, name)
		if err := copyFile(attachment.path, current); err != nil {
			res.Findings = append(res.Findings, finding(
				sensors.DimE2E,
				sensors.SeverityMedium,
				slashRel(root, attachment.path),
				0,
				"screenshot-copy-failed",
				err.Error(),
			))
			continue
		}
		if _, err := os.Stat(baseline); os.IsNotExist(err) {
			if accept {
				_ = copyFile(current, baseline)
				continue
			}
			res.Findings = append(res.Findings, finding(
				sensors.DimE2E,
				sensors.SeverityHigh,
				slashRel(root, current),
				0,
				"screenshot-baseline-missing",
				fmt.Sprintf("baseline missing for %s; rerun with --accept-screenshots after review", name),
			))
			continue
		}
		diffPixels, totalPixels, err := pngPixelDiff(current, baseline)
		if err != nil {
			res.Findings = append(res.Findings, finding(
				sensors.DimE2E,
				sensors.SeverityMedium,
				slashRel(root, current),
				0,
				"screenshot-compare-failed",
				err.Error(),
			))
			continue
		}
		if diffPixels > 0 {
			res.Findings = append(res.Findings, finding(
				sensors.DimE2E,
				sensors.SeverityHigh,
				slashRel(root, current),
				0,
				"visual-regression",
				fmt.Sprintf("%s differs from baseline: %d/%d pixels changed", name, diffPixels, totalPixels),
			))
		}
	}
}

type screenshotAttachment struct {
	name string
	path string
}

func screenshotAttachments(suites []pwSuite) []screenshotAttachment {
	var out []screenshotAttachment
	var walk func([]pwSuite)
	walk = func(suites []pwSuite) {
		for _, suite := range suites {
			for _, spec := range suite.Specs {
				for _, test := range spec.Tests {
					for _, result := range test.Results {
						for _, attachment := range result.Attachments {
							if attachment.Path != "" && attachment.ContentType == "image/png" {
								out = append(out, screenshotAttachment{name: attachment.Name, path: attachment.Path})
							}
						}
					}
				}
			}
			walk(suite.Suites)
		}
	}
	walk(suites)
	return out
}

func safeScreenshotName(name, path string) string {
	base := name
	if base == "" {
		base = filepath.Base(path)
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\"", "", "'", "")
	base = replacer.Replace(base)
	if base == "" {
		base = "screenshot"
	}
	return base + ".png"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func pngPixelDiff(aPath, bPath string) (int, int, error) {
	a, err := decodePNG(aPath)
	if err != nil {
		return 0, 0, err
	}
	b, err := decodePNG(bPath)
	if err != nil {
		return 0, 0, err
	}
	ab := a.Bounds()
	bb := b.Bounds()
	if ab.Dx() != bb.Dx() || ab.Dy() != bb.Dy() {
		return ab.Dx() * ab.Dy(), ab.Dx() * ab.Dy(), nil
	}
	diff := 0
	total := ab.Dx() * ab.Dy()
	for y := ab.Min.Y; y < ab.Max.Y; y++ {
		for x := ab.Min.X; x < ab.Max.X; x++ {
			ar, ag, abv, aa := a.At(x, y).RGBA()
			br, bg, bbv, ba := b.At(x, y).RGBA()
			if ar != br || ag != bg || abv != bbv || aa != ba {
				diff++
			}
		}
	}
	return diff, total, nil
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func hasRule(findings []sensors.Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
