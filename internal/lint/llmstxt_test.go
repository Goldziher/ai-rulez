package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llmstxt"
)

func TestLLMsTxtCodesMatchPackage(t *testing.T) {
	assert.Equal(t, llmstxt.CodeTitleMissing, CodeLLMsTxtTitleMissing)
	assert.Equal(t, llmstxt.CodeSummaryMisplaced, CodeLLMsTxtSummaryMisplaced)
	assert.Equal(t, llmstxt.CodeHeadingInvalid, CodeLLMsTxtHeadingInvalid)
	assert.Equal(t, llmstxt.CodeLinkEntryInvalid, CodeLLMsTxtLinkEntryInvalid)
	assert.Equal(t, llmstxt.CodeOptionalMisplaced, CodeLLMsTxtOptionalMisplaced)
	assert.Equal(t, llmstxt.CodeSectionDuplicateOrEmpty, CodeLLMsTxtSectionEmptyOrDup)
	assert.Equal(t, llmstxt.CodeLinkTargetInvalid, CodeLLMsTxtLinkTargetInvalid)
}

func TestLLMsTxtExplainResolves(t *testing.T) {
	for _, key := range []string{"AR9P3", "llmstxt-link-entry-invalid"} {
		e, ok := Explain(key)
		require.True(t, ok, key)
		assert.Equal(t, "AR9P3", e.Code)
	}
}
