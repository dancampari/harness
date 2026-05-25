package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateHarnessExecutableRejectsGoTestBinary(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateHarnessExecutable(exe); err == nil {
		t.Fatalf("expected Go test binary %q to be rejected as an installable Harness CLI", exe)
	}
}

func stubInstallableHarnessExecutable(t *testing.T) {
	t.Helper()
	previousPath := currentExecutablePath
	previousValidate := validateInstallableHarnessExecutable
	fake := filepath.Join(t.TempDir(), executableName("harness"))
	if err := os.WriteFile(fake, []byte("fake harness cli\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	currentExecutablePath = func() (string, error) {
		return fake, nil
	}
	validateInstallableHarnessExecutable = func(string) error {
		return nil
	}
	t.Cleanup(func() {
		currentExecutablePath = previousPath
		validateInstallableHarnessExecutable = previousValidate
	})
}
