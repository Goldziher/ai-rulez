package exposure

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

const (
	claudeDir    = ".claude"
	claudeMDName = "CLAUDE.md"
	mcpFileName  = ".mcp.json"
	skillFile    = "SKILL.md"
	markdownExt  = ".md"
	dirRules     = "rules"
	dirSkills    = "skills"
	dirCommands  = "commands"
	dirAgents    = "agents"
	commandSep   = ":"
	listingSep   = ": "
)

// The frontmatter keys the load model reads.
const (
	keyName      = "name"
	keyDesc      = "description"
	keyWhenToUse = "when_to_use"
	keyPaths     = "paths"
	keyDisable   = "disable-model-invocation"
)

// listingCap is the characters of description plus when_to_use one skill keeps
// in the listing.
func listingCap() int { return harnesslimits.MustValue("claude.skill_listing_chars") }

// collect classifies one listed file and reads the artifacts it holds.
func (b *builder) collect(file string) {
	segs := strings.Split(file, "/")
	dirs := segs[:len(segs)-1]
	switch segs[len(segs)-1] {
	case claudeMDName:
		// .claude/CLAUDE.md is the CLAUDE.md of the directory that holds .claude.
		if len(dirs) > 0 && dirs[len(dirs)-1] == claudeDir {
			dirs = dirs[:len(dirs)-1]
		}
		b.markdown(KindClaudeMD, SourceFile, file, scopeOf(dirs), file)
	case mcpFileName:
		b.mcpServers(file, scopeOf(dirs))
	default:
		if anchor := indexOf(segs, claudeDir); anchor >= 0 && anchor < len(segs)-1 {
			b.collectClaudeDir(file, scopeOf(segs[:anchor]), segs[anchor+1:])
		}
	}
}

// collectClaudeDir classifies a file below the first .claude directory of its
// path; inner is its path below that directory.
func (b *builder) collectClaudeDir(file, scope string, inner []string) {
	name := inner[len(inner)-1]
	isMD := pySuffix(name) == markdownExt
	below := pyWithoutSuffix(path.Join(inner[1:]...))
	switch {
	case inner[0] == dirRules && isMD && len(inner) > 1:
		b.markdown(KindRule, SourceFile, file, scope, below)
	case inner[0] == dirSkills && len(inner) == 3 && inner[2] == skillFile:
		b.markdown(KindSkill, SourceFile, file, scope, inner[1])
	case inner[0] == dirCommands && isMD && len(inner) > 1:
		b.markdown(KindSkill, SourceCommand, file, scope, strings.ReplaceAll(below, "/", commandSep))
	case inner[0] == dirAgents && len(inner) == 2 && isMD:
		b.markdown(KindAgent, SourceFile, file, scope, pyWithoutSuffix(name))
	case len(inner) == 1 && isSettingsFile(name):
		b.settingsArtifacts(file, scope)
	}
}

// markdown reads a markdown artifact.
func (b *builder) markdown(kind Kind, source Source, file, scope, fallback string) {
	if a, ok := b.markdownFile(kind, source, file, scope, fallback); ok {
		b.artifacts = append(b.artifacts, a)
	}
}

// markdownFile reads a markdown artifact; ok is false for a file that is gone.
// A file that was refused is kept unread (see unreadArtifact).
func (b *builder) markdownFile(kind Kind, source Source, file, scope, fallback string) (Artifact, bool) {
	r := b.read(file)
	switch r.status {
	case readGone:
		return Artifact{}, false
	case readRefused:
		a := unreadArtifact(kind, source, file, scope, fallback, r.reason)
		a.unreadBytes = r.oversized
		return a, true
	}
	return markdownArtifact(kind, source, file, scope, fallback, r.text), true
}

// unreadArtifact is a markdown artifact whose file was not read (over the read
// limit, or a symlink to outside the root). It keeps its place in the
// inventory: the conventional name, the visibility and phase of its path, an
// empty text and the reason as its frontmatter error. Whether a root rule or
// skill is gated cannot be known, so its phase is unknown.
func unreadArtifact(kind Kind, source Source, file, scope, fallback, reason string) Artifact {
	a := Artifact{
		Kind: kind, Source: source, Name: fallback, Path: file, Scope: scope, Visibility: VisibilityAlways,
		Frontmatter: &FrontmatterStatus{Error: unreadPrefix + reason},
	}
	if gated(kind) && scope == RootScope {
		a.Phase = PhaseUnknown
	}
	return a
}

// unreadPrefix leads the frontmatter error of an artifact kept unread.
const unreadPrefix = "not read: "

