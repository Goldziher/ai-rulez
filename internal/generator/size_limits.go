package generator

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// agentsMDFile is the instruction file Codex concatenates from the project root
// down to the working directory.
const agentsMDFile = "AGENTS.md"

// maxSizeContributors is how many of the largest sections a size warning names.
const maxSizeContributors = 5

// sizeContributor is one section of an instruction file and its byte size.
type sizeContributor struct {
	Label string
	Bytes int
}

// sizeFinding is one instruction file (or file chain) over a harness limit.
type sizeFinding struct {
	Harness      string
	Path         string // repository-relative; for a chain, the deepest file
	Bytes        int
	Limit        int
	Chain        []string // files counted, root first; empty for a single file
	Contributors []sizeContributor
	Hint         string
}

// message is the one-line summary logged for the finding.
func (f sizeFinding) message() string {
	what := f.Path
	if len(f.Chain) > 1 {
		what = strings.Join(f.Chain, " + ")
	}
	return fmt.Sprintf("%s exceeds the %s limit: %d bytes, limit %d", what, f.Harness, f.Bytes, f.Limit)
}

// instructionSizeFindings checks the rendered instruction files against limits
// the harnesses document. Today that is Codex: it stops reading AGENTS.md
// content once the files it concatenates (root down to the working directory)
// reach project_doc_max_bytes, and ai-rulez inlines path-scoped rules into
// AGENTS.md for Codex, so a large rule set can be cut off silently. The
// per-file character limits of Windsurf and Antigravity rules are reported by
// the rule-file renderer.
//
// It reads the exact bytes writeOutput would put on disk.
func (g *Generator) instructionSizeFindings(outputs []config.OutputFile) []sizeFinding {
	if !g.config.HasBuiltInPreset("codex") {
		return nil
	}
	limit := g.config.Codex.ProjectDocLimit()
	if limit <= 0 {
		return nil
	}

	docs := g.agentsMDDocs(outputs)
	byDir := make(map[string]agentsMDDoc, len(docs))
	for _, d := range docs {
		byDir[d.dir] = d
	}

	var findings []sizeFinding
	for _, d := range docs {
		chain := chainOf(byDir, d)
		total := 0
		names := make([]string, 0, len(chain))
		var merged strings.Builder
		for _, c := range chain {
			total += len(c.content)
			names = append(names, c.rel)
			merged.WriteString(c.content)
		}
		if total <= limit {
			continue
		}
		f := sizeFinding{
			Harness:      "Codex project_doc_max_bytes",
			Path:         d.rel,
			Bytes:        total,
			Limit:        limit,
			Contributors: sectionContributors(merged.String()),
			Hint: "Codex stops adding AGENTS.md content at the limit, so trailing rules are dropped. Set compact = true, " +
				"move bulk content to skills, split with [[scopes]], or raise project_doc_max_bytes in Codex and set " +
				"[codex] project_doc_max_bytes to match",
		}
		if len(names) > 1 {
			f.Chain = names
		}
		findings = append(findings, f)
	}
	return findings
}

// agentsMDDoc is one rendered AGENTS.md as it lands on disk.
type agentsMDDoc struct {
	rel     string // repository-relative, slash separated
	dir     string // directory of rel, "." for the root
	content string
}

// agentsMDDocs returns the AGENTS.md outputs sorted by path.
func (g *Generator) agentsMDDocs(outputs []config.OutputFile) []agentsMDDoc {
	var docs []agentsMDDoc
	for _, output := range outputs {
		if output.IsDir || output.LocalOnly || output.RawContent != nil || filepath.Base(output.Path) != agentsMDFile {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(output.Path))
		docs = append(docs, agentsMDDoc{rel: rel, dir: filepath.ToSlash(filepath.Dir(rel)), content: g.finalContent(output)})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].rel < docs[j].rel })
	return docs
}

// chainOf lists the AGENTS.md files Codex reads for d's directory: the root
// file, every file in between, then d itself.
func chainOf(byDir map[string]agentsMDDoc, d agentsMDDoc) []agentsMDDoc {
	var chain []agentsMDDoc
	if root, ok := byDir["."]; ok {
		chain = append(chain, root)
	}
	for _, ancestor := range ancestorDirs(d.dir) {
		if a, ok := byDir[ancestor]; ok {
			chain = append(chain, a)
		}
	}
	if d.dir != "." {
		chain = append(chain, d)
	}
	return chain
}

// ancestorDirs lists the directories above dir, shallowest first, excluding the
// repository root and dir itself.
func ancestorDirs(dir string) []string {
	if dir == "." || dir == "" {
		return nil
	}
	parts := strings.Split(dir, "/")
	out := make([]string, 0, len(parts)-1)
	for i := 1; i < len(parts); i++ {
		out = append(out, strings.Join(parts[:i], "/"))
	}
	return out
}

// sectionContributors splits rendered markdown at its headings and returns the
// largest sections, biggest first, ties broken by label so the order is stable.
// A level 3 heading (one inlined rule or context entry) is labeled with the
// level 2 heading above it.
func sectionContributors(markdown string) []sizeContributor {
	var (
		parent  string
		label   = "(preamble)"
		bytes   int
		results []sizeContributor
	)
	flush := func() {
		if bytes > 0 {
			results = append(results, sizeContributor{Label: label, Bytes: bytes})
		}
	}
	inFence := false
	for _, line := range strings.SplitAfter(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if !inFence {
			switch {
			case strings.HasPrefix(line, "## "):
				flush()
				parent = strings.TrimSpace(strings.TrimPrefix(line, "## "))
				label, bytes = parent, 0
			case strings.HasPrefix(line, "### "):
				flush()
				name := strings.TrimSpace(strings.TrimPrefix(line, "### "))
				label, bytes = name, 0
				if parent != "" {
					label = parent + " > " + name
				}
			}
		}
		bytes += len(line)
	}
	flush()

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Bytes != results[j].Bytes {
			return results[i].Bytes > results[j].Bytes
		}
		return results[i].Label < results[j].Label
	})
	if len(results) > maxSizeContributors {
		results = results[:maxSizeContributors]
	}
	return results
}

// warnInstructionSizes logs one warning per over-limit instruction file.
func (g *Generator) warnInstructionSizes(outputs []config.OutputFile) {
	for _, f := range g.instructionSizeFindings(outputs) {
		parts := make([]string, len(f.Contributors))
		for i, c := range f.Contributors {
			parts[i] = fmt.Sprintf("%s (%d bytes)", c.Label, c.Bytes)
		}
		g.log().Warn(f.message(), "largest", strings.Join(parts, "; "), "hint", f.Hint)
	}
}
