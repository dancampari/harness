package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/detect"
	"github.com/spf13/cobra"
)

type setupOptions struct {
	CLI      string
	Skills   string
	Scope    string
	Force    bool
	Yes      bool
	StartTUI bool
}

type setupChoices struct {
	CLI    string
	Skills bool
	Scope  string
}

func newSetupCmd(version string) *cobra.Command {
	var opts setupOptions
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "One-command project bootstrap",
		Long: `Initializes Harness, installs Codex/Claude/Cursor references, runs doctor,
and prints the command to open the live terminal dashboard.

If no coding CLI marker is detected and the command is running in a terminal,
Harness asks for the coding CLI, contract automation skills, and install
scope. In non-interactive mode, use --yes or explicit flags.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(opts, version)
		},
	}
	cmd.Flags().StringVar(&opts.CLI, "cli", "auto", "coding CLI to configure: auto|codex|claude|cursor|all|none")
	cmd.Flags().StringVar(&opts.Skills, "skills", "auto", "contract automation skills: auto|on|off")
	cmd.Flags().StringVar(&opts.Scope, "scope", "auto", "install scope: auto|project|global")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite existing .harness/")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "run setup with no prompts; installs all agent references if none are detected")
	cmd.Flags().BoolVar(&opts.StartTUI, "start", false, "launch the live TUI after setup")
	return cmd
}

func runSetup(opts setupOptions, version string) error {
	if opts.CLI == "" {
		opts.CLI = "auto"
	}
	if opts.Skills == "" {
		opts.Skills = "auto"
	}
	if opts.Scope == "" {
		opts.Scope = "auto"
	}

	fmt.Println("Harness setup")
	fmt.Println("  Goal: one command, minimal interaction, project ready for your coding CLI.")
	fmt.Println()

	project := detect.DetectProject(".")
	choices, err := setupWizard(opts, project)
	if err != nil {
		return err
	}

	if _, err := os.Stat(".harness"); err == nil && !opts.Force {
		fmt.Println("OK .harness/ already exists")
	} else {
		if err := runInit(initOptions{
			Force:         opts.Force,
			CLI:           "auto",
			InstallHooks:  false,
			InstallSkills: choices.Skills,
			Quiet:         true,
		}); err != nil {
			return err
		}
	}
	if err := installProjectCommand(); err != nil {
		return err
	}

	if err := runInstallHooks(installHookOptions{
		CLI:         choices.CLI,
		Skills:      boolSkillsMode(choices.Skills),
		Interactive: false,
		InstallGit:  true,
	}); err != nil {
		return err
	}
	if err := writeSetupState(choices, project); err != nil {
		return err
	}
	if choices.Scope == "global" {
		if err := installGlobalCommand(); err != nil {
			return err
		}
	}

	fmt.Println()
	if err := runDoctor("."); err != nil {
		return err
	}

	invoke := harnessInvocation()
	fmt.Println()
	fmt.Println("Ready.")
	fmt.Printf("  CLI references:             %s\n", choices.CLI)
	fmt.Printf("  Contract skills:            %s\n", enabledText(choices.Skills))
	fmt.Printf("  Install scope:              %s\n", choices.Scope)
	fmt.Printf("  Project command:            %s\n", harnessInvocation())
	fmt.Printf("  Open the Harness terminal: %s run --resume\n", invoke)
	fmt.Printf("  Start a sprint:             %s sprint new \"first goal\"\n", invoke)
	fmt.Printf("  Agree contract:             %s contract propose && %s contract approve --role planner && %s contract approve --role tester\n", invoke, invoke, invoke)
	fmt.Printf("  Run QA after agreement:     %s sprint qa\n", invoke)
	fmt.Println()
	fmt.Println("Codex, Claude Code, and Cursor interact with Harness by running these CLI commands from the installed references/hooks.")

	if opts.StartTUI {
		return runTUI(true, version)
	}
	return nil
}

func setupWizard(opts setupOptions, project detect.ProjectInfo) (setupChoices, error) {
	interactive := !opts.Yes && isTerminal(os.Stdin)
	cli, err := setupCLI(opts, project, interactive)
	if err != nil {
		return setupChoices{}, err
	}
	skills, err := setupSkills(opts, interactive)
	if err != nil {
		return setupChoices{}, err
	}
	scope, err := setupScope(opts, interactive)
	if err != nil {
		return setupChoices{}, err
	}
	return setupChoices{CLI: cli, Skills: skills, Scope: scope}, nil
}

func setupCLI(opts setupOptions, project detect.ProjectInfo, interactive bool) (string, error) {
	cli := normalizeCLI(opts.CLI)
	switch cli {
	case "auto":
		if len(project.CodingCLIs) > 0 {
			if !interactive {
				return "auto", nil
			}
		}
		if interactive {
			picked, err := promptHookTarget(project)
			if err != nil {
				return "", err
			}
			if normalizeCLI(picked) == "auto" {
				if len(project.CodingCLIs) > 0 {
					return "auto", nil
				}
				return "all", nil
			}
			return picked, nil
		}
		fmt.Println("No coding CLI marker detected; installing Codex, Claude Code, and Cursor references.")
		return "all", nil
	case "all", "none", "git", "codex", "claude", "cursor":
		return cli, nil
	default:
		return "", fmt.Errorf("unknown coding CLI %q; use auto|codex|claude|cursor|all|none", cli)
	}
}

func setupSkills(opts setupOptions, interactive bool) (bool, error) {
	switch normalizeSkillsMode(opts.Skills) {
	case "auto":
		if interactive {
			return promptYesNo("Install automated contract-authoring skills?", true)
		}
		return true, nil
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, fmt.Errorf("unknown skills mode %q; use auto|on|off", opts.Skills)
	}
}

func setupScope(opts setupOptions, interactive bool) (string, error) {
	scope := normalizeScope(opts.Scope)
	switch scope {
	case "auto":
		if interactive {
			return promptScope()
		}
		return "project", nil
	case "project", "global":
		return scope, nil
	default:
		return "", fmt.Errorf("unknown install scope %q; use auto|project|global", opts.Scope)
	}
}

func promptYesNo(question string, fallback bool) (bool, error) {
	defaultIndex := 1
	if fallback {
		defaultIndex = 0
	}
	value, err := promptSelect(question, []promptOption{
		{Label: "Yes", Description: "Install agent skills for automatic contract authoring", Value: "yes"},
		{Label: "No", Description: "Keep contract authoring manual", Value: "no"},
	}, defaultIndex)
	if err != nil {
		return false, err
	}
	return value == "yes", nil
}

func promptScope() (string, error) {
	value, err := promptSelect("Installation scope", []promptOption{
		{Label: "Project only", Description: "Write .harness and agent references only in this repo", Value: "project"},
		{Label: "Global command + this project", Description: "Also copy the resolved harness binary to a global bin directory", Value: "global"},
	}, 0)
	if err != nil {
		return "", err
	}
	return value, nil
}

func normalizeScope(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "", "detect":
		return "auto"
	case "local", "repo", "repository":
		return "project"
	}
	return v
}

func boolSkillsMode(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func writeSetupState(choices setupChoices, project detect.ProjectInfo) error {
	if _, err := os.Stat(".harness"); err != nil {
		return nil
	}
	state := map[string]any{
		"schema_version":          "1",
		"updated_at":              time.Now().UTC().Format(time.RFC3339),
		"project":                 valueOr(project.Name, "unknown"),
		"stack":                   valueOr(project.Stack, "unknown"),
		"coding_cli":              choices.CLI,
		"contract_skills_enabled": choices.Skills,
		"install_scope":           choices.Scope,
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(filepath.Join(".harness", "setup.json"), content, 0o644)
}

func installGlobalCommand() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current harness executable: %w", err)
	}
	dirs := globalInstallDirs()
	if len(dirs) == 0 {
		return fmt.Errorf("no global install directory found")
	}
	var lastErr error
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			lastErr = err
			continue
		}
		dest := filepath.Join(dir, executableName("harness"))
		if samePath(exe, dest) {
			fmt.Println("  OK global harness command already points to this executable:", dest)
			return nil
		}
		if err := copyFile(exe, dest, 0o755); err != nil {
			lastErr = err
			continue
		}
		fmt.Println("  OK global harness command installed:", dest)
		if !pathContains(dir) {
			fmt.Println("  Add this directory to PATH to call harness from any terminal:", dir)
		}
		return nil
	}
	return fmt.Errorf("install global harness command: %w", lastErr)
}

func installProjectCommand() error {
	if _, err := os.Stat(".harness"); err != nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current harness executable: %w", err)
	}
	dir := filepath.Join(".harness", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(dir, executableName("harness"))
	if samePath(exe, dest) {
		return nil
	}
	if err := copyFile(exe, dest, 0o755); err != nil {
		return fmt.Errorf("install project harness command: %w", err)
	}
	return nil
}

func globalInstallDirs() []string {
	var dirs []string
	if out, err := exec.Command("npm", "prefix", "-g").Output(); err == nil {
		prefix := strings.TrimSpace(string(out))
		if prefix != "" {
			if runtime.GOOS == "windows" {
				dirs = append(dirs, prefix)
			} else {
				dirs = append(dirs, filepath.Join(prefix, "bin"))
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if runtime.GOOS == "windows" {
			if local := os.Getenv("LOCALAPPDATA"); local != "" {
				dirs = append(dirs, filepath.Join(local, "Harness", "bin"))
			}
		} else {
			dirs = append(dirs, filepath.Join(home, ".local", "bin"))
		}
	}
	return uniqueStrings(dirs)
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(dest, mode)
	}
	return nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
}

func pathContains(dir string) bool {
	pathValue := os.Getenv("PATH")
	for _, entry := range filepath.SplitList(pathValue) {
		if samePath(entry, dir) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(value))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}
