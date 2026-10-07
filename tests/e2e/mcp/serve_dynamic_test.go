package mcp

// End-to-end checks of `ai-rulez mcp --serve-skills` driven over stdio with
// the built binary: the RV-DYN review harness cases that need a real process
// (signals, live reload against the file system, raw JSON-RPC ordering).

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

const serveBaseConfig = "version = \"4.0\"\nname = \"p\"\ngitignore = false\npresets = [\"claude\"]\n"

func serveSkill(name, desc, body, extra string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n%s---\n\n# %s\n\n%s\n", name, desc, extra, name, body)
}

// serveProject writes a project under a fresh directory: files are relative to
// .ai-rulez/ unless they start with "@" (project root).
func serveProject(t *testing.T, config string, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj")
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	write(".ai-rulez/config.toml", config)
	for rel, content := range files {
		if after, ok := strings.CutPrefix(rel, "@"); ok {
			write(after, content)
			continue
		}
		write(".ai-rulez/"+rel, content)
	}
	return root
}

// serveProc is one `ai-rulez mcp --serve-skills` process with a raw JSON-RPC
// peer on its stdio.
type serveProc struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan map[string]any
	nextID int
	mu     sync.Mutex
	stderr strings.Builder
	done   chan struct{}
}

func startServe(t *testing.T, root string, args ...string) *serveProc {
	t.Helper()
	bin := testutil.SetupTestBinary(t)
	home := filepath.Join(filepath.Dir(root), "home")
	require.NoError(t, os.MkdirAll(home, 0o755))
	cmd := exec.Command(bin, append([]string{"mcp", "--serve-skills"}, args...)...) //nolint:gosec // the test binary
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+home, "USERPROFILE="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
		"XDG_DATA_HOME="+filepath.Join(home, ".local", "share"), "XDG_STATE_HOME="+filepath.Join(home, ".local", "state"),
		"AI_RULEZ_HOME="+filepath.Join(home, ".ai-rulez"), "AI_RULEZ_POLICY=", "AI_RULEZ_TELEMETRY=0",
	)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	p := &serveProc{t: t, cmd: cmd, stdin: stdin, lines: make(chan map[string]any, 256), done: make(chan struct{})}
	cmd.Stderr = &lockedWriter{mu: &p.mu, b: &p.stderr}
	require.NoError(t, cmd.Start())
	go func() {
		defer close(p.lines)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1<<20), 1<<24)
		for scanner.Scan() {
			var msg map[string]any
			if json.Unmarshal(scanner.Bytes(), &msg) == nil {
				p.lines <- msg
			}
		}
	}()
	go func() {
		_ = cmd.Wait() //nolint:errcheck // the exit is observed through done
		close(p.done)
	}()
	t.Cleanup(func() {
		_ = stdin.Close() //nolint:errcheck // already closed by some tests
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill() //nolint:errcheck // best effort
		}
	})
	return p
}

type lockedWriter struct {
	mu *sync.Mutex
	b  *strings.Builder
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p) //nolint:wrapcheck // strings.Builder never fails
}

func (p *serveProc) call(method string, params any) map[string]any {
	p.t.Helper()
	p.nextID++
	raw, err := json.Marshal(params)
	require.NoError(p.t, err)
	_, err = fmt.Fprintf(p.stdin, `{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`+"\n", p.nextID, method, raw)
	require.NoError(p.t, err)
	deadline := time.After(20 * time.Second)
	for {
		select {
		case msg, ok := <-p.lines:
			require.True(p.t, ok, "server exited: %s", p.stderrText())
			if id, isNum := msg["id"].(float64); isNum && int(id) == p.nextID {
				return msg
			}
		case <-deadline:
			require.FailNow(p.t, "no response", "%s: %s", method, p.stderrText())
		}
	}
}

func (p *serveProc) notify(method string) {
	_, err := fmt.Fprintf(p.stdin, `{"jsonrpc":"2.0","method":%q}`+"\n", method)
	require.NoError(p.t, err)
}

func (p *serveProc) initialize() {
	p.t.Helper()
	resp := p.call("initialize", map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "e2e", "version": "1"},
	})
	require.Nil(p.t, resp["error"], "initialize: %v", resp)
	p.notify("notifications/initialized")
}

// load calls load_skill and returns its content, or "" when it is an error.
func (p *serveProc) load(name string) string {
	p.t.Helper()
	resp := p.call("tools/call", map[string]any{"name": "load_skill", "arguments": map[string]any{"name": name}})
	result, ok := resp["result"].(map[string]any)
	if !ok || result["isError"] == true {
		return ""
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	var body struct {
		Content string `json:"content"`
	}
	if json.Unmarshal([]byte(text), &body) != nil {
		return ""
	}
	return body.Content
}

func (p *serveProc) stderrText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stderr.String()
}

func (p *serveProc) waitExit(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(15 * time.Second):
		require.FailNow(t, "server did not exit")
	}
}

func eventuallyLoads(t *testing.T, p *serveProc, name, want string) {
	t.Helper()
	require.Eventually(t, func() bool { return strings.Contains(p.load(name), want) }, 10*time.Second, 50*time.Millisecond,
		"load_skill %s never returned %q; stderr: %s", name, want, p.stderrText())
}

func skipShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("drives the built binary; skipped in -short")
	}
}

