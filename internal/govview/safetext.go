package govview

import (
	"fmt"
	"strings"
	"unicode"
)

// SafeText makes untrusted text safe to print: control characters (other than
// tab), bidirectional controls, zero-width and tag characters are replaced by
// their \u escape, so content cannot hide text from the reviewer.
func SafeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r), r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E,
			r >= 0x2060 && r <= 0x2069, r == 0xFEFF, r >= 0xE0000 && r <= 0xE007F:
			fmt.Fprintf(&b, "\\u{%X}", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
