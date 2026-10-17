package generator

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitignore"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// maxTrackedListed bounds the files a tracked-by-the-block warning names.
const maxTrackedListed = 5

// trackedByBlock is a tracked file that only the managed block ignores.
type trackedByBlock struct {
	Path string
	Rule string
}

// trackedBlockWarnings lists the files git tracks that a line of the managed
// block in dir/.gitignore matches. git keeps tracking them, but any new file
// next to them is silently ignored. Machine-local and secret entries are left
// out: the local manifest has its own warning. It returns nil when git cannot
// answer.
func trackedBlockWarnings(ctx context.Context, git gitutil.Git, dir string, content string) []trackedByBlock {
	first, last := blockLineRange(content)
	if first == 0 {
		return nil
	}
	tracked, err := git.TrackedIgnoredContext(ctx, dir)
	if err != nil || len(tracked) == 0 {
		return nil
	}
	rules, err := git.IgnoreRulesContext(ctx, dir, tracked)
	if err != nil || rules == nil {
		return nil
	}
	var out []trackedByBlock
	for _, p := range tracked {
		m := rules[p]
		if filepath.Clean(m.Source) != gitignoreFileName || m.Line < first || m.Line > last || m.Negated() {
			continue
		}
		if strings.Contains(p, ".local.") || strings.HasSuffix(p, generatedLocalManifestName) || strings.HasSuffix(p, config.RootManifestNames()[1]) {
			continue
		}
		out = append(out, trackedByBlock{Path: p, Rule: m.Pattern})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// blockLineRange is the 1-based line span of the managed block, markers
// included, or 0, 0 when content holds none.
func blockLineRange(content string) (first, last int) {
	for i, line := range strings.Split(content, "\n") {
		switch strings.TrimSpace(line) {
		case gitignore.BeginMarker:
			if first == 0 {
				first = i + 1
			}
		case gitignore.EndMarker:
			last = i + 1
		}
	}
	return first, last
}

// warnTrackedByBlock reports the tracked files the managed block would hide new
// siblings of. The fix names the gitignore_exclude entry that keeps them out of
// the block, or git rm --cached to untrack them; an unanchored entry
// (AGENTS.md, CLAUDE.md) also matches in every nested root, which is called out.
func warnTrackedByBlock(warn func(msg string, args ...any), found []trackedByBlock) {
	if len(found) == 0 {
		return
	}
	names := make([]string, 0, len(found))
	rules := map[string]bool{}
	nested := false
	for i, f := range found {
		if i < maxTrackedListed {
			names = append(names, f.Path)
		}
		rules[f.Rule] = true
		if !strings.Contains(f.Rule, "/") && strings.Contains(f.Path, "/") {
			nested = true
		}
	}
	listed := strings.Join(names, ", ")
	if len(found) > maxTrackedListed {
		listed += fmt.Sprintf(" (+%d more)", len(found)-maxTrackedListed)
	}
	ruleList := make([]string, 0, len(rules))
	for r := range rules {
		ruleList = append(ruleList, r)
	}
	sort.Strings(ruleList)
	fix := "add the entries to gitignore_exclude (for example " + fmt.Sprintf("%q", ruleList[0]) + ") to keep committing them, or run git rm --cached on them"
	if nested {
		fix += "; an unanchored entry such as AGENTS.md also matches in every nested root, so use \"**/AGENTS.md\" in gitignore_exclude to commit them all"
	}
	warn("Tracked files match the managed .gitignore block: git keeps tracking them, but new files next to them are ignored",
		"files", listed, "rules", strings.Join(ruleList, ", "), "fix", fix)
}
