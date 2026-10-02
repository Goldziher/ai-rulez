package rulefiles

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// Dialect selects the frontmatter vocabulary a tool understands.
type Dialect string

// Supported dialects.
const (
	DialectClaude   Dialect = "claude"
	DialectCursor   Dialect = "cursor"
	DialectTrigger  Dialect = "trigger" // windsurf, antigravity
	DialectCopilot  Dialect = "copilot"
	DialectCline    Dialect = "cline"
	DialectContinue Dialect = "continue"
	DialectJunie    Dialect = "junie"
)

// Target describes one rules folder a preset writes into.
type Target struct {
	Preset    string
	Dir       string // rules directory relative to the output root
	Ext       string // file extension including the dot, e.g. ".md" or ".instructions.md"
	Dialect   Dialect
	Recursive bool // the tool discovers rules in subdirectories
	MaxChars  int  // soft per-file limit; 0 means unlimited
	Banner    bool // emit the generated-file banner
}

// Kind distinguishes rules from context files.
type Kind int

// Item kinds.
const (
	KindRule Kind = iota
	KindContext
)

// Item is one rule or context file bound for a rules folder.
type Item struct {
	File       config.ContentFile
	Kind       Kind
	ID         string
	Activation config.Activation
}

const contextPrefix = "context-"

// FileName returns the file name (relative to Target.Dir) of an item:
// "<id><ext>" for rules and "context-<id><ext>" for context.
func FileName(t Target, it Item) string {
	if it.Kind == KindContext {
		// A scoped recursive ID is "<slug>/<id>"; the prefix goes on the last segment.
		dir, base := "", it.ID
		if idx := strings.LastIndex(it.ID, "/"); idx >= 0 {
			dir, base = it.ID[:idx+1], it.ID[idx+1:]
		}
		return dir + contextPrefix + base + t.Ext
	}
	return it.ID + t.Ext
}

// LocalPath returns the path of the personal override of a rule:
// "<dir>/<id>.local<ext>".
func LocalPath(t Target, id string) string {
	dir := strings.TrimRight(t.Dir, "/")
	name := id + ".local" + t.Ext
	if dir == "" {
		return name
	}
	return dir + "/" + name
}
