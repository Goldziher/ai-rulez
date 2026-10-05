package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/okf"
)

func TestOKFCodesMatchPackage(t *testing.T) {
	assert.Equal(t, okf.CodeIndexMismatch, CodeOKFIndexMismatch)
	assert.Equal(t, okf.CodeTypeInvalid, CodeOKFTypeInvalid)
	assert.Equal(t, okf.CodeLinkBroken, CodeOKFLinkBroken)
	assert.Equal(t, okf.CodeVersionInvalid, CodeOKFVersionInvalid)
	assert.Equal(t, okf.CodeOrphan, CodeOKFOrphan)
	assert.Equal(t, okf.CodeExportDrift, CodeOKFExportDrift)
	assert.Equal(t, okf.CodeReservedStructure, CodeOKFReservedStructure)
	assert.Equal(t, okf.CodeTitleDuplicate, CodeOKFTitleDuplicate)
	assert.Equal(t, okf.CodePathUnsafe, CodeOKFPathUnsafe)
	assert.Equal(t, okf.CodeLossyMapping, CodeOKFLossyMapping)
}