// UnreadReason is why the file of an artifact kept unread was not read (see
// unreadArtifact), "" for an artifact that was read.
func (a *Artifact) UnreadReason() string {
	if a.Frontmatter == nil {
		return ""
	}
	reason, ok := strings.CutPrefix(a.Frontmatter.Error, unreadPrefix)
	if !ok {
		return ""
	}
	return reason
}

// header is the frontmatter the load model reads.
type header struct {
	name, description, whenToUse string
	paths                        []string
	disableModelInvocation       bool
}

// markdownArtifact builds a CLAUDE.md, rule, skill or agent from its text.
func markdownArtifact(kind Kind, source Source, file, scope, fallback, text string) Artifact {
	block := frontmatter.SplitString(text)
	h, err := decodeHeader(block)
	a := Artifact{Kind: kind, Source: source, Path: file, Scope: scope, Paths: h.paths, Frontmatter: &FrontmatterStatus{OK: err == nil}}
	if err != nil {
		a.Frontmatter.Error = err.Error()
	}
	a.Name = h.name
	if a.Name == "" {
		a.Name = fallback
	}
	a.Visibility = visibilityOf(kind, h)
	switch kind {
	case KindSkill, KindAgent:
		desc := listingDescription(h)
		if kind == KindSkill {
			desc, a.Truncated = capRunes(desc, listingCap())
		}
		a.ListingText = a.Name + listingSep + desc
		a.Text = a.ListingText
	default:
		a.Text, a.LineOffset = bodyOf(text, block)
	}
	if err != nil && gated(kind) && scope == RootScope && frontmatter.NamesKey(block.Raw, frontmatter.GateKeys()...) {
		a.Phase = PhaseUnknown
	}
	return a
}

// gated reports whether a kind honors a frontmatter gate.
func gated(kind Kind) bool { return kind == KindSkill || kind == KindRule }

// decodeHeader decodes the frontmatter of a block. On an error the header is
// empty, as Claude Code reads a block it cannot parse: no name, description or
// gate.
func decodeHeader(block frontmatter.Block) (header, error) {
	front, err := frontmatter.Decode(block)
	if err != nil {
		return header{}, err //nolint:wrapcheck // the decode error is the reported status
	}
	var h header
	for _, f := range []struct {
		key string
		dst *string
	}{{keyName, &h.name}, {keyDesc, &h.description}, {keyWhenToUse, &h.whenToUse}} {
		v, ok := front[f.key]
		if !ok || v == nil {
			continue
		}
		s, isString := v.(string)
		if !isString {
			return header{}, fmt.Errorf("frontmatter %q is not a string", f.key)
		}
		*f.dst = s
	}
	if v, ok := front[keyPaths]; ok {
		paths, pathsOK := frontmatter.StringList(v)
		if !pathsOK {
			return header{}, fmt.Errorf("frontmatter %q is not a string or a list of strings", keyPaths)
		}
		h.paths = paths
	}
	if v, ok := front[keyDisable]; ok {
		on, isBool := v.(bool)
		if !isBool {
			return header{}, fmt.Errorf("frontmatter %q is not a boolean", keyDisable)
		}
		h.disableModelInvocation = on
	}
	return h, nil
}

func visibilityOf(kind Kind, h header) Visibility {
	switch {
	case kind == KindSkill && h.disableModelInvocation:
		return VisibilityManualOnly
	case gated(kind) && len(h.paths) > 0:
		return VisibilityPathGated
	}
	return VisibilityAlways
}

// listingDescription is description and when_to_use, each whitespace squashed,
// joined by a space. A part that is set counts even when it squashes to nothing.
func listingDescription(h header) string {
	var parts []string
	for _, p := range []string{h.description, h.whenToUse} {
		if p != "" {
			parts = append(parts, strings.Join(strings.Fields(p), " "))
		}
	}
	return strings.Join(parts, " ")
}

// capRunes cuts s to at most n characters and reports whether it cut.
func capRunes(s string, n int) (string, bool) {
	if utf8.RuneCountInString(s) <= n {
		return s, false
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos], true
		}
		i++
	}
	return s, false
}

// bodyOf is the text a CLAUDE.md or rule puts in context and the lines before
// it: the whole file without a closed block, else the lines after the closing
// fence joined by LF.
func bodyOf(text string, block frontmatter.Block) (body string, offset int) {
	if !block.Closed {
		return block.Body, 0
	}
	return strings.Join(splitLines(block.Body), "\n"), block.BodyLine - 1
}

func scopeOf(dirs []string) string {
	if len(dirs) == 0 {
		return RootScope
	}
	return strings.Join(dirs, "/")
}

func indexOf(segs []string, s string) int {
	for i, seg := range segs {
		if seg == s {
			return i
		}
	}
	return -1
}
