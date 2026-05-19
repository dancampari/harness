package harness

import (
	"bufio"
	"os"
	"strings"
)

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func joinList(values []string) string {
	return strings.Join(values, ", ")
}

func readPromptLine() (string, error) {
	return bufio.NewReader(os.Stdin).ReadString('\n')
}
