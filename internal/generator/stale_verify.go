package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/Goldziher/ai-rulez/v5/internal/roles"
	"github.com/Goldziher/ai-rulez/v5/internal/usage"
)

// The generated manifest is a committed file anyone can edit, so a path it lists
// is only a claim. ai-rulez deletes a listed file only when both hold:
//
//  1. the path is one some preset could write (outputMatcher), so a forged entry
//     cannot reach secret.txt or a source file, and
//  2. the file proves ai-rulez wrote it: its Content-Hash matches its own body,
//     or (formats with no header) its digest equals the one this machine recorded
//     in the gitignored local manifest. A digest in the committed manifest proves
//     nothing, so a teammate's fresh clone removes header-less stale files never,
//     only warns.
//
// Anything else is left alone and reported.

// fileDigest is the digest the manifest records for a file without a header.
func fileDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// manifestDigests records the digest of every listed file that carries no
// Content-Hash of its own; base is the directory the entries are relative to.
func (g *Generator) manifestDigests(base string, files []string) map[string]string {
	var digests map[string]string
	for _, rel := range files {
		abs := filepath.Join(base, filepath.FromSlash(rel))
		if stored, _, _ := g.scanHashes(abs); stored != "" {
			continue
		}
		data, err := g.config.ReadExisting(abs)
		if err != nil {
			continue
		}
		if digests == nil {
			digests = map[string]string{}
		}
		digests[rel] = fileDigest(data)
	}
	return digests
}

// manifestDigestSet is the digests this machine recorded in the machine-local
// manifest. The committed manifest's digests are never proof: whoever edits that
// file can pick the digest of any file.
func (g *Generator) manifestDigestSet() map[string]string {
	set := map[string]string{}
	for rel, sum := range g.readManifest(g.localManifestPath()).Digests {
		set[rel] = sum
	}
	return set
}

// provablyGenerated reports whether the file at abs, listed in a manifest as rel,
// can be shown to be an unedited ai-rulez output.
func (g *Generator) provablyGenerated(rel, abs string, digests map[string]string) bool {
	data, err := g.config.ReadExisting(abs)
	if err != nil {
		return false
	}
	if stored, _, _ := g.scanHashes(abs); stored != "" {
		return !g.bodyEdited(string(data), abs)
	}
	want, ok := digests[rel]
	return ok && bytes.Equal([]byte(want), []byte(fileDigest(data)))
}

// legacyOutputPaths are folders and files earlier ai-rulez versions wrote for
// presets that have since moved or been removed (windsurf became devin,
// continue-dev is gone, ...). A manifest from those versions still lists them.
var legacyOutputPaths = []string{
	".windsurf", ".windsurfrules", ".continue", ".continue/rules", ".roo", ".roo/rules", ".codex/prompts",
	".codex/skills", ".codex/commands", ".github/commands", ".agents/agents", ".agents/rules", ".cursor/agents",
	".cursorrules", ".clinerules",
}

// outputMatcher recognizes the paths a preset could write.
type outputMatcher struct {
	exact  map[string]bool
	prefix []string
}

// sharedTopLevelDirs hold files that are not AI tool configuration (workflows,
// editor settings), so being a preset's root never vouches for what is below them.
var sharedTopLevelDirs = map[string]bool{
	".github": true, ".gitlab": true, ".vscode": true, ".idea": true, ".devcontainer": true,
	".husky": true, ".git": true, ".circleci": true,
}

func (m *outputMatcher) add(rel string, dir bool) {
	rel = strings.Trim(filepath.ToSlash(filepath.Clean(rel)), "/")
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
		return
	}
	m.exact[rel] = true
	if (dir || !hasExtension(rel)) && !sharedTopLevelDirs[rel] {
		m.prefix = append(m.prefix, rel)
	}
}

// hasExtension reports whether the last element has an extension; a dotfile such
// as .clinerules has none.
func hasExtension(rel string) bool {
	return strings.LastIndex(filepath.Base(rel), ".") > 0
}

func (m *outputMatcher) matches(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, candidate := range []string{rel, localVariantBase(rel)} {
		if m.matchesOne(candidate) {
			return true
		}
	}
	return false
}

func (m *outputMatcher) matchesOne(rel string) bool {
	for pattern := range m.exact {
		if rel == pattern || strings.HasSuffix(rel, "/"+pattern) {
			return true
		}
	}
	for _, dir := range m.prefix {
		if strings.HasPrefix(rel, dir+"/") || strings.Contains(rel, "/"+dir+"/") {
			return true
		}
	}
	return false
}

// localVariantBase maps CLAUDE.local.md to CLAUDE.md and AGENTS.override.md to
// AGENTS.md.
func localVariantBase(rel string) string {
	ext := filepath.Ext(rel)
	stem := strings.TrimSuffix(rel, ext)
	for _, infix := range []string{".local", ".override"} {
		if strings.HasSuffix(stem, infix) {
			return strings.TrimSuffix(stem, infix) + ext
		}
	}
	return rel
}

// outputMatcher builds the set of paths this config's presets, every built-in
// preset and the merged documents can write.
func (g *Generator) outputMatcher(outputs []config.OutputFile) *outputMatcher {
	m := &outputMatcher{exact: map[string]bool{}}
	base := g.config.BaseDir
	g.config.Registry.Each(func(_ string, gen config.PresetGenerator) {
		for _, p := range gen.GetOutputPaths(base) {
			if rel, err := filepath.Rel(base, p); err == nil {
				m.add(rel, false)
			}
		}
	})
	for _, legacy := range legacyOutputPaths {
		m.add(legacy, !hasExtension(legacy))
	}
	for _, hint := range providers.BuiltinPathHints() {
		m.add(hint, strings.HasSuffix(hint, "/") || !hasExtension(hint))
	}
	for _, doc := range mergedDocuments() {
		m.add(doc, false)
	}
	for _, output := range outputs {
		m.add(g.relSlash(g.absOutputPath(output.Path)), output.IsDir)
	}
	configDir := g.relSlash(g.manifestDir())
	for _, name := range []string{usage.IndexFileName, roles.FileName} {
		m.add(configDir+"/"+name, false)
	}
	if g.config.OKFEnabled() {
		m.add(g.config.OKFDir(), true)
	}
	return m
}

// wholeMergedDocuments lists the merged documents (manifest paths) this run wrote
// without any content of the user's: ai-rulez created them, so it may delete them
// once its keys leave.
func (g *Generator) wholeMergedDocuments(outputs []config.OutputFile) map[string]bool {
	whole := map[string]bool{}
	for _, output := range outputs {
		if output.IsDir || output.PartiallyOwned || len(output.MergeClaims) == 0 {
			continue
		}
		whole[g.relSlash(g.absOutputPath(output.Path))] = true
	}
	return whole
}

// addMergedDigests records the digest of each whole merged document claimed in
// merged, as written to disk.
func (g *Generator) addMergedDigests(digests *map[string]string, merged map[string][]jsonmerge.Claim, whole map[string]bool) {
	for rel := range merged {
		if !whole[rel] {
			continue
		}
		data, err := g.config.ReadExisting(filepath.Join(g.config.BaseDir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		if *digests == nil {
			*digests = map[string]string{}
		}
		(*digests)[rel] = fileDigest(data)
	}
}
