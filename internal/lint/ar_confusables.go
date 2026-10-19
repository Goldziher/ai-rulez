// Derived from Unicode confusables.txt (Unicode-3.0, see THIRD_PARTY_NOTICES.md); U+0196, U+0399, U+0406 map to I.

package lint

import "sync"

// confusableLetters maps the Greek, Cyrillic and Latin-extended characters that
// look like an ASCII letter to that letter. Characters that NFKC already folds
// (full-width forms, mathematical alphanumerics, ligatures) are not listed. The
// table is a hand-curated set of look-alikes in the style of Unicode UTS #39
// confusables.txt whose target is one ASCII letter, not a generated copy of that
// file; lookalikeExtras adds Armenian letters and a few small capitals, and
// accents on Latin letters are removed separately. A capital that looks like an I
// maps to I, not to l.
var confusableLetters = sync.OnceValue(func() map[rune]rune {
	return map[rune]rune{
		'\u00fe': 'p', // latin small letter thorn
		'\u0131': 'i', // latin small letter dotless i
		'\u0192': 'f', // latin small letter f with hook
		'\u0196': 'I', // latin capital letter iota
		'\u01c0': 'l', // latin letter dental click
		'\u0251': 'a', // latin small letter alpha
		'\u0261': 'g', // latin small letter script g
		'\u0263': 'y', // latin small letter gamma
		'\u0269': 'i', // latin small letter iota
		'\u026a': 'i', // latin letter small capital i
		'\u028b': 'u', // latin small letter v with hook
		'\u028f': 'y', // latin letter small capital y
		'\u037f': 'J', // greek capital letter yot
		'\u0391': 'A', // greek capital letter alpha
		'\u0392': 'B', // greek capital letter beta
		'\u0395': 'E', // greek capital letter epsilon
		'\u0396': 'Z', // greek capital letter zeta
		'\u0397': 'H', // greek capital letter eta
		'\u0399': 'I', // greek capital letter iota
		'\u039a': 'K', // greek capital letter kappa
		'\u039c': 'M', // greek capital letter mu
		'\u039d': 'N', // greek capital letter nu
		'\u039f': 'O', // greek capital letter omicron
		'\u03a1': 'P', // greek capital letter rho
		'\u03a4': 'T', // greek capital letter tau
		'\u03a5': 'Y', // greek capital letter upsilon
		'\u03a7': 'X', // greek capital letter chi
		'\u03b1': 'a', // greek small letter alpha
		'\u03b3': 'y', // greek small letter gamma
		'\u03b9': 'i', // greek small letter iota
		'\u03bd': 'v', // greek small letter nu
		'\u03bf': 'o', // greek small letter omicron
		'\u03c1': 'p', // greek small letter rho
		'\u03c5': 'u', // greek small letter upsilon
		'\u03f2': 'c', // greek lunate sigma symbol
		'\u03f3': 'j', // greek letter yot
		'\u03f9': 'C', // greek capital lunate sigma symbol
		'\u03fa': 'M', // greek capital letter san
		'\u0405': 'S', // cyrillic capital letter dze
		'\u0406': 'I', // cyrillic capital letter byelorussian-ukrainian i
		'\u0408': 'J', // cyrillic capital letter je
		'\u0410': 'A', // cyrillic capital letter a
		'\u0412': 'B', // cyrillic capital letter ve
		'\u0415': 'E', // cyrillic capital letter ie
		'\u041a': 'K', // cyrillic capital letter ka
		'\u041c': 'M', // cyrillic capital letter em
		'\u041d': 'H', // cyrillic capital letter en
		'\u041e': 'O', // cyrillic capital letter o
		'\u0420': 'P', // cyrillic capital letter er
		'\u0421': 'C', // cyrillic capital letter es
		'\u0422': 'T', // cyrillic capital letter te
		'\u0423': 'Y', // cyrillic capital letter u
		'\u0425': 'X', // cyrillic capital letter ha
		'\u0430': 'a', // cyrillic small letter a
		'\u0435': 'e', // cyrillic small letter ie
		'\u043e': 'o', // cyrillic small letter o
		'\u0440': 'p', // cyrillic small letter er
		'\u0441': 'c', // cyrillic small letter es
		'\u0443': 'y', // cyrillic small letter u
		'\u0445': 'x', // cyrillic small letter ha
		'\u0455': 's', // cyrillic small letter dze
		'\u0456': 'i', // cyrillic small letter byelorussian-ukrainian i
		'\u0458': 'j', // cyrillic small letter je
		'\u0474': 'V', // cyrillic capital letter izhitsa
		'\u0475': 'v', // cyrillic small letter izhitsa
		'\u04ae': 'Y', // cyrillic capital letter straight u
		'\u04af': 'y', // cyrillic small letter straight u
		'\u04bb': 'h', // cyrillic small letter shha
		'\u04bd': 'e', // cyrillic small letter abkhasian che
		'\u04c0': 'l', // cyrillic letter palochka
		'\u04cf': 'l', // cyrillic small letter palochka
		'\u0501': 'd', // cyrillic small letter komi de
		'\u050c': 'G', // cyrillic capital letter komi sje
		'\u051b': 'q', // cyrillic small letter qa
		'\u051c': 'W', // cyrillic capital letter we
		'\u051d': 'w', // cyrillic small letter we
		'\u1d04': 'c', // latin letter small capital c
		'\u1d0f': 'o', // latin letter small capital o
		'\u1d1c': 'u', // latin letter small capital u
		'\u1d20': 'v', // latin letter small capital v
		'\u1d21': 'w', // latin letter small capital w
		'\u1d22': 'z', // latin letter small capital z
		'\u1e9d': 'f', // latin small letter long s with high stroke
		'\ua731': 's', // latin letter small capital s
	}
})
