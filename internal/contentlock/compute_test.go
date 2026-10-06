package contentlock

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

type fixture struct {
	t    *testing.T
	root string
	cfg  *config.Config
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	return &fixture{t: t, root: root, cfg: &config.Config{
		BaseDir: root, ConfigDir: filepath.Join(root, ".ai-rulez"),
		Content: &config.ContentTree{Domains: map[string]*config.Domain{}},
	}}
}

func (f *fixture) write(rel string, data []byte, perm os.FileMode) string {
	f.t.Helper()
	abs := filepath.Join(f.cfg.ConfigDir, filepath.FromSlash(rel))
	require.NoError(f.t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(f.t, os.WriteFile(abs, data, perm))
	return abs
}

func (f *fixture) rule(name, body string) {
	p := f.write("rules/"+name+".md", []byte(body), 0o644)
	f.cfg.Content.Rules = append(f.cfg.Content.Rules, config.ContentFile{Name: name, Path: p, Content: "stripped"})
}

func (f *fixture) skill(domain, id string, files map[string]string) {
	var cf config.ContentFile
	for rel, body := range files {
		perm := os.FileMode(0o644)
		if filepath.Ext(rel) == ".sh" {
			perm = 0o755
		}
		abs := f.write(filepath.ToSlash(filepath.Join("domains", domain, "skills", id, rel)), []byte(body), perm)
		if rel == "SKILL.md" {
			cf.Path, cf.Name = abs, id
		}
	}
	for rel, body := range files {
		if rel == "SKILL.md" {
			continue
		}
		perm := os.FileMode(0o644)
		if filepath.Ext(rel) == ".sh" {
			perm = 0o755
		}
		cf.Resources = append(cf.Resources, config.SkillResource{RelPath: rel, Content: []byte(body), Mode: perm})
	}
	if raw, ok := files["SKILL.md"]; ok && strings.Contains(raw, "owner: team-a") {
		cf.Metadata = &config.Metadata{Extra: map[string]string{"owner": "team-a", "version": "1.2.0"}}
	}
	d := f.cfg.Content.Domains[domain]
	if d == nil {
		d = &config.Domain{Name: domain}
		f.cfg.Content.Domains[domain] = d
	}
	d.Skills = append(d.Skills, cf)
}

func (f *fixture) items() map[string]string {
	f.t.Helper()
	snap, err := Compute(f.cfg, Options{})
	require.NoError(f.t, err)
	out := map[string]string{}
	for _, i := range snap.Items {
		out[i.Kind+":"+i.Domain+"/"+i.ID] = i.Digest
	}
	return out
}

func TestComputeHashesRawFilesNotStrippedContent(t *testing.T) {
	f := newFixture(t)
	f.rule("style", "---\npriority: high\n---\n# Style\n")
	before := f.items()["rule:/style"]
	require.NotEmpty(t, before)

	f.cfg.Content.Rules[0].Content = "something else entirely" // the loader's view is irrelevant
	assert.Equal(t, before, f.items()["rule:/style"])

	f.write("rules/style.md", []byte("---\npriority: low\n---\n# Style\n"), 0o644) // frontmatter is part of the pin
	assert.NotEqual(t, before, f.items()["rule:/style"])
}

func TestComputeNormalizesLineEndings(t *testing.T) {
	f := newFixture(t)
	f.rule("style", "# Style\nbody\n")
	lf := f.items()["rule:/style"]
	f.write("rules/style.md", []byte("# Style\r\nbody\r\n"), 0o644)
	assert.Equal(t, lf, f.items()["rule:/style"], "CRLF checkout pins the same digest")
}

func TestComputeSkillResources(t *testing.T) {
	f := newFixture(t)
	f.skill("backend", "deploy", map[string]string{
		"SKILL.md":            "---\nname: deploy\nowner: team-a\nversion: 1.2.0\n---\nDeploy\n",
		"references/api.md":   "api\n",
		"scripts/run.sh":      "#!/bin/sh\n",
		"assets/data.bin":     "\x00\x01",
		"references/empty.md": "",
	})
	first := f.items()["skill:backend/deploy"]
	require.NotEmpty(t, first)

	f.write("domains/backend/skills/deploy/references/api.md", []byte("api changed\n"), 0o644)
	changed := f.items()["skill:backend/deploy"]
	assert.NotEqual(t, first, changed, "a resource change changes the skill digest")

	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(filepath.Join(f.cfg.ConfigDir, "domains/backend/skills/deploy/scripts/run.sh"), 0o644))
		assert.NotEqual(t, changed, f.items()["skill:backend/deploy"], "dropping the executable bit changes the digest")
	}

	snap, err := Compute(f.cfg, Options{})
	require.NoError(t, err)
	require.Len(t, snap.Items, 1)
	assert.Equal(t, "team-a", snap.Items[0].Owner)
	assert.Equal(t, "domains/backend/skills/deploy", snap.Items[0].Path)
}

