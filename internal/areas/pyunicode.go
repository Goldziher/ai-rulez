package areas

import (
	"strings"
	"unicode"
)

// The matcher reproduces the character semantics of Python's re module with re.IGNORECASE on str patterns
// (CPython 3.11 and later, Lib/re/_compiler.py and Modules/_sre), so an area map classifies text the same way
// wherever the format is read with those regular expressions. RE2 has no lookbehind, so the matcher is a
// hand-written scanner over code points; these predicates are its character classes. A differential test holds
// them to vectors recorded under CPython.

// pyLower is the simple lowercase mapping _sre uses for case-insensitive matching.
func pyLower(r rune) rune { return unicode.ToLower(r) }

// pyIsCased reports whether case-insensitive matching treats r as cased (_sre.unicode_iscased): its lowercase or
// uppercase differs from it. CPython takes the first character of a full uppercase mapping, so the characters
// whose only uppercase is a SpecialCasing sequence (U+00DF to "SS") count as cased too.
func pyIsCased(r rune) bool {
	return unicode.ToLower(r) != r || unicode.ToUpper(r) != r || hasSpecialUpper(r)
}

// hasSpecialUpper reports whether r has an unconditional SpecialCasing uppercase but no simple one.
func hasSpecialUpper(r rune) bool {
	switch r {
	case 0xdf, 0x149, 0x1f0, 0x390, 0x3b0, 0x587, 0x1e96, 0x1e97, 0x1e98, 0x1e99, 0x1e9a,
		0x1f50, 0x1f52, 0x1f54, 0x1f56, 0x1fb2, 0x1fb4, 0x1fb6, 0x1fb7, 0x1fc2, 0x1fc4, 0x1fc6, 0x1fc7,
		0x1fd2, 0x1fd3, 0x1fd6, 0x1fd7, 0x1fe2, 0x1fe3, 0x1fe4, 0x1fe6, 0x1fe7, 0x1ff2, 0x1ff4, 0x1ff6, 0x1ff7,
		0xfb00, 0xfb01, 0xfb02, 0xfb03, 0xfb04, 0xfb05, 0xfb06, 0xfb13, 0xfb14, 0xfb15, 0xfb16, 0xfb17:
		return true
	}
	return false
}

// pyExtraCases lists the lowercase code points that re.IGNORECASE also treats as equal to the lowercase r
// beyond the simple mapping (re._casefix._EXTRA_CASES, the same in CPython 3.11 and later).
func pyExtraCases(r rune) []rune { //nolint:gocyclo // a flat copy of a CPython table; a package-level map would be mutable state
	switch r {
	case 0x69:
		return []rune{0x131}
	case 0x73:
		return []rune{0x17f}
	case 0xb5:
		return []rune{0x3bc}
	case 0x131:
		return []rune{0x69}
	case 0x17f:
		return []rune{0x73}
	case 0x345:
		return []rune{0x3b9, 0x1fbe}
	case 0x390:
		return []rune{0x1fd3}
	case 0x3b0:
		return []rune{0x1fe3}
	case 0x3b2:
		return []rune{0x3d0}
	case 0x3b5:
		return []rune{0x3f5}
	case 0x3b8:
		return []rune{0x3d1}
	case 0x3b9:
		return []rune{0x345, 0x1fbe}
	case 0x3ba:
		return []rune{0x3f0}
	case 0x3bc:
		return []rune{0xb5}
	case 0x3c0:
		return []rune{0x3d6}
	case 0x3c1:
		return []rune{0x3f1}
	case 0x3c2:
		return []rune{0x3c3}
	case 0x3c3:
		return []rune{0x3c2}
	case 0x3c6:
		return []rune{0x3d5}
	case 0x3d0:
		return []rune{0x3b2}
	case 0x3d1:
		return []rune{0x3b8}
	case 0x3d5:
		return []rune{0x3c6}
	case 0x3d6:
		return []rune{0x3c0}
	case 0x3f0:
		return []rune{0x3ba}
	case 0x3f1:
		return []rune{0x3c1}
	case 0x3f5:
		return []rune{0x3b5}
	case 0x432:
		return []rune{0x1c80}
	case 0x434:
		return []rune{0x1c81}
	case 0x43e:
		return []rune{0x1c82}
	case 0x441:
		return []rune{0x1c83}
	case 0x442:
		return []rune{0x1c84, 0x1c85}
	case 0x44a:
		return []rune{0x1c86}
	case 0x463:
		return []rune{0x1c87}
	case 0x1c80:
		return []rune{0x432}
	case 0x1c81:
		return []rune{0x434}
	case 0x1c82:
		return []rune{0x43e}
	case 0x1c83:
		return []rune{0x441}
	case 0x1c84:
		return []rune{0x442, 0x1c85}
	case 0x1c85:
		return []rune{0x442, 0x1c84}
	case 0x1c86:
		return []rune{0x44a}
	case 0x1c87:
		return []rune{0x463}
	case 0x1c88:
		return []rune{0xa64b}
	case 0x1e61:
		return []rune{0x1e9b}
	case 0x1e9b:
		return []rune{0x1e61}
	case 0x1fbe:
		return []rune{0x345, 0x3b9}
	case 0x1fd3:
		return []rune{0x390}
	case 0x1fe3:
		return []rune{0x3b0}
	case 0xa64b:
		return []rune{0x1c88}
	case 0xfb05:
		return []rune{0xfb06}
	case 0xfb06:
		return []rune{0xfb05}
	}
	return nil
}

