package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/memory"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var force bool
	var cli string
	var installHooks bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .harness/ in the current repo",
		Long: `Creates the .harness/ directory with:
  - config.yaml (auto-detected for your stack)
  - spec.md (template you should fill in)
  - progress.md (the narrative brain of the project)
  - contracts/, evaluations/, screenshots/, reports/ (empty dirs)
  - memory.db (SQLite index, initialized)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(initOptions{
				Force:        force,
				CLI:          cli,
				InstallHooks: installHooks,
			})
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing .harness/")
	cmd.Flags().StringVar(&cli, "cli", "auto", "coding CLI to configure: auto|codex|claude|cursor|all|none")
	cmd.Flags().BoolVar(&installHooks, "install-hooks", false, "install coding CLI references during init")
	return cmd
}

type initOptions struct {
	Force        bool
	CLI          string
	InstallHooks bool
}

func runInit(opts initOptions) error {
	root := ".harness"
	if _, err := os.Stat(root); err == nil && !opts.Force {
		return errors.New(".harness/ already exists - use --force to overwrite")
	}

	dirs := []string{
		root,
		filepath.Join(root, "contracts"),
		filepath.Join(root, "evaluations"),
		filepath.Join(root, "screenshots"),
		filepath.Join(root, "reports"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	project := detect.DetectProject(".")
	cfg := config.DefaultFor(project.Stack)
	if err := config.Save(filepath.Join(root, "config.yaml"), cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if err := writeTemplate(filepath.Join(root, "spec.md"), specTemplate); err != nil {
		return err
	}
	if err := writeTemplate(filepath.Join(root, "progress.md"), progressTemplate); err != nil {
		return err
	}

	db, err := memory.Open(filepath.Join(root, "memory.db"))
	if err != nil {
		return fmt.Errorf("init memory: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("migrate memory: %w", err)
	}

	gi := filepath.Join(root, ".gitignore")
	_ = os.WriteFile(gi, []byte("memory.db\nreports/\nscreenshots/\n"), 0o644)

	fmt.Println("OK .harness/ initialized")
	fmt.Printf("  Project: %s\n", valueOr(project.Name, "unknown"))
	fmt.Printf("  Stack: %s\n", valueOr(project.Stack, "unknown"))
	if project.PackageManager != "" {
		fmt.Printf("  Package manager: %s\n", project.PackageManager)
	}
	if len(project.Frameworks) > 0 {
		fmt.Printf("  Frameworks: %s\n", joinList(project.Frameworks))
	}
	if len(project.CodingCLIs) > 0 {
		fmt.Printf("  Coding CLI markers: %s\n", joinList(project.CodingCLIs))
	}

	shouldInstallHooks := opts.InstallHooks || opts.CLI != "auto"
	if shouldInstallHooks {
		if err := runInstallHooks(installHookOptions{
			CLI:         opts.CLI,
			Interactive: isTerminal(os.Stdin) && opts.CLI == "auto",
			InstallGit:  true,
		}); err != nil {
			return err
		}
	}

	fmt.Println("  Next steps:")
	fmt.Println("    1. Edit .harness/spec.md with your product spec")
	fmt.Println("    2. harness install-hooks --interactive    # choose Codex, Claude Code, or Cursor")
	fmt.Println("    3. harness sprint new \"first goal\"")
	return nil
}

func writeTemplate(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // do not overwrite user content
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

const specTemplate = `# Product Specification

## Vision
<what you are building, in 2-3 sentences>

## Personas
- ...

## Core Features
- ...

## Non-Goals
- ...

## Constraints
- runtime: ...
- deployment: ...
- compliance: ...

## Acceptance Bar
Define here the global criteria that every sprint must satisfy.
The harness validates each sprint's contract against this bar.
`

const progressTemplate = `# Project Progress

This file is the narrative brain of the project. It is append-only, versioned
in Git, and read by every CLI as the first source of truth when resuming work.

## History

<!-- harness append below -->
`