func TestComputeRenameAndDeterminism(t *testing.T) {
	f := newFixture(t)
	f.rule("b", "b\n")
	f.rule("a", "a\n")
	f.skill("d", "s", map[string]string{"SKILL.md": "s\n"})
	first, err := Compute(f.cfg, Options{IncludeOutputs: true, Outputs: []Output{{Path: "z.md", Data: []byte("z")}, {Path: "a.md", Data: []byte("a")}}})
	require.NoError(t, err)

	// shuffle the in-memory order of everything
	f.cfg.Content.Rules[0], f.cfg.Content.Rules[1] = f.cfg.Content.Rules[1], f.cfg.Content.Rules[0]
	second, err := Compute(f.cfg, Options{IncludeOutputs: true, Outputs: []Output{{Path: "a.md", Data: []byte("a")}, {Path: "z.md", Data: []byte("z")}}})
	require.NoError(t, err)
	assert.Equal(t, first.Items, second.Items)
	assert.Equal(t, first.Outputs, second.Outputs)

	var a, b lockfile.File
	Build(&a, first)
	Build(&b, second)
	assert.Equal(t, a.Tree, b.Tree)

	// Renaming a rule is a removal plus an addition.
	require.NoError(t, os.Rename(filepath.Join(f.cfg.ConfigDir, "rules/a.md"), filepath.Join(f.cfg.ConfigDir, "rules/c.md")))
	for i := range f.cfg.Content.Rules {
		if f.cfg.Content.Rules[i].Name == "a" {
			f.cfg.Content.Rules[i].Name = "c"
			f.cfg.Content.Rules[i].Path = filepath.Join(f.cfg.ConfigDir, "rules/c.md")
		}
	}
	renamed, err := Compute(f.cfg, Options{IncludeOutputs: true, Outputs: second.Options.Outputs})
	require.NoError(t, err)
	diff := Compare(&a, renamed)
	var kinds []string
	for _, c := range diff.Sources() {
		kinds = append(kinds, c.Change+":"+c.ID)
	}
	assert.Equal(t, []string{"removed:a", "added:c"}, kinds)
}

func TestComputeSkipsRemoteAndBuiltinContent(t *testing.T) {
	f := newFixture(t)
	f.rule("mine", "x\n")
	f.cfg.Content.Domains["shared"] = &config.Domain{Name: "shared", FromInclude: true,
		Rules: []config.ContentFile{{Name: "theirs", Path: filepath.Join(f.cfg.ConfigDir, "cache/x.md")}}}
	f.cfg.Content.Domains["pack"] = &config.Domain{Name: "pack", Builtin: true,
		Rules: []config.ContentFile{{Name: "builtin", Path: "builtin://pack/x.md"}}}
	f.cfg.Content.Rules = append(f.cfg.Content.Rules, config.ContentFile{Name: "outside", Path: "/elsewhere/outside.md"})
	assert.Equal(t, []string{"rule:/mine"}, keys(f.items()))
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestComputeDeclaredItems(t *testing.T) {
	f := newFixture(t)
	script := filepath.Join(f.root, "scripts", "hook.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(script), 0o755))
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755))
	f.cfg.Hooks = []config.HookGroup{
		{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Script: "scripts/hook.sh"}}},
		{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "echo"}}},
	}
	f.cfg.Roles = []config.RoleConfig{{Name: "dev", Domains: []string{"backend"}}}
	f.cfg.Permissions = &config.Permissions{Allow: []string{"Bash(git *)"}}
	items := f.items()
	assert.Contains(t, items, "hook:/PreToolUse:Bash:0")
	assert.Contains(t, items, "hook:/PreToolUse:Bash:1")
	assert.Contains(t, items, "role:/dev")
	assert.Contains(t, items, "settings:/permissions")

	before := items["hook:/PreToolUse:Bash:0"]
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nrm -rf /\n"), 0o755))
	assert.NotEqual(t, before, f.items()["hook:/PreToolUse:Bash:0"], "a hook script change is a pin change")

	snap, err := Compute(f.cfg, Options{Scope: config.LockScopeSkills})
	require.NoError(t, err)
	assert.Empty(t, snap.Items, "scope = skills pins no declared items")
}

