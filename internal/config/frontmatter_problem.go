package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/frontmatter"
)

// CodeFrontmatterMalformed is the lint code (AR306) a malformed frontmatter block
// is reported under by `validate`.
const CodeFrontmatterMalformed = "AR306"

// frontmatterFence is the line that opens and closes a frontmatter block.
const frontmatterFence = "---"

// FrontmatterProblem says why the frontmatter of the file at path cannot be used,
// as "path:line: reason". The loader only records that a block was unusable; this
// reads the file again to find the line and the parser's own error. It never
// fails: without a readable reason it still names the file.
func FrontmatterProblem(path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // a content file the loader already read
	if err != nil {
		return path + ":1: frontmatter cannot be parsed"
	}
	block := frontmatter.Split(data)
	if !block.Closed {
		return path + ":1: the frontmatter opened with " + frontmatterFence + " never closes with " + frontmatterFence
	}
	var out map[string]any
	err = yaml.Unmarshal([]byte(block.Raw), &out)
	if err == nil {
		return path + ":1: frontmatter is not a YAML mapping"
	}
	reason := strings.TrimPrefix(err.Error(), "yaml: ")
	line := 1
	// yaml.v3 reports "line N: ..." relative to the block; the file has the --- line above it.
	var n int
	if _, scanErr := fmt.Sscanf(reason, "line %d:", &n); scanErr == nil {
		line = n + 1
	}
	return fmt.Sprintf("%s:%d: yaml: %s", path, line, reason)
}
