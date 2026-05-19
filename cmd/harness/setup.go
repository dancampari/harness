package harness

import (
	"fmt"
	"os"

	"github.com/dancampari/harness/internal/detect"
	"github.com/spf13/cobra"
)

type setupOptions struct {
	CLI      string
	Force    bool
	Yes      bool
	StartTUI bool
}

func newSetupCmd() *cobra.Command {
	var opts setupOptions
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "One-command project bootstrap",
		Long: `Initializes Harness, installs Codex/Claude/Cursor references, runs doctor,
and prints the command to open the live terminal dashboard.

If no coding CLI marker is detected and the command is running in a terminal,
Harness asks one question. In non-interactive mode, use --yes or --cli.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(opts)
		},
	}
	cmd.Flags().StringVar(&opts.CLI, "cli", "auto", "coding CLI to configure: auto|codex|claude|cursor|all|none")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite existing .harness/")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "run setup with no prompts; installs all agent references if none are detected")
	cmd.Flags().BoolVar(&opts.StartTUI, "start", false, "launch the live TUI after setup")
	return cmd
}

func runSetup(opts setupOptions) error {
	if opts.CLI == "" {
		opts.CLI = "auto"
	}

	fmt.Println("Harness setup")
	fmt.Println("  Goal: one command, minimal interaction, project ready for your coding CLI.")
	fmt.Println()

	if _, err := os.Stat(".harness"); err == nil && !opts.Force {
		fmt.Println("OK .harness/ already exists")
	} else {
		if err := runInit(initOptions{
			Force:        opts.Force,
			CLI:          "auto",
			InstallHooks: false,
			Quiet:        true,
		}); err != nil {
			return err
		}
	}

	project := detect.DetectProject(".")
	cli, interactive, err := setupHookMode(opts, project)
	if err != nil {
		return err
	}
	if err := runInstallHooks(installHookOptions{
		CLI:         cli,
		Interactive: interactive,
		InstallGit:  true,
	}); err != nil {
		return err
	}

	fmt.Println()
	if err := runDoctor("."); err != nil {
		return err
	}

	invoke := harnessInvocation()
	fmt.Println()
	fmt.Println("Ready.")
	fmt.Printf("  Open the Harness terminal: %s run --resume\n", invoke)
	fmt.Printf("  Start a sprint:             %s sprint new \"first goal\"\n", invoke)
	fmt.Printf("  Run QA after changes:       %s sprint qa\n", invoke)
	fmt.Println()
	fmt.Println("Codex, Claude Code, and Cursor interact with Harness by running these CLI commands from the installed references/hooks.")

	if opts.StartTUI {
		return runTUI(true)
	}
	return nil
}

func setupHookMode(opts setupOptions, project detect.ProjectInfo) (string, bool, error) {
	cli := normalizeCLI(opts.CLI)
	switch cli {
	case "auto":
		if len(project.CodingCLIs) > 0 {
			return "auto", false, nil
		}
		if !opts.Yes && isTerminal(os.Stdin) {
			picked, err := promptHookTarget(project)
			if err != nil {
				return "", false, err
			}
			if normalizeCLI(picked) == "auto" {
				return "all", false, nil
			}
			return picked, false, nil
		}
		fmt.Println("No coding CLI marker detected; installing Codex, Claude Code, and Cursor references.")
		return "all", false, nil
	case "all", "none", "git", "codex", "claude", "cursor":
		return cli, false, nil
	default:
		return "", false, fmt.Errorf("unknown coding CLI %q; use auto|codex|claude|cursor|all|none", cli)
	}
}
