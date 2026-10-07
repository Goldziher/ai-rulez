package commands

import (
	"errors"
	"strings"
	"testing"

	"github.com/samber/oops"
)

func TestFormatErrorShowsHint(t *testing.T) {
	err := oops.Hint("pass only one").Errorf("role and profile conflict")
	got := FormatError(err)
	if !strings.Contains(got, "role and profile conflict") || !strings.Contains(got, "Hint: pass only one") {
		t.Fatalf("hint missing: %q", got)
	}
}

func TestFormatErrorPlain(t *testing.T) {
	if got := FormatError(errors.New("boom")); got != "boom" {
		t.Fatalf("got %q", got)
	}
}
