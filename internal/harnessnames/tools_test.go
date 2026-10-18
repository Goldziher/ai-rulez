package harnessnames

import (
	"os/exec"
	"testing"
)

// TestHarnessBuiltinsToolsPass runs the offline unit tests of tools/harness-builtins (the
// extractor parsers, the merge into the data files and the quote check) against their saved
// fixtures. It needs python3 and skips without it; the network tests are not part of it.
func TestHarnessBuiltinsToolsPass(t *testing.T) {
	// Arrange
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	cmd := exec.Command(python, "-I", "-m", "unittest", "discover", "-s", "tests", "-t", ".")
	cmd.Dir = "../../tools/harness-builtins"
	// Act
	out, err := cmd.CombinedOutput()
	// Assert
	if err != nil {
		t.Fatalf("python3 -I -m unittest discover failed: %v\n%s", err, out)
	}
}
