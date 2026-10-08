// Package conformance proves the standards claims of docs/standards.md.
//
// Each test validates a document ai-rulez produces (a checked-in golden, or the
// output of the freshly built CLI) against the standard's own schema, vendored
// and pinned under schemas/. Nothing here touches the network; README.md says
// how to refresh a schema.
package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

var (
	repoRoot  string
	binary    string
	buildOnce sync.Once
	buildErr  error
)

func TestMain(m *testing.M) {
	testutil.CeilGit()
	_, file, _, _ := runtime.Caller(0)
	repoRoot = filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	code := m.Run()
	if binary != "" {
		_ = os.RemoveAll(filepath.Dir(binary)) //nolint:errcheck // best effort
	}
	os.Exit(code)
}

// cli builds the ai-rulez binary once and returns its path.
func cli(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "conformance-cli-")
		if err != nil {
			buildErr = err
			return
		}
		binary = filepath.Join(dir, "ai-rulez")
		cmd := exec.Command("go", "build", "-o", binary, "./cmd/ai-rulez")
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build the CLI: %w\n%s", err, out)
		}
	})
	require.NoError(t, buildErr)
	return binary
}

// run executes the CLI in dir with an isolated home and returns stdout.
func run(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(cli(t), args...)
	cmd.Dir = dir
	home := t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "XDG_CACHE_HOME="+home, "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), "ai-rulez %v\n%s", args, stderr.String())
	return stdout.Bytes()
}

// project writes files (relative path to content) into a fresh directory that
// is also a git repository, and returns it.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	testutil.WriteTree(t, dir, files)
	testutil.Git(t, dir, "init", "-q")
	return dir
}

func read(t *testing.T, elem ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(elem...))
	require.NoError(t, err)
	return data
}

// repoFile reads a file relative to the repository root.
func repoFile(t *testing.T, rel string) []byte {
	t.Helper()
	return read(t, repoRoot, filepath.FromSlash(rel))
}

// schemaFile reads a vendored schema relative to schemas/.
func schemaFile(t *testing.T, rel string) []byte {
	t.Helper()
	return read(t, "schemas", filepath.FromSlash(rel))
}

// compile compiles the root schema (by $id) of a set of vendored schemas, each
// keyed by the $id other schemas refer to it with. Remote references fail.
func compile(t *testing.T, root string, byID map[string]string) *jsonschema.Schema {
	t.Helper()
	docs := map[string][]byte{}
	for id, rel := range byID {
		docs[id] = schemaFile(t, rel)
	}
	return compileDocs(t, root, docs)
}

// compileDocs is compile for schema documents already in memory.
func compileDocs(t *testing.T, root string, docs map[string][]byte) *jsonschema.Schema {
	t.Helper()
	compiled, err := jsonschema.NewCompiler().SetAssertFormat(true).CompileBatch(docs)
	require.NoError(t, err)
	return compiled[root]
}

// requireValid fails with every violation unless doc validates against schema.
func requireValid(t *testing.T, schema *jsonschema.Schema, doc []byte) {
	t.Helper()
	result := schema.Validate(doc)
	if !result.IsValid() {
		details, _ := json.MarshalIndent(result.ToList(), "", " ") //nolint:errcheck // diagnostics only
		t.Fatalf("schema violations:\n%s\n%s", details, doc)
	}
}

// requireInvalid guards a schema against vacuity: it must reject doc.
func requireInvalid(t *testing.T, schema *jsonschema.Schema, doc string) {
	t.Helper()
	require.False(t, schema.Validate([]byte(doc)).IsValid(), "the schema accepted %s", doc)
}
