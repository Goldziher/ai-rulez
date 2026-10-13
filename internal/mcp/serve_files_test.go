package mcp

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bundleCatalog is a served skill with a 0755 script, a 0644 helper and a binary asset.
func bundleCatalog(t *testing.T) (*Catalog, []byte) {
	t.Helper()
	blob := make([]byte, 600*1024)
	for i := range blob {
		blob[i] = byte(i%251) | 0x80 // never valid UTF-8
	}
	served := []generator.ServedSkill{servedSkill("served", "", "Ships a script", nil,
		generator.ServedSkillFile{RelPath: "assets/b.bin", Content: blob, Mode: 0o644},
		generator.ServedSkillFile{RelPath: "scripts/lib/x.sh", Content: []byte("echo x\n"), Mode: 0o644},
		generator.ServedSkillFile{RelPath: "scripts/run.py", Content: []byte("#!/usr/bin/env python3\nprint(1)\n"), Mode: 0o755},
		generator.ServedSkillFile{RelPath: "scripts/tool.sh", Content: []byte("#!/bin/sh\necho hi\n"), Mode: 0o755},
	)}
	cat, err := BuildCatalog("p", "claude", served, SkillFilter{})
	require.NoError(t, err)
	return cat, blob
}

func resourceByPath(t *testing.T, out map[string]any) map[string]map[string]any {
	t.Helper()
	byPath := map[string]map[string]any{}
	for _, r := range out["resources"].([]any) {
		m := r.(map[string]any)
		byPath[m["path"].(string)] = m
	}
	return byPath
}

func TestListSkillResources_ReportsModeExecutableAndInterpreter(t *testing.T) {
	t.Parallel()
	cat, _ := bundleCatalog(t)
	p, _ := startSkillServerWith(t, cat, ServeOptions{})

	out, isErr, text := callTool(t, p, "list_skill_resources", map[string]any{"name": "served"})
	require.False(t, isErr, text)
	got := resourceByPath(t, out)

	tests := []struct {
		path, mode, interpreter string
		executable              bool
	}{
		{"scripts/tool.sh", "0755", "/bin/sh", true},
		{"scripts/run.py", "0755", "/usr/bin/env python3", true},
		{"scripts/lib/x.sh", "0644", "sh", false},
		{"assets/b.bin", "0644", "", false},
		{"SKILL.md", "0644", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			entry := got[tt.path]
			require.NotNil(t, entry)
			assert.Equal(t, tt.mode, entry["mode"])
			assert.Equal(t, tt.executable, entry["executable"])
			if tt.interpreter == "" {
				assert.NotContains(t, entry, "interpreter")
			} else {
				assert.Equal(t, tt.interpreter, entry["interpreter"])
			}
			for _, k := range []string{"path", "uri", "size", "digest", "mime"} {
				assert.Contains(t, entry, k, "existing field %s stays", k)
			}
		})
	}
}

func TestLoadSkill_StatesWhereFilesLiveAndHowToUseThem(t *testing.T) {
	t.Parallel()
	cat, _ := bundleCatalog(t)
	p, _ := startSkillServerWith(t, cat, ServeOptions{})

	out, isErr, text := callTool(t, p, "load_skill", map[string]any{"name": "served"})
	require.False(t, isErr, text)

	note, _ := out["files_access"].(string)
	for _, want := range []string{"not on disk", "read_skill_file", "base64", "chmod +x", "delivery", "both"} {
		assert.Contains(t, note, want)
	}
	entry := resourceByPath(t, out)["scripts/tool.sh"]
	assert.Equal(t, "0755", entry["mode"])
	assert.Equal(t, true, entry["executable"])

	t.Run("a skill without supporting files has no note", func(t *testing.T) {
		plain, err := BuildCatalog("p", "claude", []generator.ServedSkill{servedSkill("plain", "", "Only SKILL.md", nil)}, SkillFilter{})
		require.NoError(t, err)
		p, _ := startSkillServerWith(t, plain, ServeOptions{})
		out, isErr, _ := callTool(t, p, "load_skill", map[string]any{"name": "plain"})
		require.False(t, isErr)
		assert.NotContains(t, out, "files_access")
	})
}

func TestResourceMetadata_CarriesMode(t *testing.T) {
	t.Parallel()
	cat, _ := bundleCatalog(t)
	p, _ := startSkillServerWith(t, cat, ServeOptions{})

	t.Run("skills/get", func(t *testing.T) {
		resp := p.call("skills/get", map[string]any{"uri": "skill://served/SKILL.md"})
		require.Nil(t, resp["error"])
		var found map[string]any
		for _, r := range resp["result"].(map[string]any)["skill"].(map[string]any)["resources"].([]any) {
			if strings.HasSuffix(r.(map[string]any)["uri"].(string), "scripts/tool.sh") {
				found = r.(map[string]any)
			}
		}
		require.NotNil(t, found)
		assert.Contains(t, found, "digest", "existing fields stay")
		meta := found["_meta"].(map[string]any)
		assert.Equal(t, "0755", meta[skillsMetaPrefix+"mode"])
		assert.Equal(t, true, meta[skillsMetaPrefix+"executable"])
		assert.Equal(t, "/bin/sh", meta[skillsMetaPrefix+"interpreter"])
	})

	t.Run("resources/list", func(t *testing.T) {
		resp := p.call("resources/list", map[string]any{})
		require.Nil(t, resp["error"])
		var found map[string]any
		for _, r := range resp["result"].(map[string]any)["resources"].([]any) {
			if r.(map[string]any)["uri"] == "skill://served/scripts/tool.sh" {
				found = r.(map[string]any)
			}
		}
		require.NotNil(t, found)
		meta := found["_meta"].(map[string]any)
		assert.Equal(t, "0755", meta[skillsMetaPrefix+"mode"])
		assert.Equal(t, true, meta[skillsMetaPrefix+"executable"])
		assert.Contains(t, meta, skillsMetaPrefix+"digest")
	})
}

