package commands

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listHelperEnv runs the command line in AI_RULEZ_LIST_HELPER_ARGS in the
// helper process; the list commands exit the process themselves.
const listHelperEnv = "AI_RULEZ_LIST_HELPER_ARGS"

func TestListHelperProcess(t *testing.T) {
	args := os.Getenv(listHelperEnv)
	if args == "" {
		t.Skip("helper process for TestListCommands_ExitOneOnABrokenOrLegacyConfig")
	}
	RootCmd.SetArgs(strings.Fields(args))
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// A list command reads the project, so a configuration that does not load fails
// it with exit 1 like every other command, instead of listing the content tree
// as if nothing were wrong.
func TestListCommands_ExitOneOnABrokenOrLegacyConfig(t *testing.T) {
	projects := []struct {
		name  string
		files map[string]string
		want  int
	}{
		{"valid", map[string]string{"config.toml": "version = \"4.0\"\nname = \"ok\"\npresets = [\"claude\"]\n"}, 0},
		{"toml syntax error", map[string]string{"config.toml": "version = \"4.0\"\nname = \"x\"\npresets = [\n"}, 1},
		{"legacy V3 config", map[string]string{"config.yaml": "metadata:\n  name: old\npresets:\n  - claude\n"}, 1},
	}
	commands := []string{"list rules", "list context", "list skills", "list agents", "list commands", "list checks", "domain list"}
	for _, p := range projects {
		for _, c := range commands {
			t.Run(p.name+"/"+c, func(t *testing.T) {
				// Arrange
				root := t.TempDir()
				for rel, content := range p.files {
					writeFile(t, filepath.Join(root, ".ai-rulez", rel), content)
				}
				home := t.TempDir()
				cmd := exec.Command(os.Args[0], "-test.run=^TestListHelperProcess$")
				cmd.Dir = root
				cmd.Env = append(os.Environ(), listHelperEnv+"="+c, "HOME="+home, "XDG_CONFIG_HOME=", "AI_RULEZ_HOME="+home)

				// Act
				out, err := cmd.CombinedOutput()

				// Assert
				code := 0
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					code = exitErr.ExitCode()
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, p.want, code, "output:\n%s", out)
			})
		}
	}
}
