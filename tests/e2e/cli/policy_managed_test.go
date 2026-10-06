package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The managed policy lives at a fixed system path (/etc/ai-rulez/policy.toml,
// /Library/Application Support/ai-rulez/policy.toml, %ProgramData%\ai-rulez\policy.toml),
// which a test cannot write without root. These tests build the binary with the
// managed root moved by a link-time variable (see internal/policy/host.go) and
// drive the real CLI against a managed file in the platform layout on this OS.
// The paths themselves, per OS, are covered by internal/policy/managed_test.go.

const policyPkg = "github.com/Goldziher/ai-rulez/v5/internal/policy"

// managedEnv is a project, a home and a binary whose managed root is a temp dir.
type managedEnv struct {
	t       *testing.T
	bin     string
	root    string // the managed root: the policy is <root>/ai-rulez/policy.toml
	project string
	home    string
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above %s", file)
		dir = parent
	}
}

// buildBinary builds the CLI; ldflags is passed to the linker when not empty.
func buildBinary(t *testing.T, ldflags string) string {
	t.Helper()
	name := "ai-rulez-managed-e2e"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(t.TempDir(), name)
	args := []string{"build", "-o", out}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/ai-rulez")
	cmd := exec.Command("go", args...) //nolint:gosec // fixed arguments; ldflags is a temp dir path
	cmd.Dir = projectRoot(t)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build: %s", output)
	return out
}

func newManagedEnv(t *testing.T) *managedEnv {
	t.Helper()
	root := t.TempDir()
	m := &managedEnv{t: t, root: root, project: t.TempDir(), home: t.TempDir()}
	m.bin = buildBinary(t, fmt.Sprintf("-X %s.managedRoot=%s", policyPkg, root))
	require.NoError(t, os.MkdirAll(filepath.Join(m.project, ".ai-rulez", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(m.project, ".ai-rulez", "rules", "local.md"), []byte("# Local\n\nlocal rule\n"), 0o644))
	m.config("")
	return m
}

func (m *managedEnv) managedFile() string { return filepath.Join(m.root, "ai-rulez", "policy.toml") }

func (m *managedEnv) writeManaged(body string) {
	m.t.Helper()
	require.NoError(m.t, os.MkdirAll(filepath.Dir(m.managedFile()), 0o755))
	require.NoError(m.t, os.WriteFile(m.managedFile(), []byte(body), 0o644))
}

func (m *managedEnv) config(extra string) {
	m.t.Helper()
	require.NoError(m.t, os.WriteFile(filepath.Join(m.project, ".ai-rulez", "config.toml"), []byte(policyBaseConfig+extra), 0o644))
}

type managedRun struct {
	Stdout, Stderr string
	Exit           int
}

// run executes the binary in the project with a private home and no inherited policy variables.
func (m *managedEnv) run(env map[string]string, args ...string) managedRun {
	m.t.Helper()
	return runBinary(m.t, m.bin, m.project, m.home, env, args...)
}

func runBinary(t *testing.T, bin, dir, home string, env map[string]string, args ...string) managedRun {
	t.Helper()
	cmd := exec.Command(bin, args...) //nolint:gosec // the binary this test built
	cmd.Dir = dir
	overrides := map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"), "XDG_CACHE_HOME": filepath.Join(home, ".cache"),
	}
	for k, v := range env {
		overrides[k] = v
	}
	var environ []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if _, replaced := overrides[key]; replaced || strings.HasPrefix(key, "AI_RULEZ_POLICY") {
			continue
		}
		environ = append(environ, e)
	}
	for k, v := range overrides {
		if v != "" {
			environ = append(environ, k+"="+v)
		}
	}
	cmd.Env = environ
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exit := 0
	if ee, ok := err.(*exec.ExitError); ok { //nolint:errorlint // exec.ExitError is returned directly
		exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return managedRun{Stdout: stdout.String(), Stderr: stderr.String(), Exit: exit}
}

const managedPolicy = `policy_version = 1
name = "e2e managed baseline"

[lint.severity_floor]
AR008 = "warning"

[lock]
enforce = true
`