// pyLiteralEqual reports whether the text code point c matches the pattern code point t under re.IGNORECASE:
// an uncased t matches only itself, a cased one every c whose lowercase is t's or one of its extra cases.
func pyLiteralEqual(t, c rune) bool {
	if !pyIsCased(t) {
		return c == t
	}
	lo, cl := pyLower(t), pyLower(c)
	if cl == lo {
		return true
	}
	for _, extra := range pyExtraCases(lo) {
		if cl == extra {
			return true
		}
	}
	return false
}

// pyFoldKeys returns the lowercase code points a text code point may have to match the pattern code point t
// (the keys of the first-character index).
func pyFoldKeys(t rune) []rune {
	if !pyIsCased(t) {
		return []rune{t}
	}
	lo := pyLower(t)
	return append([]rune{lo}, pyExtraCases(lo)...)
}

// pyIsAlnumFold reports whether r is in [A-Za-z0-9] under re.IGNORECASE: ASCII letters and digits plus the
// code points whose lowercase is one (U+0130, U+212A) or an extra case of one (U+0131, U+017F).
func pyIsAlnumFold(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r < 0x80:
		lo := r | 0x20
		return lo >= 'a' && lo <= 'z'
	}
	switch pyLower(r) {
	case 'i', 'k', 0x131, 0x17f:
		return true
	}
	return false
}

// pyIsWord reports whether r is in \w of a Python str pattern: "_" or a character str.isalnum() accepts.
func pyIsWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// pyIsSpace reports whether r is in \s of a Python str pattern (str.isspace()): Go's white space plus the
// information separators U+001C to U+001F.
func pyIsSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// pyIsCasedLetter is _PyUnicode_IsCased, the "cased" property str.lower() consults for a final sigma.
func pyIsCasedLetter(r rune) bool {
	return unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

// pyIsCaseIgnorable is _PyUnicode_IsCaseIgnorable: Mn, Me, Cf, Lm, Sk, or Word_Break MidLetter, MidNumLet or
// Single_Quote.
func pyIsCaseIgnorable(r rune) bool {
	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk) {
		return true
	}
	switch r {
	case '\'', '.', ':', 0xb7, 0x387, 0x55f, 0x5f4, 0x2018, 0x2019, 0x2024, 0x2027, 0xfe13, 0xfe52, 0xfe55, 0xff07, 0xff0e, 0xff1a:
		return true
	}
	return false
}

// pyStrLower is Python's str.lower(): the simple mapping, the full mapping of U+0130 and the final-sigma rule.
func pyStrLower(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range runes {
		switch r {
		case 0x130:
			b.WriteString("i\u0307")
		case 0x3a3:
			if finalSigma(runes, i) {
				b.WriteRune(0x3c2)
			} else {
				b.WriteRune(0x3c3)
			}
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// finalSigma reports whether the capital sigma at runes[i] ends a word: a cased letter precedes it and none
// follows it, case-ignorable characters skipped on both sides.
func finalSigma(runes []rune, i int) bool {
	j := i - 1
	for j >= 0 && pyIsCaseIgnorable(runes[j]) {
		j--
	}
	if j < 0 || !pyIsCasedLetter(runes[j]) {
		return false
	}
	j = i + 1
	for j < len(runes) && pyIsCaseIgnorable(runes[j]) {
		j++
	}
	return j == len(runes) || !pyIsCasedLetter(runes[j])
}