// RV-DYN-7: the Skills extension methods follow the MCP lifecycle.
func TestServeSkills_ExtensionMethodsWaitForInitialize(t *testing.T) {
	skipShort(t)
	root := serveProject(t, serveBaseConfig, map[string]string{
		"skills/kit/SKILL.md": serveSkill("kit", "A served kit", "kit body", "delivery: served\n"),
	})
	p := startServe(t, root)

	before := p.call("skills/list", map[string]any{})
	p.initialize()
	after := p.call("skills/list", map[string]any{})

	assert.NotNil(t, before["error"], "skills/list answered before initialize")
	assert.Nil(t, after["error"])
}

// RV-DYN-2: a skill source cannot take the name of a static project skill.
func TestServeSkills_SourceSkillDoesNotShadowAStaticProjectSkill(t *testing.T) {
	skipShort(t)
	root := serveProject(t, serveBaseConfig+"\n[[skill_sources]]\nname = \"vend\"\nurl = \"vend\"\n", map[string]string{
		"skills/deploy/SKILL.md": serveSkill("deploy", "Project deploy", "PROJECT", "delivery: static\n"),
		"skills/other/SKILL.md":  serveSkill("other", "Other", "other body", "delivery: served\n"),
		"@vend/deploy/SKILL.md":  serveSkill("deploy", "Vendor deploy", "VENDOR", ""),
	})
	p := startServe(t, root)
	p.initialize()

	assert.NotContains(t, p.load("deploy"), "VENDOR")
	assert.Contains(t, p.load("other"), "other body")
}

// RV-DYN-3, RV-DYN-5, RV-DYN-6: live reload against the file system.
func TestServeSkills_LiveReload(t *testing.T) {
	skipShort(t)
	tests := []struct {
		name  string
		files map[string]string
		edit  func(t *testing.T, root string)
		check func(t *testing.T, p *serveProc)
	}{
		{
			name: "same-size edit with the mtime restored",
			edit: func(t *testing.T, root string) {
				path := filepath.Join(root, ".ai-rulez", "skills", "kit", "SKILL.md")
				info, err := os.Stat(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, []byte(serveSkill("kit", "A served kit", "kit BODY", "delivery: served\n")), 0o644))
				require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
			},
			check: func(t *testing.T, p *serveProc) { eventuallyLoads(t, p, "kit", "kit BODY") },
		},
		{
			name:  "local include outside the config dir",
			files: map[string]string{"config.local.toml": "[[includes]]\nname = \"shared\"\nsource = \"../shared\"\n", "@../shared/.ai-rulez/skills/inc/SKILL.md": serveSkill("inc", "Included", "inc v1", "delivery: served\n")},
			edit: func(t *testing.T, root string) {
				path := filepath.Join(filepath.Dir(root), "shared", ".ai-rulez", "skills", "inc", "SKILL.md")
				require.NoError(t, os.WriteFile(path, []byte(serveSkill("inc", "Included", "inc v2", "delivery: served\n")), 0o644))
			},
			check: func(t *testing.T, p *serveProc) { eventuallyLoads(t, p, "inc", "inc v2") },
		},
		{
			name: "truncated SKILL.md keeps the previous version",
			edit: func(t *testing.T, root string) {
				require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "skills", "kit", "SKILL.md"), nil, 0o644))
			},
			check: func(t *testing.T, p *serveProc) {
				time.Sleep(500 * time.Millisecond)
				assert.Contains(t, p.load("kit"), "kit body")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			files := map[string]string{"skills/kit/SKILL.md": serveSkill("kit", "A served kit", "kit body", "delivery: served\n")}
			for k, v := range tt.files {
				files[k] = v
			}
			root := serveProject(t, serveBaseConfig, files)
			p := startServe(t, root, "--reload-interval", "50ms", "--budget-bytes", "-1")
			p.initialize()
			require.Contains(t, p.load("kit"), "kit body")

			// Act
			tt.edit(t, root)

			// Assert
			tt.check(t, p)
		})
	}
}

// RV-DYN-4: SIGTERM flushes the queued --usage-sink records, like EOF.
func TestServeSkills_ShutdownFlushesTheUsageSink(t *testing.T) {
	skipShort(t)
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM cannot be sent to a process on Windows")
	}
	tests := []struct {
		name string
		stop func(p *serveProc)
	}{
		{name: "end of input", stop: func(p *serveProc) { _ = p.stdin.Close() }},                  //nolint:errcheck // closing is the signal
		{name: "SIGTERM", stop: func(p *serveProc) { _ = p.cmd.Process.Signal(syscall.SIGTERM) }}, //nolint:errcheck // checked by the exit
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := serveProject(t, serveBaseConfig, map[string]string{
				"skills/kit/SKILL.md": serveSkill("kit", "A served kit", "kit body", "delivery: served\n"),
			})
			out := filepath.Join(t.TempDir(), "sink.out")
			p := startServe(t, root, "--no-watch", "--usage-sink", "sleep 0.3; cat >> '"+out+"'")
			p.initialize()
			for range 3 {
				require.Contains(t, p.load("kit"), "kit body")
			}

			// Act
			tt.stop(p)
			p.waitExit(t)

			// Assert
			data, err := os.ReadFile(out)
			require.NoError(t, err)
			assert.Len(t, strings.Split(strings.TrimSpace(string(data)), "\n"), 3, "stderr: %s", p.stderrText())
			assert.Equal(t, 0, p.cmd.ProcessState.ExitCode(), "a signal is a clean shutdown")
		})
	}
}
