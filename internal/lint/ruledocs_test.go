package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const strictDoc = "../../docs/strict-validation.md"

func TestEveryRuleHasDocs(t *testing.T) {
	for _, r := range Rules() {
		e, ok := Explain(r.Code)
		require.True(t, ok, r.Code)
		assert.NotEmpty(t, e.Why, "%s needs ruleDocs.Why", r.Code)
		assert.NotEmpty(t, e.Bad, "%s needs ruleDocs.Bad", r.Code)
		assert.NotEmpty(t, e.Good, "%s needs ruleDocs.Good", r.Code)
		assert.NotEmpty(t, e.Anchor, r.Code)
		assert.True(t, strings.HasSuffix(e.DocsURL, "#"+e.Anchor), r.Code)
	}
	for code := range ruleTables().docs {
		_, ok := lookupRule(code)
		assert.True(t, ok, "ruleDocs has an entry for unregistered code %s", code)
	}
}

func TestExplainResolvesCodeAndName(t *testing.T) {
	byCode, ok := Explain("ar001")
	require.True(t, ok)
	byName, ok := Explain("secret-detected")
	require.True(t, ok)
	assert.Equal(t, byCode, byName)
	assert.Equal(t, "ar001-secret-detected", byCode.Anchor)
	_, ok = Explain("AR999")
	assert.False(t, ok)
}

func TestEveryRuleAnchorIsAHeadingInTheDocs(t *testing.T) {
	raw, err := os.ReadFile(filepath.FromSlash(strictDoc))
	require.NoError(t, err)
	slugs := headingSlugs(string(raw))
	for _, r := range Rules() {
		_, ok := slugs[anchorFor(r)]
		assert.True(t, ok, "docs/strict-validation.md has no heading for %s (run UPDATE_DOCS=1 go test ./internal/lint -run TestRuleReferenceDoc)", r.Code)
	}
}

func TestRuleReferenceDoc(t *testing.T) {
	path := filepath.FromSlash(strictDoc)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	doc := string(raw)
	begin := strings.Index(doc, "<!-- rules:begin")
	end := strings.Index(doc, ruleRefEnd)
	require.True(t, begin >= 0 && end > begin, "docs/strict-validation.md lacks the rules:begin/rules:end markers")
	want := doc[:begin] + RuleReferenceMarkdown() + strings.TrimPrefix(doc[end+len(ruleRefEnd):], "\n")
	if os.Getenv("UPDATE_DOCS") != "" {
		require.NoError(t, os.WriteFile(path, []byte(want), 0o644)) //nolint:gosec // docs file
		return
	}
	assert.Equal(t, want, doc, "rule reference is stale: UPDATE_DOCS=1 go test ./internal/lint -run TestRuleReferenceDoc")
}
