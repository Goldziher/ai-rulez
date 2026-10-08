package commands

import (
	"os"
	"testing"
)

func withPipeStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
}

func TestShouldOverwriteConfigIgnoresCIEnvironment(t *testing.T) {
	for _, env := range []string{"CI", "NO_INTERACTIVE"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "true")
			old := autoYes
			autoYes = false
			t.Cleanup(func() { autoYes = old })
			withPipeStdin(t, "y\n")
			if shouldOverwriteConfig(".ai-rulez/") {
				t.Fatalf("%s must not authorize overwriting an existing config directory", env)
			}
		})
	}
}

func TestShouldOverwriteConfigHonorsYesFlag(t *testing.T) {
	old := autoYes
	autoYes = true
	t.Cleanup(func() { autoYes = old })
	if !shouldOverwriteConfig(".ai-rulez/") {
		t.Fatal("--yes must authorize overwrite")
	}
}
