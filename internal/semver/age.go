package semver

import (
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"
)

// MaxAge bounds a min_release_age: ten years is already "never adopt".
const MaxAge = 3650 * 24 * time.Hour

// ParseAge parses a minimum release age: a whole number with the unit h (hours),
// d (days) or w (weeks) ("12h", "7d", "2w"), or a Go duration of whole hours or
// more ("168h"). "0" and "" mean no minimum. Negative values and ages above
// MaxAge are refused.
func ParseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	unit := map[byte]time.Duration{'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour}[s[len(s)-1]]
	if unit == 0 {
		return 0, oops.Errorf("%q is not an age: use a whole number with h, d or w (for example 7d)", s)
	}
	n, err := strconv.ParseUint(s[:len(s)-1], 10, 32)
	if err != nil {
		return 0, oops.Errorf("%q is not an age: use a whole number with h, d or w (for example 7d)", s)
	}
	d := time.Duration(n) * unit
	if d > MaxAge {
		return 0, oops.Errorf("%q is longer than the %d day limit", s, int(MaxAge/(24*time.Hour)))
	}
	return d, nil
}
