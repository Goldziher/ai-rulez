package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	_ "github.com/Goldziher/ai-rulez/v5/internal/generator" // registers every preset name
)

// Regenerate the preset enums after adding a preset:
//
//	UPDATE_SCHEMA=1 go test ./schema -run TestPresetEnums
var presetEnumFiles = []string{"ai-rules.schema.json", "ai-rules-local.schema.json"}

// enumArray matches a JSON `"enum": [ "a", "b" ]` array of strings, capturing the
// indentation of the key so a rewrite keeps the file's formatting.
var enumArray = regexp.MustCompile(`(?m)^([ \t]*)"enum": \[((?:\s*"[^"]*",?)+)\s*\]`)

var enumItem = regexp.MustCompile(`"([^"]*)"`)

func enumItems(raw string) []string {
	var items []string
	for _, m := range enumItem.FindAllStringSubmatch(raw, -1) {
		items = append(items, m[1])
	}
	return items
}

// isPresetEnum identifies an enum listing preset names: it names both the claude
// and mcp presets (the plugin runtime enums name claude but not mcp).
func isPresetEnum(items []string) bool {
	return slices.Contains(items, "claude") && slices.Contains(items, "mcp")
}

// syncedPresetEnum keeps the existing order of items still valid, drops removed
// presets and appends new ones in sorted order, so a regeneration diff is minimal.
func syncedPresetEnum(items, want []string) []string {
	out := make([]string, 0, len(want))
	for _, item := range items {
		if slices.Contains(want, item) {
			out = append(out, item)
		}
	}
	for _, name := range want {
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

func renderEnum(indent string, items []string) string {
	body := indent + `"enum": [` + "\n"
	for i, item := range items {
		quoted, _ := json.Marshal(item)
		body += indent + "  " + string(quoted)
		if i < len(items)-1 {
			body += ","
		}
		body += "\n"
	}
	return body + indent + "]"
}

func TestPresetEnums_MatchRegisteredPresets(t *testing.T) {
	want := config.AllPresetNames()
	update := os.Getenv("UPDATE_SCHEMA") != ""

	for _, file := range presetEnumFiles {
		t.Run(file, func(t *testing.T) {
			// Arrange
			path := filepath.Join(".", file)
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			// Act
			found := 0
			updated := enumArray.ReplaceAllStringFunc(string(data), func(match string) string {
				sub := enumArray.FindStringSubmatch(match)
				items := enumItems(sub[2])
				if !isPresetEnum(items) {
					return match
				}
				found++
				return renderEnum(sub[1], syncedPresetEnum(items, want))
			})

			// Assert
			require.Positive(t, found, "no preset enum found in %s", file)
			if update {
				require.NoError(t, os.WriteFile(path, []byte(updated), 0o644)) //nolint:gosec // schema is public
				return
			}
			for _, m := range enumArray.FindAllStringSubmatch(string(data), -1) {
				items := enumItems(m[2])
				if !isPresetEnum(items) {
					continue
				}
				got := append([]string(nil), items...)
				sort.Strings(got)
				assert.Equal(t, want, got,
					"%s preset enum is stale; run UPDATE_SCHEMA=1 go test ./schema -run TestPresetEnums", file)
			}
		})
	}
}
