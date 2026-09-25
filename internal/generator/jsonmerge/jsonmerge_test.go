package jsonmerge_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handAuthoredSettings mirrors a real, version-controlled settings document:
// every top-level key here belongs to the human except `mcpServers`.
const handAuthoredSettings = `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(git status:*)"
    ]
  },
  "skillOverrides": {
    "init": "off"
  },
  "mcpServers": {
    "stale-hand-authored": {
      "command": "this-entry-is-replaced"
    }
  }
}
`

// ownedMCPServers is the owned key a settings-style document carries.
func ownedMCPServers() []jsonmerge.OwnedKey {
	return []jsonmerge.OwnedKey{
		{Name: "mcpServers", Value: map[string]any{
			"generated": map[string]any{
				"command": "npx",
				"args":    []string{"-y", "generated-server"},
			},
		}},
	}
}

type rawMember struct {
	key string
	raw string
}

// jsonMembers decodes the top-level members of a JSON object in source order.
// Independent of the production implementation on purpose: the test must be able
// to fail if the merge's own ordering logic regresses.
func jsonMembers(t *testing.T, doc string) []rawMember {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader([]byte(doc)))
	open, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), open)

	var members []rawMember
	for decoder.More() {
		keyToken, err := decoder.Token()
		require.NoError(t, err)
		key, ok := keyToken.(string)
		require.True(t, ok, "object key must be a string")
		var raw json.RawMessage
		require.NoError(t, decoder.Decode(&raw))
		members = append(members, rawMember{key: key, raw: string(raw)})
	}
	return members
}

func jsonMemberOrder(t *testing.T, doc string) []string {
	t.Helper()
	keys := make([]string, 0, 8)
	for _, member := range jsonMembers(t, doc) {
		keys = append(keys, member.key)
	}
	return keys
}

func rawValue(t *testing.T, doc, key string) string {
	t.Helper()
	for _, member := range jsonMembers(t, doc) {
		if member.key == key {
			return member.raw
		}
	}
	t.Fatalf("key %q missing from document:\n%s", key, doc)
	return ""
}

// writeFixture materializes an existing on-disk document and returns its path,
// so a test reads as "given this file".
func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestApply_PreservesHandAuthoredKeys is the unit-level regression test for
// GitHub issue #185: rendering a settings document used to replace it wholesale
// with {"mcpServers": ...}, destroying every hand-authored key.
func TestApply_PreservesHandAuthoredKeys(t *testing.T) {
	t.Parallel()

	path := writeFixture(t, "settings.json", handAuthoredSettings)

	result, err := jsonmerge.Apply(path, ownedMCPServers())
	require.NoError(t, err)

	for _, key := range []string{"$schema", "permissions", "skillOverrides"} {
		assert.Equal(t,
			rawValue(t, handAuthoredSettings, key),
			rawValue(t, result.Body, key),
			"user-owned key %q must survive byte-for-byte", key)
	}
	assert.True(t, result.PartiallyOwned,
		"a document carrying user-owned keys is shared, not a generated artifact")

	var parsed struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Body), &parsed))
	assert.Equal(t, "npx", parsed.MCPServers["generated"].Command)
	assert.Equal(t, []string{"-y", "generated-server"}, parsed.MCPServers["generated"].Args)
	assert.NotContains(t, parsed.MCPServers, "stale-hand-authored",
		"ai-rulez owns the mcpServers key outright")
}

// TestApply_PreservesTopLevelKeyOrder pins the ordering guarantee: merging
// rewrites one member in place rather than re-serializing the document from a Go
// map (which would alphabetise every key).
func TestApply_PreservesTopLevelKeyOrder(t *testing.T) {
	t.Parallel()

	path := writeFixture(t, "settings.json", handAuthoredSettings)

	result, err := jsonmerge.Apply(path, ownedMCPServers())
	require.NoError(t, err)
	assert.Equal(t,
		jsonMemberOrder(t, handAuthoredSettings),
		jsonMemberOrder(t, result.Body),
		"top-level key order must be preserved")
}

// TestApply_AppendsOwnedKeyWhenAbsent covers the common case of a settings file
// that has never carried the owned key: it is appended last and the existing
// members keep their order and bytes.
func TestApply_AppendsOwnedKeyWhenAbsent(t *testing.T) {
	t.Parallel()

	const existing = `{
  "model": "opus",
  "skillOverrides": {
    "init": "off"
  }
}
`
	path := writeFixture(t, "settings.json", existing)

	result, err := jsonmerge.Apply(path, ownedMCPServers())
	require.NoError(t, err)
	assert.Equal(t, []string{"model", "skillOverrides", "mcpServers"}, jsonMemberOrder(t, result.Body))
	assert.Equal(t, `"opus"`, rawValue(t, result.Body, "model"))
	assert.Contains(t, result.Body, "generated")
	assert.True(t, result.PartiallyOwned)
}

