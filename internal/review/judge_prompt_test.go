package review

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The reply instruction states the same quote limits the evidence check enforces,
// so a quote the check would drop is one the model was told not to send.
func TestReplyInstructionStatesTheQuoteLimits(t *testing.T) {
	// Arrange
	want := []string{
		"character for character",
		"contiguous",
		"at least " + strconv.Itoa(minQuoteRunes) + " characters",
		"at most " + strconv.Itoa(maxQuoteWords) + " words",
	}

	// Act / Assert
	for _, w := range want {
		assert.Contains(t, replyInstruction, w)
	}
}
