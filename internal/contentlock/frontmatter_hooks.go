package contentlock

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
)

// Agents, skills and commands can declare `hooks` in their own frontmatter. The
// declaration is part of the file the item digest already covers; the scripts
// those hooks run are not, so editing a script would leave the lock green. They
// are pinned here, exactly as the scripts of top-level [[hooks]] are.

// hookScriptRoots are the variables harnesses expand to the project directory.
var hookScriptRoots = []string{"${CLAUDE_PROJECT_DIR}/", "$CLAUDE_PROJECT_DIR/", "${CODEX_PROJECT_DIR}/", "$CODEX_PROJECT_DIR/"}

// frontmatterHookScripts returns the project-relative scripts the command
// handlers of a frontmatter hooks block run: a word that starts with ./, ../ or
// the project-directory variable. It returns nil for content without hooks.
func frontmatterHookScripts(primary []byte) []string {
	fm, ok := frontmatterBlock(string(primary))
	if !ok {
		return nil
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `yaml:"type"`
				Command string `yaml:"command"`
			} `yaml:"hooks"`
		} `yaml:"hooks"`
	}
	if yaml.Unmarshal([]byte(fm), &doc) != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, groups := range doc.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if h.Type != "" && h.Type != "command" {
					continue
				}
				for _, word := range commandWords(h.Command) {
					if rel, ok := scriptPath(word); ok && !seen[rel] {
						seen[rel] = true
						out = append(out, rel)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// frontmatterBlock returns the YAML between the leading --- fences.
func frontmatterBlock(content string) (string, bool) {
	block := frontmatter.SplitString(content)
	if !block.Closed {
		return "", false
	}
	return strings.TrimSuffix(block.Raw, "\n"), true
}

// commandWords splits a command line on whitespace and shell separators and
// strips quotes, which is enough to find the path-shaped words in it.
func commandWords(cmd string) []string {
	return strings.FieldsFunc(cmd, func(r rune) bool {
		return strings.ContainsRune(" \t\r\n;&|()<>", r)
	})
}

func scriptPath(word string) (string, bool) {
	word = strings.Trim(word, `"'`)
	for _, root := range hookScriptRoots {
		if strings.HasPrefix(word, root) {
			return filepath.ToSlash(filepath.Clean(strings.TrimPrefix(word, root))), true
		}
	}
	if strings.HasPrefix(word, "./") || strings.HasPrefix(word, "../") {
		return filepath.ToSlash(filepath.Clean(word)), true
	}
	return "", false
}

// addFrontmatterHookScripts appends one leaf per script the item's frontmatter
// hooks run. Scripts outside the project are never read: the declaration is
// pinned and the lock reports a problem, as for top-level hooks.
func (c *collector) addFrontmatterHookScripts(kind, id string, primary []byte, leaves []Leaf) []Leaf {
	if kind != KindAgent && kind != KindSkill && kind != KindCommand {
		return leaves
	}
	for _, rel := range frontmatterHookScripts(primary) {
		if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
			c.problems = append(c.problems, fmt.Sprintf("%s %s frontmatter hook script %q is outside the project, so its content cannot be pinned; move it into the project", kind, id, rel))
			leaf := Leaf{Path: "hook-outside/" + outsideName(rel), Mode: ModeRegular, Data: []byte(rel)}
			if !containsPath(leaves, leaf.Path) {
				leaves = append(leaves, leaf)
			}
			continue
		}
		abs := filepath.Join(c.cfg.BaseDir, filepath.FromSlash(rel))
		leaf := Leaf{Path: "hook-script/" + rel, Mode: ModeRegular}
		if disk, err := os.ReadFile(abs); err == nil {
			leaf.Data = disk
			if info, statErr := os.Stat(abs); statErr == nil {
				leaf.Mode = c.modes.mode(c.cfg.BaseDir, abs, info)
			}
		} else {
			leaf.Path = "hook-missing/" + rel
		}
		if !containsPath(leaves, leaf.Path) {
			leaves = append(leaves, leaf)
		}
	}
	return leaves
}