// TestApply_ReplacesEveryOwnedKey covers a document with more than one owned key
// (the Amp settings shape), including one present and one absent.
func TestApply_ReplacesEveryOwnedKey(t *testing.T) {
	t.Parallel()

	const existing = `{
  "amp.anthropic.effort": "low",
  "amp.mcpServers": {
    "local": {
      "command": "amp-mcp"
    }
  }
}
`
	path := writeFixture(t, "settings.json", existing)

	result, err := jsonmerge.Apply(path, []jsonmerge.OwnedKey{
		{Name: "amp.anthropic.effort", Value: "high"},
		{Name: "appendedOwnedKey", Value: 42},
	})
	require.NoError(t, err)

	assert.Equal(t, `"high"`, rawValue(t, result.Body, "amp.anthropic.effort"))
	assert.Equal(t, "42", rawValue(t, result.Body, "appendedOwnedKey"))
	assert.Equal(t, rawValue(t, existing, "amp.mcpServers"), rawValue(t, result.Body, "amp.mcpServers"),
		"unowned keys must survive byte-for-byte")
	assert.Equal(t,
		[]string{"amp.anthropic.effort", "amp.mcpServers", "appendedOwnedKey"},
		jsonMemberOrder(t, result.Body))
}

// TestApply_RefusesUnparseableExistingFile documents the deliberate fail-closed
// choice: encoding/json cannot represent comments or trailing commas, so a JSONC
// settings file must abort generation rather than be silently rewritten without
// its comments.
func TestApply_RefusesUnparseableExistingFile(t *testing.T) {
	t.Parallel()

	const jsonc = `{
  // ai-rulez must not eat this comment
  "model": "opus"
}
`
	path := writeFixture(t, "settings.json", jsonc)

	_, err := jsonmerge.Apply(path, ownedMCPServers())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse existing JSON settings document")

	// The actionable detail rides on the oops hint and context, which is what the
	// CLI prints (cmd/commands/generate.go fmtError).
	oopsErr, ok := oops.AsOops(err)
	require.True(t, ok, "error must carry oops context")
	assert.Contains(t, oopsErr.Hint(), path)
	assert.Equal(t, path, oopsErr.Context()["path"])

	onDisk, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, jsonc, string(onDisk), "the unparseable file must be left untouched")
}

// TestApply_RefusesNonObjectAndTrailingContent covers the other two shapes a
// merge cannot represent: a non-object root, and content after the root object
// that a rewrite would drop.
func TestApply_RefusesNonObjectAndTrailingContent(t *testing.T) {
	t.Parallel()

	for name, existing := range map[string]string{
		"array root":       "[1, 2, 3]\n",
		"scalar root":      "42\n",
		"trailing content": "{\"model\": \"opus\"}\n{\"model\": \"sonnet\"}\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "settings.json", existing)

			_, err := jsonmerge.Apply(path, ownedMCPServers())
			require.Error(t, err)

			onDisk, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, existing, string(onDisk))
		})
	}
}

// TestApply_CreatesDocumentWhenNothingToMergeInto keeps the greenfield output
// byte-identical to the pre-merge renderers (plain map + MarshalIndent) for all
// three "nothing there" cases, so first-run output is unchanged.
func TestApply_CreatesDocumentWhenNothingToMergeInto(t *testing.T) {
	t.Parallel()

	const want = `{
  "mcpServers": {
    "generated": {
      "args": [
        "-y",
        "generated-server"
      ],
      "command": "npx"
    }
  }
}
`
	t.Run("no path", func(t *testing.T) {
		t.Parallel()

		result, err := jsonmerge.Apply("", ownedMCPServers())
		require.NoError(t, err)
		assert.Equal(t, want, result.Body)
		assert.False(t, result.PartiallyOwned, "a freshly created document is wholly generated")
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "settings.json")
		result, err := jsonmerge.Apply(path, ownedMCPServers())
		require.NoError(t, err)
		assert.Equal(t, want, result.Body)
		assert.False(t, result.PartiallyOwned)
	})

	t.Run("all whitespace file", func(t *testing.T) {
		t.Parallel()

		path := writeFixture(t, "settings.json", "\n\t \n")
		result, err := jsonmerge.Apply(path, ownedMCPServers())
		require.NoError(t, err)
		assert.Equal(t, want, result.Body)
		assert.False(t, result.PartiallyOwned)
	})
}

// TestApply_OwnedOnlyDocumentIsNotPartiallyOwned pins the flag that decides
// whether the pipeline may gitignore and delete the file: a document holding
// nothing but the owned key is a generated artifact even though it already
// existed.
func TestApply_OwnedOnlyDocumentIsNotPartiallyOwned(t *testing.T) {
	t.Parallel()

	const existing = `{
  "mcpServers": {
    "previous-run": {
      "command": "old"
    }
  }
}
`
	path := writeFixture(t, ".mcp.json", existing)

	result, err := jsonmerge.Apply(path, ownedMCPServers())
	require.NoError(t, err)
	assert.False(t, result.PartiallyOwned,
		"only the owned key is present, so the file is ai-rulez's to ignore and delete")
	assert.Equal(t, []string{"mcpServers"}, jsonMemberOrder(t, result.Body))
}

