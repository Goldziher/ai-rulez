package agentplugins

import (
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// FuzzValidateImport feeds arbitrary plugin.json, mcp.json and SKILL.md bytes
// and one symbolic-link target through an in-memory plugin tree.
func FuzzValidateImport(f *testing.F) {
	manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"acme.tools"}`
	mcp := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"v":{"type":"stdio","command":"./bin/v"}}}`
	skill := "---\nname: deploy\ndescription: d\n---\nbody\n"
	f.Add(manifest, mcp, skill, "../outside")
	f.Add(manifest, mcp, skill, "bin/v")
	f.Add(manifest, mcp, skill, "/etc/passwd")
	f.Add(manifest, mcp, skill, "loop")
	f.Add(manifest, mcp, skill, "")
	f.Add("{}", "{}", "", "..")
	f.Add("[]", "null", "---", `a\b`)
	f.Add(strings.Replace(manifest, "1.0.0", "1.1.0", 1), mcp, skill, "skills/deploy")
	f.Add(manifest, `{"mcpServers":{"x":{"type":"stdio","command":"npx -y srv","cwd":"${PLUGIN_DATA}/../x"}}}`, skill, "x")
	f.Add(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"a","extensions":{"com.example.c":{"k":1e400}}}`, mcp, skill, "ext")
	f.Fuzz(func(t *testing.T, plugin, mcpDoc, skillMD, link string) {
		// Arrange
		m := fstest.MapFS{
			"plugin.json":             {Data: []byte(plugin), Mode: 0o644},
			"mcp.json":                {Data: []byte(mcpDoc), Mode: 0o644},
			"skills/deploy/SKILL.md":  {Data: []byte(skillMD), Mode: 0o644},
			"skills/deploy/res/l.txt": {Data: []byte(link), Mode: fs.ModeSymlink},
			"bin/v":                   {Data: []byte("#!/bin/sh\n"), Mode: 0o755},
		}

		// Act
		model, res := Import(m)
		validated := Validate(m)

		// Assert
		if model == nil != res.Rejected {
			t.Fatalf("model nil=%v but rejected=%v", model == nil, res.Rejected)
		}
		if validated.Rejected != res.Rejected || len(validated.Findings) != len(res.Findings) {
			t.Fatalf("Validate and Import disagree: %+v vs %+v", validated, res)
		}
		if !slices.IsSortedFunc(res.Findings, func(a, b Finding) int { return strings.Compare(a.Path, b.Path) }) {
			t.Fatalf("findings are not ordered by path: %+v", res.Findings)
		}
		for _, fd := range res.Findings {
			if fd.Code == "" || strings.Contains(fd.Path, "..") {
				t.Fatalf("malformed finding %+v", fd)
			}
		}
		if model == nil {
			return
		}
		// An accepted plugin builds, and what is built imports to the same files.
		files, _, err := Build(model, Options{Spec: res.Spec})
		if err != nil {
			return
		}
		again, reimported := Import(mapFS(files))
		if again == nil {
			t.Fatalf("a built plugin is rejected on import: %+v", reimported)
		}
		rebuilt, _, err := Build(again, Options{Spec: reimported.Spec})
		if err != nil {
			t.Fatalf("a re-imported plugin does not build: %v", err)
		}
		if !maps.EqualFunc(files, rebuilt, func(a, b []byte) bool { return string(a) == string(b) }) {
			t.Fatalf("Build(Import(Build(p))) differs from Build(p)")
		}
	})
}
