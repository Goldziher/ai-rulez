package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hugeMetadataCatalog has one skill whose description, keywords and file list
// are far larger than any sane skill, as a hostile source could make them.
func hugeMetadataCatalog(t *testing.T) *Catalog {
	t.Helper()
	var keywords []string
	for i := range 2000 {
		keywords = append(keywords, fmt.Sprintf("kw%d-%s", i, strings.Repeat("k", 200)))
	}
	var extra []generator.ServedSkillFile
	for i := range 2000 {
		extra = append(extra, generator.ServedSkillFile{RelPath: fmt.Sprintf("references/r%04d.md", i), Content: []byte("x")})
	}
	served := []generator.ServedSkill{servedSkill("big", "", strings.Repeat("huge ", 60_000), keywords, extra...)}
	cat, err := BuildCatalog("p", "claude", served, SkillFilter{})
	require.NoError(t, err)
	return cat
}

const maxToolReply = 64 << 10

func TestServedMetadataIsBounded(t *testing.T) {
	cat := hugeMetadataCatalog(t)
	p := startSkillServer(t, cat)

	_, _, text := callTool(t, p, "find_skill", map[string]any{"task": "huge"})
	assert.Less(t, len(text), maxToolReply, "find_skill reply")

	out, _, text := callTool(t, p, "list_skill_resources", map[string]any{"name": "big"})
	assert.Less(t, len(text), maxToolReply, "list_skill_resources reply")
	assert.Equal(t, true, out["truncated"])
	assert.InDelta(t, 2001, out["total"], 0)

	// the rest is reachable by paging
	next, _, _ := callTool(t, p, "list_skill_resources", map[string]any{"name": "big", "offset": maxListedResources})
	assert.NotEmpty(t, next["resources"])

	loaded, isErr, text := callTool(t, p, "load_skill", map[string]any{"name": "big", "budget_bytes": 100})
	require.False(t, isErr, text)
	assert.Less(t, len(text), maxToolReply, "load_skill metadata")
	assert.Equal(t, true, loaded["resources_truncated"])

	resp := p.call("skills/list", map[string]any{})
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Less(t, len(raw), maxToolReply, "skills/list reply")
}
