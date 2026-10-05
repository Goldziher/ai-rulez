package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const verifiersBase = "version = \"4.0\"\nname = \"x\"\npresets = [\"claude\"]\n"

func resetVerifiersFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		verifiersStrict, verifiersJSON, verifiersNames, verifiersProfile, noLocal, configDir = false, false, nil, "", false, ""
	}
	reset()
	t.Cleanup(reset)
}

func TestRunVerifiers_ExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		config string
		strict bool
		json   bool
		names  []string
		want   int
	}{
		{"none configured", verifiersBase, false, false, nil, 0},
		{"all pass", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \".ai-rulez/config.toml\"\n", false, false, nil, 0},
		{"error failure", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\n", false, false, nil, exitVerifiersFindings},
		{"warning passes", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\nseverity = \"warning\"\n", false, true, nil, 0},
		{"warning fails with strict", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"MISSING\"\nseverity = \"warning\"\n", true, false, nil, exitVerifiersFindings},
		{"unknown name cannot run", verifiersBase, false, false, []string{"nope"}, exitVerifiersCannotRun},
		{"invalid config cannot run", verifiersBase + "[[verifiers]]\nname = \"a\"\ntype = \"command\"\n", false, false, nil, exitVerifiersCannotRun},
		{"unloadable config cannot run", brokenRootConfig, false, true, nil, exitVerifiersCannotRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetVerifiersFlags(t)
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), tt.config)
			chdir(t, root)
			verifiersStrict, verifiersJSON, verifiersNames = tt.strict, tt.json, tt.names
			var out bytes.Buffer

			got := runVerifiers(context.Background(), nil, &out)

			if got != tt.want {
				t.Errorf("exit code = %d, want %d\n%s", got, tt.want, out.String())
			}
			if tt.json && got != exitVerifiersCannotRun {
				var decoded map[string]any
				if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
					t.Errorf("--json output is not valid JSON: %v\n%s", err, out.String())
				}
			}
		})
	}
}

func TestListVerifiers(t *testing.T) {
	resetVerifiersFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersBase+"[[verifiers]]\nname = \"readme\"\ndescription = \"needs a README\"\ntype = \"file_exists\"\npath = \"README.md\"\n")
	chdir(t, root)
	var out bytes.Buffer

	got := listVerifiers(context.Background(), nil, &out)

	if got != 0 {
		t.Fatalf("exit code = %d\n%s", got, out.String())
	}
	for _, want := range []string{"readme", "file_exists", "needs a README"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunVerifiers_FailureOutranksCannotRun(t *testing.T) {
	resetVerifiersFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersBase+
		"[[verifiers]]\nname = \"a-missing\"\ntype = \"file_exists\"\npath = \"MISSING\"\n"+
		"[[verifiers]]\nname = \"b-binary\"\ntype = \"forbid\"\nglob = \"bin.dat\"\npattern = \"x\"\n")
	writeFile(t, filepath.Join(root, "bin.dat"), "a\x00b")
	chdir(t, root)
	var out bytes.Buffer

	got := runVerifiers(context.Background(), nil, &out)

	if got != exitVerifiersFindings {
		t.Errorf("exit code = %d, want %d (a failure must not be hidden by an error)\n%s", got, exitVerifiersFindings, out.String())
	}
	if !strings.Contains(out.String(), "1 failed, 1 could not run") {
		t.Errorf("report should show both outcomes:\n%s", out.String())
	}
}

func TestVerifiersListAndValidate_SurfaceConfigErrors(t *testing.T) {
	resetVerifiersFlags(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), verifiersBase+
		"[[verifiers]]\nname = \"a\"\ntype = \"file_exists\"\npath = \"x\"\npattern = \"nope\"\n")
	chdir(t, root)
	var out bytes.Buffer

	if got := listVerifiers(context.Background(), nil, &out); got != exitVerifiersCannotRun {
		t.Errorf("list exit code = %d, want %d for a verifier with an inapplicable field", got, exitVerifiersCannotRun)
	}
}
