package usage

import (
	"errors"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// An oversized hook event is refused as too large, not cut and mis-parsed.
func TestRecordRefusesAnOversizedEvent(t *testing.T) {
	huge := `{"pad":"` + strings.Repeat("x", 5<<20) + `"}`

	_, err := Record(strings.NewReader(huge), RecordOptions{})

	if !errors.Is(err, safefs.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}