func TestOutputsAreStableAcrossMetadataOnly(t *testing.T) {
	f := newFixture(t)
	out := Output{Path: "CLAUDE.md", Mode: 0o644, Data: []byte("# Rules\r\nx\r\n")}
	a, err := Compute(f.cfg, Options{IncludeOutputs: true, Outputs: []Output{out}})
	require.NoError(t, err)
	out.Data = bytes.ReplaceAll(out.Data, []byte("\r\n"), []byte("\n"))
	b, err := Compute(f.cfg, Options{IncludeOutputs: true, Outputs: []Output{out}})
	require.NoError(t, err)
	assert.Equal(t, a.Outputs, b.Outputs)
	none, err := Compute(f.cfg, Options{Outputs: []Output{out}})
	require.NoError(t, err)
	assert.Empty(t, none.Outputs, "outputs are only pinned with IncludeOutputs")
}

func TestCompare(t *testing.T) {
	f := newFixture(t)
	f.rule("style", "a\n")
	f.rule("gone", "g\n")
	opts := Options{IncludeOutputs: true, ToolVersion: "1.0.0", Outputs: []Output{{Path: "CLAUDE.md", Data: []byte("one")}}}
	snap, err := Compute(f.cfg, opts)
	require.NoError(t, err)
	var lock lockfile.File
	lock.Version = lockfile.Version
	Build(&lock, snap)

	same := Compare(&lock, snap)
	assert.True(t, same.InSync)
	assert.Empty(t, same.Changes)

	// source edit, source removal, source addition, output change
	f.write("rules/style.md", []byte("edited\n"), 0o644)
	f.cfg.Content.Rules = f.cfg.Content.Rules[:1]
	f.rule("fresh", "f\n")
	opts.Outputs = []Output{{Path: "CLAUDE.md", Data: []byte("two")}}
	opts.ToolVersion = "2.0.0"
	now, err := Compute(f.cfg, opts)
	require.NoError(t, err)
	diff := Compare(&lock, now)
	assert.False(t, diff.InSync)
	var lines []string
	for _, c := range diff.Changes {
		lines = append(lines, c.Scope+" "+c.Change+" "+c.ID)
		if c.Scope == ScopeOutput {
			lines[len(lines)-1] += c.Path
		}
	}
	assert.Equal(t, []string{
		"source added fresh", "source removed gone", "source changed style", "output changed CLAUDE.md",
	}, lines)
	assert.Len(t, diff.Notes, 1, "a tool version change is a note, not a failure")

	// a hand-edited pin without a matching tree digest is caught
	tampered := lock
	tampered.Item = append([]lockfile.Item(nil), lock.Item...)
	tampered.Item[0].Digest = "sha256:" + string(bytes.Repeat([]byte("0"), 64))
	diff = Compare(&tampered, snap)
	require.NotEmpty(t, diff.Changes)
	assert.Contains(t, diff.Changes[0].Line(), "edited by hand")

	// a v1 lock has no pins
	v1 := &lockfile.File{Version: 1}
	assert.True(t, Compare(v1, snap).NoPins)

	// settings mismatch
	only := Options{IncludeOutputs: false}
	noOut, err := Compute(f.cfg, only)
	require.NoError(t, err)
	assert.False(t, Compare(&lock, noOut).InSync)
}

