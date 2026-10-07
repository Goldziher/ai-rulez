package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// telemetryProject records InstructionsLoaded hook events through the binary.
// The endpoint is a closed loopback port: these tests never send anything.
type telemetryProject struct {
	t    *testing.T
	env  *isoEnv
	root string
}

const closedEndpoint = "http://127.0.0.1:9"

func newTelemetryProject(t *testing.T) *telemetryProject {
	t.Helper()
	root := minimalProjectIn(t, lingeringTempDir(t), "")
	writeTree(t, root, map[string]string{"CLAUDE.md": "# Project\n"})
	env := newIsoEnv(t)
	env.set("CLAUDE_PROJECT_DIR", root)
	return &telemetryProject{t: t, env: env, root: root}
}

func (p *telemetryProject) record(session string) {
	p.t.Helper()
	ev := fmt.Sprintf(`{"hook_event_name":"InstructionsLoaded","session_id":"s-%s","cwd":%q,"file_path":%q,"memory_type":"Project","load_reason":"session_start"}`,
		session, p.root, filepath.Join(p.root, "CLAUDE.md"))
	res := p.env.runStdin(p.root, ev, "telemetry", "record")
	require.Equal(p.t, 0, res.ExitCode, res.Stderr)
}

func (p *telemetryProject) run(args ...string) string {
	p.t.Helper()
	res := p.env.run(p.root, args...)
	require.Equal(p.t, 0, res.ExitCode, "%v: %s", args, res.Stderr)
	return res.Stdout
}

func (p *telemetryProject) logLines() int {
	p.t.Helper()
	data, err := os.ReadFile(filepath.Join(p.root, ".ai-rulez", "local", "usage.jsonl"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(p.t, err)
	return strings.Count(string(data), "\n")
}

func (p *telemetryProject) userConfig(body string) {
	p.t.Helper()
	writeTree(p.t, filepath.Join(p.env.vars["XDG_CONFIG_HOME"], "ai-rulez"), map[string]string{"config.toml": body})
}

func TestTelemetryConsentE2E(t *testing.T) {
	t.Run("a repository config cannot turn on export", func(t *testing.T) {
		// Arrange
		p := newTelemetryProject(t)
		writeTree(t, p.root, map[string]string{".ai-rulez/config.toml": "version = \"5.0\"\nname = \"e2e\"\npresets = [\"claude\"]\n\n[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"" + closedEndpoint + "\"\n"})

		// Act
		p.record("a")
		flush := p.env.run(p.root, "telemetry", "flush")

		// Assert
		assert.NotEqual(t, 0, flush.ExitCode, "export is not active without user consent")
		assert.Contains(t, p.run("telemetry", "status"), "consent:   none")
	})

	t.Run("enable starts recording and places the cursor at the end of the log", func(t *testing.T) {
		// Arrange
		p := newTelemetryProject(t)
		p.userConfig("[telemetry]\nenabled = true\n")
		p.record("history")

		// Act
		p.run("telemetry", "enable", "--endpoint", closedEndpoint)
		preview := p.run("telemetry", "preview")
		p.record("after")
		after := p.run("telemetry", "preview")

		// Assert
		assert.Contains(t, preview, "Nothing to send", "history recorded before consent is not exported")
		assert.Contains(t, after, "(1 events")
	})

	t.Run("events recorded while consent was withdrawn are not exported after re-enabling", func(t *testing.T) {
		// Arrange: project b records locally (its repository turns recording on)
		// and holds an export cursor from an earlier consent; a is another project
		// of the same user.
		b := newTelemetryProject(t)
		writeTree(t, b.root, map[string]string{".ai-rulez/config.toml": "version = \"5.0\"\nname = \"b\"\npresets = [\"claude\"]\n\n[telemetry]\nenabled = true\n"})
		a := newTelemetryProject(t)
		a.env = b.env
		b.run("telemetry", "enable", "--endpoint", closedEndpoint)
		b.run("telemetry", "disable")
		b.record("while-off")

		// Act: consent is given again, from project a.
		a.env.set("CLAUDE_PROJECT_DIR", a.root)
		a.run("telemetry", "enable", "--endpoint", closedEndpoint)
		a.env.set("CLAUDE_PROJECT_DIR", b.root)
		status := b.run("telemetry", "status", "--format", "json")

		// Assert
		assert.Contains(t, status, `"log": 0`, "consent is not retroactive: b's gap is not pending export")
	})

	// MAN-2: the consent record turns recording on, so withdrawing it stops the
	// recording it started; a config that turns recording on keeps it on. The
	// message says which.
	for _, tc := range []struct {
		name, userConfig, wantOut string
		wantRecorded              int
	}{
		{"disable stops the recording the consent turned on", "", "local recording is off", 0},
		{"disable keeps the recording a user config turns on", "[telemetry]\nenabled = true\n", "local recording stays on (your user config enables it)", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			p := newTelemetryProject(t)
			if tc.userConfig != "" {
				p.userConfig(tc.userConfig)
			}
			p.run("telemetry", "enable", "--endpoint", closedEndpoint)
			p.record("a")
			before := p.logLines()

			// Act
			out := p.run("telemetry", "disable")
			p.record("b")

			// Assert
			require.Contains(t, out, tc.wantOut)
			assert.Equal(t, before+tc.wantRecorded, p.logLines())
		})
	}
}