func TestManagedPolicyOnThisOS(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	m := newManagedEnv(t)

	t.Run("without a managed file there is no policy", func(t *testing.T) {
		res := m.run(nil, "validate", "--show-policy")
		assert.Equal(t, 0, res.Exit, res.Stderr)
		assert.Contains(t, res.Stdout, "policy: none")
	})

	t.Run("the managed policy is found, enforced and reported", func(t *testing.T) {
		m.writeManaged(managedPolicy)
		m.config("[lock]\nenforce = false\n")
		res := m.run(nil, "validate", "--show-policy", "--format", "json")
		assert.Equal(t, 1, res.Exit, "a loosening repository exits 1: %s", res.Stderr)
		var rep struct {
			Layers []struct {
				Origin, Source, Name string
			}
			Provenance map[string]string
			Violations []struct{ Code, Key string }
		}
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &rep), res.Stdout)
		require.Len(t, rep.Layers, 1)
		assert.Equal(t, "managed", rep.Layers[0].Origin)
		assert.Equal(t, "e2e managed baseline", rep.Layers[0].Name)
		assert.Equal(t, filepath.Clean(m.managedFile()), filepath.Clean(rep.Layers[0].Source))
		assert.Equal(t, "managed", rep.Provenance["lock.enforce"])
		require.Len(t, rep.Violations, 1)
		assert.Equal(t, "AR740", rep.Violations[0].Code)

		gen := m.run(nil, "generate")
		assert.Equal(t, 1, gen.Exit, gen.Stdout)
		assert.Contains(t, gen.Stderr, "AR740")
		assert.Contains(t, gen.Stderr, "loosens the organization policy")

		m.config("")
		ok := m.run(nil, "generate")
		assert.Equal(t, 0, ok.Exit, ok.Stdout+ok.Stderr)
	})

	t.Run("a layer from the flag is stronger than the managed one", func(t *testing.T) {
		m.writeManaged(managedPolicy)
		flagPolicy := filepath.Join(t.TempDir(), "flag.toml")
		require.NoError(t, os.WriteFile(flagPolicy, []byte("policy_version = 1\nname = \"team\"\n[lint.severity_floor]\nAR008 = \"error\"\n"), 0o644))
		res := m.run(nil, "validate", "--show-policy", "--format", "json", "--policy", flagPolicy)
		require.Equal(t, 0, res.Exit, res.Stderr)
		var rep struct {
			Layers     []struct{ Origin string }
			Provenance map[string]string
		}
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &rep), res.Stdout)
		require.Len(t, rep.Layers, 2)
		assert.Equal(t, "flag", rep.Layers[0].Origin)
		assert.Equal(t, "managed", rep.Layers[1].Origin)
		assert.Equal(t, "flag", rep.Provenance["lint.severity_floor.AR008"], "the stricter value comes from the flag layer")
		assert.Equal(t, "managed", rep.Provenance["lock.enforce"])
	})

	t.Run("an unusable managed policy fails closed", func(t *testing.T) {
		m.writeManaged("policy_version = 1\n[lint]\nrequired_code = [\"AR001\"]\n")
		for _, args := range [][]string{{"validate"}, {"generate"}, {"validate", "--show-policy"}} {
			res := m.run(nil, args...)
			assert.Equal(t, 1, res.Exit, "%v: %s", args, res.Stdout)
			assert.Contains(t, res.Stderr, "AR743", "%v", args)
			assert.Contains(t, res.Stderr, "required_code", "%v", args)
		}
	})

	t.Run("a repository cannot shadow the managed policy by its own files", func(t *testing.T) {
		m.writeManaged(managedPolicy)
		require.NoError(t, os.MkdirAll(filepath.Join(m.project, "ai-rulez"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(m.project, "ai-rulez", "policy.toml"), []byte("policy_version = 1\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(m.project, "ai-rulez-policy.toml"), []byte("policy_version = 1\n"), 0o644))
		res := m.run(nil, "validate", "--show-policy", "--format", "json")
		require.Equal(t, 0, res.Exit, res.Stderr)
		var rep struct {
			Layers []struct{ Origin, Source string }
		}
		require.NoError(t, json.Unmarshal([]byte(res.Stdout), &rep), res.Stdout)
		require.Len(t, rep.Layers, 1, "policy files inside the repository are never read")
		assert.Equal(t, filepath.Clean(m.managedFile()), filepath.Clean(rep.Layers[0].Source))
	})
}

// A release binary reads the real managed path of the OS, not a path relative to
// the repository. Without a file there, it finds no policy.
func TestReleaseBinaryReadsNoManagedPolicyFromTheRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	realPath := map[string]string{
		"darwin": "/Library/Application Support/ai-rulez/policy.toml",
		"linux":  "/etc/ai-rulez/policy.toml",
	}[runtime.GOOS]
	if runtime.GOOS == "windows" {
		realPath = filepath.Join(os.Getenv("ProgramData"), "ai-rulez", "policy.toml")
	}
	if realPath != "" {
		if _, err := os.Stat(realPath); err == nil {
			t.Skipf("this machine has a managed policy at %s", realPath)
		}
	}
	bin := buildBinary(t, "")
	project, home := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".ai-rulez", "config.toml"), []byte(policyBaseConfig), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(project, "policy.toml"), []byte("policy_version = 1\n[lock]\nenforce = true\n"), 0o644))
	// Act
	res := runBinary(t, bin, project, home, nil, "validate", "--show-policy")
	// Assert
	assert.Equal(t, 0, res.Exit, res.Stderr)
	assert.Contains(t, res.Stdout, "policy: none")
}

// On Windows the managed location follows %ProgramData%, which a test can set
// without administrator rights, so the real binary is exercised unmodified.
func TestWindowsManagedPolicyFollowsProgramData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only; the path table for every OS is in internal/policy")
	}
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	bin := buildBinary(t, "")
	programData, project, home := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(programData, "ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(programData, "ai-rulez", "policy.toml"), []byte(managedPolicy), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, ".ai-rulez", "config.toml"), []byte(policyBaseConfig), 0o644))
	// Act
	res := runBinary(t, bin, project, home, map[string]string{"ProgramData": programData}, "validate", "--show-policy")
	// Assert
	assert.Equal(t, 0, res.Exit, res.Stderr)
	assert.Contains(t, res.Stdout, "managed")
	assert.Contains(t, strings.ToLower(res.Stdout), strings.ToLower(filepath.Join(programData, "ai-rulez", "policy.toml")))
}