func TestDuplicateIDsGetSuffix(t *testing.T) {
	f := newFixture(t)
	f.rule("dup", "one\n")
	p := f.write("rules/sub/dup.md", []byte("two\n"), 0o644)
	f.cfg.Content.Rules = append(f.cfg.Content.Rules, config.ContentFile{Name: "dup", Path: p})
	assert.ElementsMatch(t, []string{"rule:/dup", "rule:/dup#2"}, keys(f.items()))
}

func TestComputeHookScriptOutsideProjectIsAProblemNotAnAbort(t *testing.T) {
	f := newFixture(t)
	outside := filepath.Join(filepath.Dir(f.root), "outside-"+filepath.Base(f.root)+".sh")
	require.NoError(t, os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755))
	t.Cleanup(func() { _ = os.Remove(outside) })
	for _, script := range []string{"../" + filepath.Base(outside), outside} {
		f.cfg.Hooks = []config.HookGroup{{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Script: script}}}}
		snap, err := Compute(f.cfg, Options{})
		require.NoError(t, err, script)
		require.Len(t, snap.Problems, 1, script)
		assert.Contains(t, snap.Problems[0], "outside the project")

		lock := &lockfile.File{Version: lockfile.Version}
		Build(lock, snap)
		diff := Compare(lock, snap)
		assert.False(t, diff.InSync, "the unpinnable script keeps the check red")
		require.Len(t, diff.Changes, 1)
		assert.Equal(t, ScopeLock, diff.Changes[0].Scope)
	}
}

func TestCompareRequiresTreeDigest(t *testing.T) {
	f := newFixture(t)
	f.rule("style", "# Style\n")
	snap, err := Compute(f.cfg, Options{})
	require.NoError(t, err)
	lock := &lockfile.File{Version: lockfile.Version}
	Build(lock, snap)
	require.True(t, Compare(lock, snap).InSync)
	lock.Tree = ""
	diff := Compare(lock, snap)
	require.False(t, diff.InSync)
	assert.Contains(t, diff.Changes[0].Detail, "no tree digest")
}

func TestComputePinsMCPServersAtTheSource(t *testing.T) {
	f := newFixture(t)
	f.cfg.MCPServersRaw = []config.MCPServer{{Name: "b", Command: "node", Args: []string{"b.js"}}, {Name: "a", Command: "npx"}}
	before := f.items()["settings:/mcp-servers"]
	require.NotEmpty(t, before, "MCP servers are pinned as a settings item")

	f.cfg.MCPServersRaw[0].Args = []string{"evil.js"}
	assert.NotEqual(t, before, f.items()["settings:/mcp-servers"], "editing a server changes the source pin")

	snap, err := Compute(f.cfg, Options{Scope: config.LockScopeSkills})
	require.NoError(t, err)
	assert.Empty(t, snap.Items, "scope = skills pins no declared items")
}

func TestDuplicateIDsStayUniqueWhenARealIDEndsInSuffix(t *testing.T) {
	f := newFixture(t)
	f.rule("dup", "one\n")
	p := f.write("rules/sub/dup.md", []byte("two\n"), 0o644)
	f.cfg.Content.Rules = append(f.cfg.Content.Rules, config.ContentFile{Name: "dup", Path: p})
	f.rule("dup#2", "literal\n")
	got := keys(f.items())
	assert.Len(t, got, 3)
	assert.ElementsMatch(t, []string{"rule:/dup", "rule:/dup#2", "rule:/dup#2#2"}, got, "no two items share a key")
}

