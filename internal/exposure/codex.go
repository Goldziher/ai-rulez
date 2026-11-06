package exposure

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

// The Codex load model, from the vendor pages cited in internal/harnesslimits
// and https://goldziher.github.io/ai-rulez/exposure/: AGENTS.md read from the repository root down (one file
// per directory, AGENTS.override.md first, 32 KiB combined), the skills of
// every .agents/skills directory from the working directory up to the root,
// and the [mcp_servers.*] tables of .codex/config.toml. Not modeled:
// project_doc_fallback_filenames, skills.max_context_tokens, user-level
// ($HOME/.codex, $HOME/.agents/skills) and admin (/etc/codex) files, the
// skill policy of agents/openai.yaml, and .codex/skills (the skills page does
// not name it).
const (
	agentsMDName       = "AGENTS.md"
	agentsOverrideName = "AGENTS.override.md"
	codexDir           = ".codex"
	agentsDir          = ".agents"
	codexConfigName    = "config.toml"
	codexMCPKey        = "mcp_servers"
)

// withCodexArtifacts adds the Codex context files a git listing leaves out
// (generated AGENTS.md and .agents/ are often gitignored).
func withCodexArtifacts(root string, files []string) []string {
	seen := make(map[string]bool, len(files)+8)
	for _, file := range files {
		seen[file] = true
	}
	add := func(rel string) {
		if !seen[rel] {
			files = append(files, rel)
			seen[rel] = true
		}
	}
	for _, name := range []string{agentsMDName, agentsOverrideName} {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.Mode().IsRegular() {
			add(name)
		}
	}
	for _, dir := range []string{agentsDir, codexDir} {
		//nolint:errcheck // an unreadable entry is left out, as in the listing
		_ = filepath.WalkDir(filepath.Join(root, dir), func(p string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil //nolint:nilerr // an unreadable entry is left out
			}
			if rel, relErr := filepath.Rel(root, p); relErr == nil {
				add(filepath.ToSlash(rel))
			}
			return nil
		})
	}
	return files
}

// collectCodex classifies one listed file under the Codex load model.
func (b *builder) collectCodex(file string) {
	segs := strings.Split(file, "/")
	dirs := segs[:len(segs)-1]
	name := segs[len(segs)-1]
	switch {
	case name == agentsOverrideName, name == agentsMDName:
		b.agentsMD(file, scopeOf(dirs))
	case name == codexConfigName && len(dirs) > 0 && dirs[len(dirs)-1] == codexDir:
		b.codexMCPServers(file, scopeOf(dirs[:len(dirs)-1]))
	case name == skillFile:
		if anchor := indexOf(segs, agentsDir); anchor >= 0 && len(segs)-anchor == 4 && segs[anchor+1] == dirSkills {
			b.codexSkill(file, scopeOf(segs[:anchor]), segs[anchor+2])
		}
	}
}

// agentsMD reads the AGENTS.md of a directory. Codex includes at most one file
// per directory and checks AGENTS.override.md first, so an AGENTS.md beside an
// override is not loaded and is left out.
func (b *builder) agentsMD(file, scope string) {
	if path.Base(file) == agentsMDName && b.hasFile(path.Join(path.Dir(file), agentsOverrideName)) {
		return
	}
	r := b.read(file)
	switch r.status {
	case readGone:
		return
	case readRefused:
		a := unreadArtifact(KindAgentsMD, SourceFile, file, scope, file, r.reason)
		a.unreadBytes = r.oversized
		b.artifacts = append(b.artifacts, a)
		return
	}
	b.artifacts = append(b.artifacts, Artifact{Kind: KindAgentsMD, Source: SourceFile, Name: file, Path: file, Scope: scope, Visibility: VisibilityAlways, Text: r.text})
}

func (b *builder) hasFile(rel string) bool {
	if b.fileSet == nil {
		b.fileSet = make(map[string]bool, len(b.files))
		for _, f := range b.files {
			b.fileSet[f] = true
		}
	}
	return b.fileSet[rel]
}

// codexSkill reads a skill of an .agents/skills directory. The listing line
// carries the skill's path, and none of Claude Code's gates apply: `paths:` and
// `disable-model-invocation` are Claude frontmatter, and the description is not
// cut at Claude's per-skill cap.
func (b *builder) codexSkill(file, scope, fallback string) {
	r := b.read(file)
	switch r.status {
	case readGone:
		return
	case readRefused:
		a := unreadArtifact(KindSkill, SourceFile, file, scope, fallback, r.reason)
		a.unreadBytes = r.oversized
		b.artifacts = append(b.artifacts, a)
		return
	}
	a := markdownArtifactCapped(KindSkill, SourceFile, file, scope, fallback, r.text, 0)
	a.Visibility, a.Paths, a.Phase = VisibilityAlways, nil, ""
	a.ListingText += " (" + file + ")"
	a.Text = a.ListingText
	b.artifacts = append(b.artifacts, a)
}

// codexMCPServers lists the [mcp_servers.<name>] tables of a .codex/config.toml,
// except a server set to `enabled = false`.
func (b *builder) codexMCPServers(file, scope string) {
	r := b.read(file)
	if r.status != readOK {
		return
	}
	var doc map[string]any
	if err := toml.Unmarshal([]byte(r.text), &doc); err != nil {
		b.problem(file, err.Error())
		return
	}
	servers, _ := doc[codexMCPKey].(map[string]any) //nolint:errcheck // a missing or mistyped table lists none
	names := make([]string, 0, len(servers))
	for name, v := range servers {
		if table, ok := v.(map[string]any); ok && table["enabled"] != false {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		b.artifacts = append(b.artifacts, Artifact{Kind: KindMCPServer, Source: SourceFile, Name: name, Path: file, Scope: scope, Visibility: VisibilityAlways, Text: name})
	}
}

// capCodexInstructions applies the combined AGENTS.md size limit to a
// start-of-session AGENTS.md: Codex stops adding bytes past it. At the
// repository root the chain is the one file.
func capCodexInstructions(a *Artifact) {
	if a.Kind != KindAgentsMD {
		return
	}
	if limit := harnesslimits.MustValue("codex.agents_md_chain_bytes"); a.TextBytes > limit {
		a.Tokens = EstimateTokens(limit)
		a.Truncated = true
	}
}
