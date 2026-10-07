package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

// skillsServer is `ai-rulez mcp --serve-skills` over stdio, started with the
// isolated environment.
type skillsServer struct {
	session *sdkmcp.ClientSession
	stderr  *bytes.Buffer
	cancel  context.CancelFunc
}

func startSkillsServer(t *testing.T, env *isoEnv, root string, args ...string) (*skillsServer, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cmd := exec.CommandContext(ctx, testutil.SetupTestBinary(t), append([]string{"mcp", "--serve-skills"}, args...)...) //nolint:gosec // the test binary
	cmd.Dir = root
	cmd.Env = env.environ()
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "e2e", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: cmd, TerminateDuration: 2 * time.Second}, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	s := &skillsServer{session: session, stderr: stderr, cancel: cancel}
	t.Cleanup(func() { _ = s.session.Close(); s.cancel() }) //nolint:errcheck // best-effort shutdown
	return s, nil
}

// call returns the tool's text and whether it is an error result.
func (s *skillsServer) call(t *testing.T, tool string, args map[string]any) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := s.session.CallTool(ctx, &sdkmcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return err.Error(), true
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

const servedConfig = "\n[skills]\ndelivery = \"served\"\n"

func TestServeSkillsGatingE2E(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		arrange   func(t *testing.T, env *isoEnv, root string)
		args      []string
		blocked   string
		load      map[string]any
		wantError bool
		wantText  string
	}{
		{name: "an authored skill is served", load: map[string]any{"name": "deploy"}, wantText: "Run make deploy."},
		{
			name: "a skill changed after locking is refused (AR995)",
			arrange: func(t *testing.T, env *isoEnv, root string) {
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
				writeTree(t, root, map[string]string{".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Deploy the service to staging.\n---\n# Deploy\n\nRun make deploy-v2.\n"})
			},
			load: map[string]any{"name": "deploy"}, wantError: true, wantText: "AR995",
		},
		{
			name:   "an unapproved served skill is refused under require_approval = [\"kind:served\"] (AR710)",
			config: "\n[governance]\nrequire_approval = [\"kind:served\"]\nenforce = true\n",
			arrange: func(t *testing.T, env *isoEnv, root string) {
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
			},
			load: map[string]any{"name": "deploy"}, wantError: true, wantText: "AR710",
		},
		{
			name:   "an unapproved skill is refused under require_approval = [\"all\"] (AR710)",
			config: "\n[governance]\nrequire_approval = [\"all\"]\nenforce = true\n",
			arrange: func(t *testing.T, env *isoEnv, root string) {
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
			},
			blocked: "MAN-4",
			load:    map[string]any{"name": "deploy"}, wantError: true, wantText: "AR710",
		},
		{
			name:   "an approved served skill is served under enforced approvals",
			config: "\n[governance]\nrequire_approval = [\"kind:served\"]\nenforce = true\n",
			arrange: func(t *testing.T, env *isoEnv, root string) {
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
				require.Equal(t, 0, env.run(root, "approve", "served:deploy", "--reviewer", "alice@example.org", "--yes").ExitCode)
			},
			load: map[string]any{"name": "deploy"}, wantText: "Run make deploy.",
		},
		{
			name: "a denied served digest is refused (AR717)",
			arrange: func(t *testing.T, env *isoEnv, root string) {
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
				require.Equal(t, 0, env.run(root, "approve", "served:deploy", "--reviewer", "alice@example.org", "--yes").ExitCode)
				require.Equal(t, 0, env.run(root, "approve", "served:deploy", "--revoke", "--deny", "--reason", "bad", "--yes").ExitCode)
			},
			load: map[string]any{"name": "deploy"}, wantError: true, wantText: "AR717",
		},
		{
			name: "a skill denied as skill:<name> is not served either (AR717)",
			arrange: func(t *testing.T, env *isoEnv, root string) {
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
				require.Equal(t, 0, env.run(root, "approve", "skill:deploy", "--reviewer", "alice@example.org", "--yes").ExitCode)
				require.Equal(t, 0, env.run(root, "approve", "skill:deploy", "--revoke", "--deny", "--reason", "bad", "--yes").ExitCode)
			},
			blocked: "MAN-5",
			load:    map[string]any{"name": "deploy"}, wantError: true, wantText: "AR717",
		},
		{
			name:   "a skill needs a signed lock under [signing] require = [\"served\"] (AR720)",
			config: "\n[signing]\nrequire = [\"served\"]\ntlog = \"off\"\nkey_file = \"release.pub\"\n",
			arrange: func(t *testing.T, env *isoEnv, root string) {
				_, pub := writeKeyPair(t, t.TempDir(), "release")
				data, err := os.ReadFile(pub) //nolint:gosec // test key
				require.NoError(t, err)
				writeTree(t, root, map[string]string{"release.pub": string(data)})
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
			},
			load: map[string]any{"name": "deploy"}, wantError: true, wantText: "AR720",
		},
		{
			name:   "a signed lock serves the skill",
			config: "\n[signing]\nrequire = [\"served\"]\ntlog = \"off\"\nkey_file = \"release.pub\"\n",
			arrange: func(t *testing.T, env *isoEnv, root string) {
				key, pub := writeKeyPair(t, t.TempDir(), "release")
				data, err := os.ReadFile(pub) //nolint:gosec // test key
				require.NoError(t, err)
				writeTree(t, root, map[string]string{"release.pub": string(data)})
				require.Equal(t, 0, env.run(root, "lock").ExitCode)
				require.Equal(t, 0, env.run(root, "sign", "--lock", "--key", key).ExitCode)
			},
			load: map[string]any{"name": "deploy"}, wantText: "Run make deploy.",
		},
		{
			name: "a path outside the skill is refused",
			load: map[string]any{"name": "deploy", "path": "../../config.toml"}, wantError: true,
		},
		{
			name: "the session byte budget is enforced",
			args: []string{"--budget-bytes", "10"},
			load: map[string]any{"name": "deploy"}, wantError: true, wantText: "budget",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.blocked != "" {
				blockedOn(t, tt.blocked)
			}
			// Arrange
			env := newIsoEnv(t)
			root := minimalProject(t, servedConfig+tt.config)
			env.commitAll(root, "init")
			if tt.arrange != nil {
				tt.arrange(t, env, root)
			}
			srv, err := startSkillsServer(t, env, root, tt.args...)
			require.NoError(t, err)

			// Act
			text, isErr := srv.call(t, "load_skill", tt.load)

			// Assert
			assert.Equal(t, tt.wantError, isErr, "load_skill answered: %s\nstderr: %s", text, srv.stderr.String())
			if tt.wantText != "" {
				assert.Contains(t, text, tt.wantText)
			}
			if tt.wantError {
				assert.NotContains(t, text, "Run make deploy.", "a refused skill's content never reaches the agent")
				assert.NotContains(t, text, "deploy-v2")
			}
		})
	}
}

// TestServeSkillsNeverWritesE2E: the read-only server registers no authoring
// tool and leaves the project untouched.
func TestServeSkillsNeverWritesE2E(t *testing.T) {
	// Arrange
	env := newIsoEnv(t)
	root := minimalProject(t, servedConfig)
	env.commitAll(root, "init")
	srv, err := startSkillsServer(t, env, root)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Act
	tools, err := srv.session.ListTools(ctx, nil)
	require.NoError(t, err)
	_, _ = srv.call(t, "find_skill", map[string]any{"task": "deploy to staging"})
	_, _ = srv.call(t, "load_skill", map[string]any{"name": "deploy"})

	// Assert
	for _, tool := range tools.Tools {
		for _, verb := range []string{"create", "update", "delete", "generate", "install"} {
			assert.False(t, strings.HasPrefix(tool.Name, verb), "authoring tool %s is registered in --serve-skills", tool.Name)
		}
	}
	assert.Empty(t, env.git(root, "status", "--porcelain"), "serving changed the project")
	_, statErr := os.Stat(filepath.Join(root, "CLAUDE.md"))
	assert.True(t, os.IsNotExist(statErr), "serving generated outputs")
}