func TestComputePinsLegacyMCPServersLikeInlineOnes(t *testing.T) {
	f := newFixture(t)
	inline := config.MCPServer{Name: "a", Command: "npx"}
	f.cfg.MCPServersRaw = []config.MCPServer{inline}
	f.cfg.MCPServers = map[string]*config.MCPServer{"a": &inline}
	base := f.items()["settings:/mcp-servers"]
	require.NotEmpty(t, base)

	// A server the loader merged in from a legacy mcp.yaml is in the map only.
	legacy := config.MCPServer{Name: "legacy", Command: "node", Args: []string{"x.js"}}
	f.cfg.MCPServers["legacy"] = &legacy
	withLegacy := f.items()["settings:/mcp-servers"]
	assert.NotEqual(t, base, withLegacy, "a legacy MCP server is pinned")

	legacy.Args = []string{"evil.js"}
	assert.NotEqual(t, withLegacy, f.items()["settings:/mcp-servers"], "editing a legacy file changes the pin")

	only := newFixture(t)
	only.cfg.MCPServersRaw = []config.MCPServer{{Name: "a", Command: "npx"}}
	assert.Equal(t, base, only.items()["settings:/mcp-servers"], "projects without a legacy file keep their digest")
}

func TestComputeMCPPinIgnoresResolvedPlaceholders(t *testing.T) {
	// Arrange: a server written with a ${VAR} placeholder.
	f := newFixture(t)
	f.cfg.MCPServersRaw = []config.MCPServer{{Name: "a", Command: "npx", Env: map[string]string{"TOKEN": "${TOKEN}"}}}
	before := f.items()["settings:/mcp-servers"]
	require.NotEmpty(t, before)

	// Act: a render in the same process resolved the placeholder in the working copy.
	resolved := config.MCPServer{Name: "a", Command: "npx", Env: map[string]string{"TOKEN": "s3cret-value"}}
	f.cfg.MCPServers = map[string]*config.MCPServer{"a": &resolved}

	// Assert: the pin is still computed from the as-written server.
	assert.Equal(t, before, f.items()["settings:/mcp-servers"], "the pin must not depend on resolved values")
}

func TestTreeOfDetectsRelabelledRemoteEntries(t *testing.T) {
	entry := func(mutate func(*lockfile.Entry)) string {
		e := lockfile.Entry{Name: "pdf", Source: "https://example.com/a", Ref: "v1", Path: "skills/pdf", Commit: "c1", Digest: "sha256:d1"}
		mutate(&e)
		return TreeOf(&lockfile.File{Served: []lockfile.Entry{e}})
	}
	base := entry(func(*lockfile.Entry) {})
	tests := []struct {
		name   string
		mutate func(*lockfile.Entry)
	}{
		{"view", func(e *lockfile.Entry) { e.View = "role:backend" }},
		{"source", func(e *lockfile.Entry) { e.Source = "https://example.com/b" }},
		{"ref", func(e *lockfile.Entry) { e.Ref = "v2" }},
		{"path", func(e *lockfile.Entry) { e.Path = "skills/other" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEqual(t, base, entry(tt.mutate), "editing %s must change the tree digest", tt.name)
		})
	}
	// Swapping the views of two entries changes the digest too.
	a := lockfile.Entry{Name: "pdf", View: "role:a", Commit: "c", Digest: "sha256:1"}
	b := lockfile.Entry{Name: "pdf", View: "role:b", Commit: "c", Digest: "sha256:2"}
	swapped := lockfile.Entry{Name: "pdf", View: "role:a", Commit: "c", Digest: "sha256:2"}
	other := lockfile.Entry{Name: "pdf", View: "role:b", Commit: "c", Digest: "sha256:1"}
	assert.NotEqual(t, TreeOf(&lockfile.File{Served: []lockfile.Entry{a, b}}), TreeOf(&lockfile.File{Served: []lockfile.Entry{swapped, other}}))
}

func TestComputeIgnoresTheSynthesizedGuardHook(t *testing.T) {
	// Arrange
	f := newFixture(t)
	f.cfg.Hooks = []config.HookGroup{{Event: "PreToolUse", Matcher: "Bash", Hooks: []config.HookAction{{Command: "echo"}}}}
	before := f.items()

	// Act: a generation adds its built-in group, which embeds the binary version.
	f.cfg.Hooks = append(f.cfg.Hooks, config.HookGroup{
		Event: "PreToolUse", Matcher: "Write", Builtin: config.HookBuiltinGuard,
		Hooks: []config.HookAction{{Command: "ai-rulez guard v9.9.9"}},
	})
	after := f.items()

	// Assert
	assert.Equal(t, before, after)
}
