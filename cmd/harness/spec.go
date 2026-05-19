package harness

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newSpecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "spec",
		Short: "View or edit .harness/spec.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ".harness/spec.md"
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("spec.md not found — run 'harness init' first")
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				// No editor — just print.
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				fmt.Print(string(b))
				return nil
			}
			c := exec.Command(editor, path)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}
