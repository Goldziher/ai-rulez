package sbom_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

// fixture is a project on disk that can be rebuilt after edits.
type fixture struct {
	t    *testing.T
	dir  string
	root string
}

func newFixture(t *testing.T, cfg string, files map[string]string) *fixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir := t.TempDir()
	f := &fixture{t: t, dir: dir, root: filepath.Join(dir, ".ai-rulez")}
	require.NoError(t, os.MkdirAll(f.root, 0o755))
	f.write("config.toml", cfg)
	for rel, content := range files {
		f.write(rel, content)
	}
	return f
}

func (f *fixture) write(rel, content string) {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	require.NoError(f.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(f.t, os.WriteFile(full, []byte(content), 0o644))
}

func (f *fixture) writeExec(rel, content string) {
	f.t.Helper()
	f.write(rel, content)
	require.NoError(f.t, os.Chmod(filepath.Join(f.root, filepath.FromSlash(rel)), 0o755))
}

func (f *fixture) config() *config.Config {
	f.t.Helper()
	cfg, err := config.LoadConfig(config.WithOfflineIncludes(context.Background()), f.dir, config.WithoutLocal())
	require.NoError(f.t, err)
	return cfg
}

func (f *fixture) build(opts sbom.Options) *sbom.BOM {
	f.t.Helper()
	bom, err := sbom.Build(f.config(), "9.9.9", opts)
	require.NoError(f.t, err)
	return bom
}

func (f *fixture) cdx(opts sbom.Options) (*sbom.BOM, string) {
	f.t.Helper()
	bom := f.build(opts)
	var buf bytes.Buffer
	require.NoError(f.t, sbom.Write(&buf, bom))
	return bom, buf.String()
}

func (f *fixture) spdx(opts sbom.Options) (*sbom.SPDX, string) {
	f.t.Helper()
	bom := f.build(opts)
	doc := bom.ToSPDX()
	var buf bytes.Buffer
	require.NoError(f.t, sbom.WriteSPDX(&buf, doc))
	return doc, buf.String()
}

// lock pins the project, then lets mutate edit the lock (approvals) before saving.
func (f *fixture) lock(mutate func(*lockfile.File)) *lockfile.File {
	f.t.Helper()
	cfg := f.config()
	snap, err := govview.Snapshot(cfg, "", true, "9.9.9")
	require.NoError(f.t, err)
	lock := &lockfile.File{}
	contentlock.Build(lock, snap)
	if mutate != nil {
		mutate(lock)
	}
	require.NoError(f.t, lockfile.Save(cfg.ConfigDir, lock))
	return lock
}

func (f *fixture) digestOf(kind, id string) string {
	f.t.Helper()
	snap, err := govview.Snapshot(f.config(), "", true, "9.9.9")
	require.NoError(f.t, err)
	for _, it := range snap.Items {
		if it.Kind == kind && it.ID == id {
			return it.Digest
		}
	}
	f.t.Fatalf("no %s %s", kind, id)
	return ""
}

func compileSchema(t *testing.T, files map[string]string, root string) *jsonschema.Schema {
	t.Helper()
	sources := map[string][]byte{}
	for id, name := range files {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		require.NoError(t, err)
		sources[id] = data
	}
	compiled, err := jsonschema.NewCompiler().CompileBatch(sources)
	require.NoError(t, err)
	return compiled[root]
}

func spdxSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	const id = "http://spdx.org/rdf/terms/2.3"
	return compileSchema(t, map[string]string{id: "spdx-2.3.schema.json"}, id)
}

func requireValid(t *testing.T, schema *jsonschema.Schema, doc string) {
	t.Helper()
	result := schema.Validate([]byte(doc))
	if !result.IsValid() {
		details, _ := json.MarshalIndent(result.ToList(), "", " ") //nolint:errcheck // diagnostics only
		t.Fatalf("schema violations:\n%s", details)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func propValue(props []sbom.Property, name string) string {
	for _, p := range props {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

func findComponent(bom *sbom.BOM, ref string) *sbom.Component {
	for i := range bom.Components {
		if bom.Components[i].BOMRef == ref {
			return &bom.Components[i]
		}
	}
	return nil
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err)
	return ts
}