func TestReadSkillFile_EncodingAndChunks(t *testing.T) {
	t.Parallel()
	cat, blob := bundleCatalog(t)
	p, _ := startSkillServerWith(t, cat, ServeOptions{BudgetBytes: -1}) // the 600 KB asset exceeds the default cap
	const bin = "skill://served/assets/b.bin"

	decode := func(t *testing.T, out map[string]any) []byte {
		t.Helper()
		raw, err := base64.StdEncoding.DecodeString(out["content"].(string))
		require.NoError(t, err)
		return raw
	}

	t.Run("a text file reads as before", func(t *testing.T) {
		out, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": "skill://served/scripts/tool.sh"})
		require.False(t, isErr, text)
		assert.Equal(t, "#!/bin/sh\necho hi\n", out["content"])
		assert.Equal(t, "0755", out["mode"])
		assert.Equal(t, true, out["executable"])
	})

	t.Run("a binary file comes back as base64", func(t *testing.T) {
		out, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": bin, "encoding": "base64", "length": 1024})
		require.False(t, isErr, text)
		assert.Equal(t, "base64", out["encoding"])
		assert.Equal(t, blob[:1024], decode(t, out))
		assert.InDelta(t, 1024, out["next_offset"], 0)
		assert.InDelta(t, len(blob), out["size"], 0)
	})

	t.Run("a binary file without an encoding defaults to base64", func(t *testing.T) {
		out, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": bin, "length": 16})
		require.False(t, isErr, text)
		assert.Equal(t, "base64", out["encoding"])
		assert.Equal(t, blob[:16], decode(t, out))
	})

	t.Run("chunks reassemble the whole file", func(t *testing.T) {
		var got []byte
		offset := 0
		for range 10 {
			out, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": bin, "encoding": "base64", "offset": offset, "length": 200 * 1024})
			require.False(t, isErr, text)
			got = append(got, decode(t, out)...)
			next, more := out["next_offset"]
			if !more {
				break
			}
			offset = int(next.(float64))
		}
		assert.True(t, bytes.Equal(blob, got))
	})

	t.Run("the default chunk is bounded", func(t *testing.T) {
		out, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": bin, "encoding": "base64"})
		require.False(t, isErr, text)
		assert.LessOrEqual(t, len(decode(t, out)), maxReadChunkBytes)
		assert.Contains(t, out, "next_offset")
	})

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"text encoding on a binary file", map[string]any{"uri": bin, "encoding": "text"}, "binary"},
		{"unknown encoding", map[string]any{"uri": bin, "encoding": "hex"}, "encoding"},
		{"offset past the end", map[string]any{"uri": bin, "offset": len(blob) + 1}, "offset"},
		{"negative offset", map[string]any{"uri": bin, "offset": -1}, "offset"},
		{"unknown file", map[string]any{"uri": "skill://served/nope"}, "no served skill file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, isErr, text := callTool(t, p, "read_skill_file", tt.args)
			assert.True(t, isErr)
			assert.Contains(t, text, tt.want)
		})
	}
}

func TestReadSkillFile_TextChunksStayValidUTF8(t *testing.T) {
	t.Parallel()
	body := strings.Repeat("héllo wörld\n", 50)
	cat, err := BuildCatalog("p", "claude", []generator.ServedSkill{servedSkill("txt", "", "d", nil,
		generator.ServedSkillFile{RelPath: "references/a.md", Content: []byte(body)})}, SkillFilter{})
	require.NoError(t, err)
	p, _ := startSkillServerWith(t, cat, ServeOptions{})

	var got strings.Builder
	offset := 0
	for range 100 {
		out, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": "skill://txt/references/a.md", "offset": offset, "length": 7})
		require.False(t, isErr, text)
		got.WriteString(out["content"].(string))
		next, more := out["next_offset"]
		if !more {
			break
		}
		offset = int(next.(float64))
	}
	assert.Equal(t, body, got.String())
}

func TestReadSkillFile_ChunksChargeTheSessionBudget(t *testing.T) {
	t.Parallel()
	cat, _ := bundleCatalog(t)
	p, _ := startSkillServerWith(t, cat, ServeOptions{BudgetBytes: 1500})

	_, isErr, text := callTool(t, p, "read_skill_file", map[string]any{"uri": "skill://served/assets/b.bin", "length": 1000})
	require.False(t, isErr, text)
	_, isErr, text = callTool(t, p, "read_skill_file", map[string]any{"uri": "skill://served/assets/b.bin", "offset": 1000, "length": 1000})
	assert.True(t, isErr)
	assert.Contains(t, text, "budget")
}

func TestInterpreterHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, rel, content, want string
	}{
		{"shebang wins over extension", "scripts/a.sh", "#!/usr/bin/env python3\n", "/usr/bin/env python3"},
		{"shebang with a flag", "scripts/a", "#!/bin/bash -e\nx", "/bin/bash -e"},
		{"extension py", "scripts/a.py", "print(1)", "python3"},
		{"extension js", "scripts/a.js", "x", "node"},
		{"extension sh", "scripts/a.sh", "x", "sh"},
		{"none", "scripts/a.txt", "x", ""},
		{"binary content has none", "scripts/a.sh", "\xff\xfe\x00", ""},
		{"shebang too long is ignored", "scripts/a.txt", "#!" + strings.Repeat("x", 400) + "\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, interpreterHint(tt.rel, []byte(tt.content)))
		})
	}
}