// TestApply_PreservesExistingIndent checks the merge adapts to the existing
// file's top-level indentation instead of forcing two spaces onto a document
// indented differently.
func TestApply_PreservesExistingIndent(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		existing string
		wantKey  string
		wantOwn  string
	}{
		"four spaces": {
			existing: "{\n    \"model\": \"opus\",\n    \"env\": {\n        \"A\": \"b\"\n    }\n}\n",
			wantKey:  "\n    \"model\": \"opus\",",
			wantOwn:  "\n    \"mcpServers\": {\n        \"generated\"",
		},
		"tabs": {
			existing: "{\n\t\"model\": \"opus\"\n}\n",
			wantKey:  "\n\t\"model\": \"opus\",",
			wantOwn:  "\n\t\"mcpServers\": {\n\t\t\"generated\"",
		},
		"single line falls back to two spaces": {
			existing: "{\"model\": \"opus\"}\n",
			wantKey:  "\n  \"model\": \"opus\",",
			wantOwn:  "\n  \"mcpServers\": {\n    \"generated\"",
		},
		// Reading the line right after the opening brace measures a width of
		// zero on these, which silently re-indents the whole top level.
		"blank line after the opening brace": {
			existing: "{\n\n    \"model\": \"opus\"\n}\n",
			wantKey:  "\n    \"model\": \"opus\",",
			wantOwn:  "\n    \"mcpServers\": {\n        \"generated\"",
		},
		"leading blank lines before the opening brace": {
			existing: "\n\n{\n\t\"model\": \"opus\"\n}\n",
			wantKey:  "\n\t\"model\": \"opus\",",
			wantOwn:  "\n\t\"mcpServers\": {\n\t\t\"generated\"",
		},
		// The first indented line belongs to a nested object, not the top
		// level. Measuring it re-indents every hand-authored member.
		"first key opens an object on the brace line": {
			existing: "{\"permissions\": {\n    \"allow\": []\n  },\n  \"model\": \"opus\"\n}\n",
			wantKey:  "\n  \"model\": \"opus\",",
			wantOwn:  "\n  \"mcpServers\": {\n    \"generated\"",
		},
		// A nested array is the same trap: its entries are indented deeper
		// than the top-level members that follow.
		"first key opens an array on the brace line": {
			existing: "{\"hooks\": [\n      \"a\"\n  ],\n  \"model\": \"opus\"\n}\n",
			wantKey:  "\n  \"model\": \"opus\",",
			wantOwn:  "\n  \"mcpServers\": {\n    \"generated\"",
		},
		// A brace inside a string must not be read as nesting.
		"a brace inside a string value": {
			existing: "{\n    \"note\": \"a { brace\",\n    \"model\": \"opus\"\n}\n",
			wantKey:  "\n    \"model\": \"opus\",",
			wantOwn:  "\n    \"mcpServers\": {\n        \"generated\"",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "settings.json", testCase.existing)
			result, err := jsonmerge.Apply(path, ownedMCPServers())
			require.NoError(t, err)
			assert.Contains(t, result.Body, testCase.wantKey)
			assert.Contains(t, result.Body, testCase.wantOwn)
		})
	}
}

// TestApply_PreservesLineEndings covers the CRLF counterpart of indent
// preservation. Untouched members are re-emitted verbatim, so their internal
// CRLFs survive; writing LF around them produced a file with both, which reads
// as a whole-file change to git and to the editor that wrote it.
func TestApply_PreservesLineEndings(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		existing string
		want     string
	}{
		"crlf document stays crlf": {
			existing: "{\r\n  \"permissions\": {\r\n    \"allow\": []\r\n  },\r\n  \"model\": \"opus\"\r\n}\r\n",
			want:     "{\r\n  \"permissions\": {\r\n    \"allow\": []\r\n  },\r\n  \"model\": \"opus\",\r\n  \"mcpServers\": {\r\n",
		},
		"lf document stays lf": {
			existing: "{\n  \"model\": \"opus\"\n}\n",
			want:     "{\n  \"model\": \"opus\",\n  \"mcpServers\": {\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeFixture(t, "settings.json", testCase.existing)

			result, err := jsonmerge.Apply(path, ownedMCPServers())

			require.NoError(t, err)
			assert.Contains(t, result.Body, testCase.want)
			if !strings.Contains(testCase.existing, "\r\n") {
				assert.NotContains(t, result.Body, "\r", "an LF document must not gain carriage returns")
			}
		})
	}
}
