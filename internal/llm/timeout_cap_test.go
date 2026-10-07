package llm

import (
	"math"
	"testing"
	"time"
)

func TestHugeTimeoutIsCapped(t *testing.T) {
	if len((Config{TimeoutSeconds: math.MaxInt32}).Validate()) == 0 {
		t.Error("Validate accepted a huge timeout")
	}
	got := (Config{TimeoutSeconds: math.MaxInt}).Timeout()
	if got <= 0 || got > MaxTimeoutSeconds*time.Second {
		t.Errorf("Timeout overflowed: %v", got)
	}
}
