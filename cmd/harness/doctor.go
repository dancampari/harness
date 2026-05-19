package harness

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dancampari/harness/internal/adapters"
	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/detect"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check Harness config and required local sensors",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(".")
		},
	}
}

func runDoctor(root string) error {
	project := detect.DetectProject(root)
	cfg, err := loadDoctorConfig(root, project.Stack)
	if err != nil {
		return err
	}
	fmt.Println("Harness doctor")
	fmt.Printf("  Project: %s\n", valueOr(project.Name, "unknown"))
	fmt.Printf("  Stack: %s\n", valueOr(project.Stack, "unknown"))
	if project.PackageManager != "" {
		fmt.Printf("  Package manager: %s\n", project.PackageManager)
	}
	if len(project.Frameworks) > 0 {
		fmt.Printf("  Frameworks: %s\n", joinList(project.Frameworks))
	}

	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Println()
		fmt.Println("Config errors:")
		for _, err := range errs {
			fmt.Println("  FAIL", err)
		}
		return nil
	}

	reg := adapters.BuildRegistry()
	fmt.Println()
	fmt.Println("Active dimensions:")
	for _, dim := range cfg.ActiveDimensions() {
		fmt.Printf("  %s threshold=%d weight=%d\n", dim, cfg.ThresholdFor(dim), cfg.WeightFor(dim))
		if dim == config.DimContract {
			fmt.Println("    OK contract-validator built in")
			continue
		}
		names := cfg.AdapterNamesForDimension(dim)
		if len(names) == 0 {
			fmt.Println("    FAIL no sensors configured")
			continue
		}
		for _, name := range names {
			s, ok := reg.ByName(name)
			switch {
			case !ok:
				fmt.Printf("    FAIL %-18s not registered in this binary\n", name)
			case s.Available(root):
				fmt.Printf("    OK   %-18s available\n", name)
			default:
				fmt.Printf("    MISS %-18s %s\n", name, installHint(name))
			}
		}
	}
	return nil
}

func loadDoctorConfig(root, stack string) (config.Config, error) {
	path := filepath.Join(root, ".harness", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		return config.DefaultFor(stack), nil
	}
	return config.Load(path)
}

func installHint(sensor string) string {
	switch sensor {
	case "eslint":
		return "install eslint and add an eslint.config.* file"
	case "jest", "jest-coverage":
		return "install jest and keep tests runnable with npx jest"
	case "vitest", "vitest-coverage":
		return "install vitest and @vitest/coverage-v8 when coverage is active"
	case "npm-audit":
		return "ensure npm is on PATH and package-lock.json exists"
	case "playwright":
		return "install @playwright/test and add playwright.config.*"
	case "js-complexity", "js-architecture":
		return "built-in static sensor should be available for Node/TS projects"
	default:
		return "install or disable this sensor in .harness/config.yaml"
	}
}
