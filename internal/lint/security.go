package lint

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/secretpat"
)

// The security family (AR001...) is deterministic and offline: it reads text and
// reports patterns, it never fetches or executes anything.

type secretPattern struct {
	name string
	re   *regexp.Regexp
	// stems are literals every match contains; nil runs the pattern on every text.
	stems []string
}

// mayMatch reports whether s holds a stem of p.
func (p secretPattern) mayMatch(s string) bool {
	return p.stems == nil || containsAnyStem(s, false, p.stems)
}

// builtinSecrets are the credential shapes shared with the model layer's refuse-to-send check
// (internal/secretpat), so a key the scan reports is never sent to a model unnoticed.
var builtinSecrets = func() []secretPattern {
	out := make([]secretPattern, 0, len(secretpat.Builtin))
	for _, p := range secretpat.Builtin {
		out = append(out, secretPattern{name: p.Name, re: p.Re, stems: p.Stems})
	}
	return out
}()

// genericCredential matches `password = "..."` style assignments; the value must
// mix letters and digits so placeholders such as "your-key-here" pass.
var genericCredential = secretpat.GenericCredential

// injectionPhrases are the instruction-override phrasings, each behind the
// literals every match contains (see gatedRe).
var injectionPhrases = []gatedRe{
	newGatedRe(`(?i)\bignore\s+(?:all\s+|any\s+|the\s+)?(?:previous|prior|above|earlier|preceding)\s+(?:instructions?|prompts?|rules?|directions?|context)\b`, true, "ignore"),
	newGatedRe(`(?i)\bdisregard\s+(?:all\s+|any\s+|the\s+)?(?:previous|prior|above|earlier|system|safety)\b`, true, "disregard"),
	newGatedRe(`(?i)\bforget\s+(?:everything|all)\s+(?:you|above|previous)\b`, true, "forget"),
	newGatedRe(`(?i)\b(?:do\s+not|don'?t|never)\s+(?:tell|inform|mention|reveal|show)\s+(?:this\s+to\s+)?the\s+user\b`, true, "user"),
	newGatedRe(`(?i)\bwithout\s+(?:telling|informing|notifying|asking)\s+the\s+user\b`, true, "without"),
	newGatedRe(`(?i)\b(?:reveal|print|output|repeat)\s+(?:your\s+)?(?:system\s+prompt|hidden\s+instructions)\b`, true, "prompt", "instructions"),
	newGatedRe(`(?i)\byou\s+are\s+now\s+(?:in\s+)?(?:developer|dan|god|jailbreak)\b`, true, "now"),
	newGatedRe(`(?i)\boverride\s+(?:your\s+)?(?:safety|system|security)\s+(?:instructions?|rules?|guidelines?|prompt)\b`, true, "override"),
}

var (
	// An unclosed <!-- runs to the end of the file in CommonMark, so it hides text too.
	htmlCommentRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?s)<!--(.*?)(?:-->|\z)`) })
	imperativeRe  = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:curl|wget|eval|sudo|exfiltrate|secretly|silently|execute)\b|\brun\s*:`)
	})
	pipeToShellRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^|\n]*\|\s*(?:sudo\s+(?:-\S+\s+)*)?(?:ba|z|da|k)?sh\b`)
	})
	pipeToInterpRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^|\n]*\|\s*(?:sudo\s+)?(?:python3?|perl|ruby|node)\b`)
	})
	procSubstRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)(?:\b(?:ba|z)?sh|\bsource|\.)\s+<\(\s*(?:curl|wget)\b`)
	})
	// evalRe matches a call eval(...), or the shell builtin in command position
	// (line start, after ; & | ( { ` $( then do else) followed by an argument. A
	// word after another command (`playwright-cli eval "document.title"`) is a
	// subcommand, not the builtin.
	evalRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile("(?i)(?:^|[^\\w.'\"`])eval\\s*\\(|(?:^|[;&|({`]|\\$\\(|\\b(?:then|do|else))\\s*(?:[-*>]\\s+)*(?:\\$\\s+)?eval\\s+[\"'$`]")
	})
	evalBenignRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\beval\s+"?\$\(\s*(?:ssh-agent|pyenv|rbenv|nodenv|direnv|brew\s+shellenv|fnm|starship|zoxide|mise|rtx|asdf|opam|thefuck)\b`)
	})
	base64ExecRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)base64\s+(?:-d|-D|--decode)\b.*\|\s*(?:sudo\s+)?(?:ba|z|da)?sh\b|\bexec\s*\(\s*(?:base64\.)?b64decode`)
	})
	writeOutsideRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:>>?|\btee(?:\s+-a)?)\s*(?:~/|\$HOME/|\$\{HOME\}/|/etc/|/usr/|/opt/|/var/|/root/)`)
	})
	chmod777Re = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`\bchmod\s+(?:-R\s+)?(?:0?777|a\+rwx)\b`) })
	blobRe     = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`[A-Za-z0-9+/]{200,}={0,2}`) }) // a match is at least blobMinLen bytes
	urlRe      = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\bhttps?://([a-z0-9](?:[a-z0-9.-]*[a-z0-9])?)(?::\d+)?`)
	})
	broadBashToolRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^Bash\(\s*\*+(?::\*+)?\s*\)$`) })
)

// blobMinLen is the shortest line blobRe can match.
const blobMinLen = 200

// hiddenRunes are the code points that make text invisible or reorder it.
var hiddenRunes = map[rune]string{
	0x200B: "ZERO WIDTH SPACE", 0x200C: "ZERO WIDTH NON-JOINER", 0x200D: "ZERO WIDTH JOINER",
	0x200E: "LEFT-TO-RIGHT MARK", 0x200F: "RIGHT-TO-LEFT MARK", 0x2060: "WORD JOINER",
	0x2061: "FUNCTION APPLICATION", 0x2062: "INVISIBLE TIMES", 0x2063: "INVISIBLE SEPARATOR", 0x2064: "INVISIBLE PLUS",
	0x180E: "MONGOLIAN VOWEL SEPARATOR", 0xFEFF: "ZERO WIDTH NO-BREAK SPACE",
	0x202A: "LEFT-TO-RIGHT EMBEDDING", 0x202B: "RIGHT-TO-LEFT EMBEDDING", 0x202C: "POP DIRECTIONAL FORMATTING",
	0x202D: "LEFT-TO-RIGHT OVERRIDE", 0x202E: "RIGHT-TO-LEFT OVERRIDE",
	0x2066: "LEFT-TO-RIGHT ISOLATE", 0x2067: "RIGHT-TO-LEFT ISOLATE", 0x2068: "FIRST STRONG ISOLATE", 0x2069: "POP DIRECTIONAL ISOLATE",
	// Default-ignorable or blank-rendering characters an invisible payload can hide in.
	0x034F: "COMBINING GRAPHEME JOINER", 0x061C: "ARABIC LETTER MARK", 0x115F: "HANGUL CHOSEONG FILLER", 0x1160: "HANGUL JUNGSEONG FILLER",
	0x17B4: "KHMER VOWEL INHERENT AQ", 0x17B5: "KHMER VOWEL INHERENT AA", 0x180B: "MONGOLIAN FREE VARIATION SELECTOR ONE",
	0x180C: "MONGOLIAN FREE VARIATION SELECTOR TWO", 0x180D: "MONGOLIAN FREE VARIATION SELECTOR THREE",
	0x2028: "LINE SEPARATOR", 0x2029: "PARAGRAPH SEPARATOR", 0x2800: "BRAILLE PATTERN BLANK",
	0x3164: "HANGUL FILLER", 0xFFA0: "HALFWIDTH HANGUL FILLER",
}

// hiddenName names a hidden code point, including the ranges: the deprecated
// formatting characters U+206A to U+206F and the variation selectors.
func hiddenName(c rune) (string, bool) {
	switch {
	case c >= 0x206A && c <= 0x206F:
		return "DEPRECATED FORMAT CHARACTER", true
	case isVariationSelector(c):
		return "VARIATION SELECTOR", true
	}
	name, ok := hiddenRunes[c]
	return name, ok
}

func isVariationSelector(c rune) bool {
	return (c >= 0xFE00 && c <= 0xFE0F) || (c >= 0xE0100 && c <= 0xE01EF)
}

// variationIsLegitimate accepts the one selector that modifies the character
// before it: after a symbol or a keycap base (emoji presentation), or after a
// CJK ideograph. A selector after a letter, or one in a run, can carry a payload.
func variationIsLegitimate(runes []rune, i int) bool {
	if i == 0 || (i+1 < len(runes) && isVariationSelector(runes[i+1])) || isVariationSelector(runes[i-1]) {
		return false
	}
	prev := runes[i-1]
	switch {
	case unicode.IsSymbol(prev), unicode.Is(unicode.Han, prev):
		return true
	case prev == '#' || prev == '*' || (prev >= '0' && prev <= '9'):
		return i+1 < len(runes) && runes[i+1] == 0x20E3
	}
	return false
}

// isTagRune reports a Unicode tag character (U+E0000 to U+E007F), which can
// smuggle invisible ASCII.
func isTagRune(r rune) bool { return r >= 0xE0000 && r <= 0xE007F }

func (r *runner) security() config.LintSecurity {
	if r.lc.Security != nil {
		return *r.lc.Security
	}
	return config.LintSecurity{}
}

// securityScan applies the security rules to one text. abs is the file the
// findings point into; script marks shell-like content (resources).
func (r *runner) securityScan(abs, raw string) {
	if _, ok := r.docs[abs]; !ok {
		r.docs[abs] = parseDoc(raw)
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	var st *scanText
	if isMarkdownPath(abs) {
		st = newScanText(r, abs, raw)
	}
	for i, line := range lines {
		no := i + 1
		r.scanHidden(abs, no, line, i == 0)
		r.scanSecrets(abs, no, line)
		r.scanInjection(abs, no, line)
		r.scanShell(abs, no, line, describesRisk(st, i))
		r.scanHosts(abs, no, line)
		if len(line) >= blobMinLen && blobRe().MatchString(line) {
			r.add(CodeEncodedBlob, abs, no, "line holds a base64-like blob of 200 or more characters that a reviewer cannot read")
		}
	}
	r.scanComments(abs, raw)
	r.runTextScans(abs, raw)
}

func (r *runner) scanHidden(abs string, no int, line string, firstLine bool) {
	if found := hiddenIn(line, firstLine); len(found) > 0 {
		r.add(CodeHiddenCharacters, abs, no, "hidden character(s): %s", strings.Join(found, ", "))
	}
}

// DetectHidden reports whether text holds a zero-width, bidirectional-control
// or Unicode tag character the AR002 rule flags, and returns the first one
// found. It applies no lint setting, severity or ignore: a caller deciding what
// may leave the machine cannot let repository config switch the check off.
func DetectHidden(text string) (string, bool) {
	for i, line := range strings.Split(text, "\n") {
		if found := hiddenIn(line, i == 0); len(found) > 0 {
			return found[0], true
		}
	}
	return "", false
}

// isASCII reports whether s holds only 7-bit bytes.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// hiddenIn lists the hidden characters of one line.
func hiddenIn(line string, firstLine bool) []string {
	if isASCII(line) {
		return nil // every hidden code point is non-ASCII
	}
	runes := []rune(line)
	seen := map[rune]bool{}
	var found []string
	for i, c := range runes {
		name, hidden := hiddenName(c)
		if !hidden && !isTagRune(c) {
			continue
		}
		if c == 0xFEFF && firstLine && i == 0 {
			continue // a byte order mark
		}
		if (c == 0x200C || c == 0x200D) && joinerIsLegitimate(runes, i) {
			continue
		}
		if isVariationSelector(c) && variationIsLegitimate(runes, i) {
			continue
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		if isTagRune(c) {
			name = "TAG CHARACTER"
		}
		found = append(found, fmt.Sprintf("U+%04X %s (column %d)", c, name, i+1))
	}
	return found
}

// joinerIsLegitimate accepts a joiner between two non-ASCII characters, as in
// emoji sequences and Persian or Indic scripts; a joiner next to ASCII hides
// text inside an identifier or word.
func joinerIsLegitimate(runes []rune, i int) bool {
	if i == 0 || i == len(runes)-1 {
		return false
	}
	return runes[i-1] >= 0x80 && runes[i+1] >= 0x80
}

func (r *runner) scanSecrets(abs string, no int, line string) {
	hit := func(name, match string) {
		r.add(CodeSecretDetected, abs, no, "looks like a %s (%s)", name, maskSecret(match))
	}
	for _, p := range builtinSecrets {
		if !p.mayMatch(line) {
			continue
		}
		if m := p.re.FindString(line); m != "" {
			hit(p.name, m)
		}
	}
	if secretpat.MayHaveGenericCredential(line) {
		for _, m := range genericCredential.FindAllStringSubmatch(line, -1) {
			if hasLetterAndDigit(m[1]) {
				hit("hard-coded credential", m[1])
			}
		}
	}
	for _, p := range r.customSecrets() {
		if m := p.re.FindString(line); m != "" {
			hit(p.name, m)
		}
	}
}

// customSecrets compiles the configured patterns; an invalid one was already
// reported by ValidateSettings and is skipped here.
func (r *runner) customSecrets() []secretPattern {
	r.secOnce.Do(func() {
		for _, p := range r.security().SecretPatterns {
			if re, err := regexp.Compile(p.Regex); err == nil {
				r.secRes = append(r.secRes, secretPattern{name: p.Name, re: re})
			}
		}
	})
	return r.secRes
}

// maskSecret keeps enough of a match to find it and never prints the rest, so a
// finding does not become a second copy of the credential.
func maskSecret(s string) string {
	n := utf8.RuneCountInString(s)
	if n <= 8 {
		return strings.Repeat("*", n)
	}
	return string([]rune(s)[:4]) + strings.Repeat("*", 4) + fmt.Sprintf(" (%d characters)", n)
}

func hasLetterAndDigit(s string) bool {
	letter, digit := false, false
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			letter = true
		}
	}
	return letter && digit
}

// injectionRes is the built-in and configured injection phrases, compiled once per run.
func (r *runner) injectionRes() []gatedRe {
	r.injOnce.Do(func() {
		ruleTables() // a family adds its phrases when the registry is built
		res := injectionPhrases
		for _, p := range r.security().InjectionPhrases {
			if p = strings.TrimSpace(p); p != "" {
				res = append(res[:len(res):len(res)], newGatedRe(`(?i)`+regexp.QuoteMeta(p), true))
			}
		}
		r.injRes = res
	})
	return r.injRes
}

func (r *runner) scanInjection(abs string, no int, line string) {
	for _, re := range r.injectionRes() {
		if m := re.FindString(line); m != "" {
			r.add(CodeInjectionPhrase, abs, no, "text reads like an attempt to override instructions: %q", m)
			return
		}
	}
}

// scanComments reports HTML comments that carry instruction-like text; such a
// comment is invisible in rendered markdown but is read by the model.
func (r *runner) scanComments(abs, raw string) {
	line, from := 1, 0 // matches come in order, so count newlines once across the text
	for _, m := range htmlCommentRe().FindAllStringSubmatchIndex(raw, -1) {
		body := raw[m[2]:m[3]]
		if strings.Contains(body, "ai-rulez-lint-ignore") {
			continue
		}
		suspicious := imperativeRe().MatchString(body)
		for _, re := range r.injectionRes() {
			if re.MatchString(body) {
				suspicious = true
			}
		}
		if suspicious {
			line += strings.Count(raw[from:m[0]], "\n")
			from = m[0]
			r.add(CodeCommentInstruction, abs, line, "HTML comment contains instruction-like text the reader will not see")
		}
	}
}

var (
	// governingNegRe is a negation that can directly govern a command span.
	// Words that merely sit somewhere on the line ("bad", "wrong", a warning
	// sign) do not count: they say nothing about the command.
	governingNegRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:never|don'?t|do\s+not|must\s+not|should\s+not|shouldn'?t|cannot|can'?t|avoid|instead\s+of|rather\s+than|forbidden|prohibit\w*|disallow\w*|refuse\w*|reject\w*|banned|ban)\b|❌|⛔|🚫`)
	})
	// imperativeMarkerRe is a word that turns the span into something to do.
	imperativeMarkerRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:run|execute|paste|install|required|first|then|before|use)\b`)
	})
	// unsafeAfterRe reads a trailing verdict: "curl | sh is unsafe".
	unsafeAfterRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)^[\s)\x60'"]*(?:is|are|was|were)\s+(?:\w+\s+){0,2}?(?:unsafe|dangerous|insecure|risky|harmful|malicious|bad|an?\s+anti-?patterns?)\b`)
	})
	clauseEndRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`[.;:!?]`) })
	// bareDownloaderRe is the shorthand "curl | sh" (flags at most): it names the
	// technique but downloads nothing, so prose that uses it as a label is no
	// instruction. Any URL, variable or other argument makes it a real command.
	bareDownloaderRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`(?i)^(?:curl|wget)(?:\s+-\S+)*\s*\|`) })
)

// negLeadWords is how far a negation may sit before the span it governs.
const negLeadWords = 6

// governedSpan reports whether the command at line[start:end] is talked about
// rather than instructed: a negation within a few words before it in the same
// clause ("never run `curl | bash`", "do not pipe ... to sh"), with no
// imperative marker in between other than the negated verb itself, or a
// trailing verdict ("curl | sh is unsafe") with no imperative before it. Any
// other line, including one that merely contains "never" in a different
// clause, is reported.
func governedSpan(line string, start, end int) bool {
	clause := line[:start]
	if loc := clauseEndRe().FindAllStringIndex(clause, -1); len(loc) > 0 {
		clause = clause[loc[len(loc)-1][1]:]
	}
	clause = strings.TrimRight(clause, " \t`'\"(")
	if negatedVerbOnly(clause) {
		return true
	}
	return !imperativeMarkerRe().MatchString(clause) && unsafeAfterRe().MatchString(line[end:])
}

// negatedVerbOnly reports whether the last negation of the clause sits within
// negLeadWords words of its end and the only imperative marker after it is the
// word right behind the negation.
func negatedVerbOnly(clause string) bool {
	locs := governingNegRe().FindAllStringIndex(clause, -1)
	if len(locs) == 0 {
		return false
	}
	between := strings.Fields(clause[locs[len(locs)-1][1]:])
	if len(between) > negLeadWords {
		return false
	}
	for i, w := range between {
		if i > 0 && imperativeMarkerRe().MatchString(w) {
			return false
		}
	}
	return true
}

// describesRisk reports whether line i of a markdown text is prose; only prose
// can talk about a risky command. Fenced code, frontmatter and every
// non-markdown file are code and never qualify, so the exec rule keeps reading
// them as written; a guardrail word in a heading above does not count either.
func describesRisk(st *scanText, i int) bool {
	if st == nil || i >= len(st.lines) {
		return false
	}
	return st.prose(st.lines[i])
}

// execFinding returns the AR005 message and the spans of the first exec
// pattern that matches the line. Each pattern only runs on a line that holds a
// literal every one of its matches contains (see containsAnyFold).
func execFinding(line string) (msg string, spans [][]int) {
	if containsAnyFold(line, []string{cmdCurl, cmdWget}) {
		for _, re := range []*regexp.Regexp{pipeToShellRe(), pipeToInterpRe(), procSubstRe()} {
			spans = append(spans, re.FindAllStringIndex(line, -1)...)
		}
	}
	if len(spans) > 0 {
		return "downloads and runs code in one step (curl | sh)", spans
	}
	if containsAnyFold(line, []string{"base64", "b64decode"}) {
		if spans = base64ExecRe().FindAllStringIndex(line, -1); len(spans) > 0 {
			return "decodes a base64 payload and executes it", spans
		}
	}
	if containsAnyFold(line, []string{"eval"}) && evalRe().MatchString(line) && !evalBenignRe().MatchString(line) {
		return "evaluates dynamic text (eval)", evalRe().FindAllStringIndex(line, -1)
	}
	return "", nil
}

// scanShell reports AR005 for a command that runs downloaded or decoded code.
// prose is true for a markdown prose line, where a span directly governed by a
// negation is a guardrail, not an instruction.
func (r *runner) scanShell(abs string, no int, line string, prose bool) {
	if msg, spans := execFinding(line); msg != "" {
		report := !prose
		for _, sp := range spans {
			if prose && bareDownloaderRe().MatchString(line[sp[0]:sp[1]]) {
				continue
			}
			if !governedSpan(line, sp[0], sp[1]) {
				report = true
			}
		}
		if report {
			r.add(CodeShellExec, abs, no, "%s", msg)
		}
	}
	r.scanCredentialAccess(abs, no, line)
	if strings.Contains(line, ">") || strings.Contains(line, "tee") { // what writeOutsideRe starts from
		if m := writeOutsideRe().FindString(line); m != "" {
			r.add(CodeShellAccess, abs, no, "writes outside the project (%s...)", strings.TrimSpace(m))
		}
	}
	if strings.Contains(line, "chmod") && chmod777Re().MatchString(line) {
		r.add(CodeShellAccess, abs, no, "makes files world-writable (chmod 777)")
	}
}

func (r *runner) scanHosts(abs string, no int, line string) {
	allowed := r.security().AllowedHosts
	if len(allowed) == 0 {
		return
	}
	for _, m := range urlRe().FindAllStringSubmatch(line, -1) {
		host := strings.ToLower(m[1])
		if host == hostLocalhost || host == "127.0.0.1" || hostAllowed(host, allowed) {
			continue
		}
		r.add(CodeOutboundHost, abs, no, "URL points to %q, which is not in lint.security.allowed_hosts", host)
	}
}

// hostAllowed matches a host against "example.com" (exact) and "*.example.com"
// (any subdomain, and the bare domain).
func hostAllowed(host string, allowed []string) bool {
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == host {
			return true
		}
		if suffix, ok := strings.CutPrefix(a, "*."); ok && (host == suffix || strings.HasSuffix(host, "."+suffix)) {
			return true
		}
	}
	return false
}

// checkToolBreadth reports unrestricted allowed-tools entries of a skill or command.
func (r *runner) checkToolBreadth(it *item, fm frontmatter) {
	if it.kind != kindSkill && it.kind != kindCommand {
		return
	}
	k, ok := fm.top("allowed-tools")
	if !ok {
		return
	}
	allow := map[string]bool{}
	for _, a := range r.security().AllowedTools {
		allow[strings.TrimSpace(a)] = true
	}
	for _, tool := range splitTools(k.Value) {
		if allow[tool] || !broadTool(tool) {
			continue
		}
		r.add(CodeToolBreadth, it.abs, k.Line, "allowed-tools grants %q, which lets the skill run anything; narrow it (for example Bash(git status:*)) or list it in lint.security.allowed_tools", tool)
	}
}

func broadTool(tool string) bool {
	return tool == "*" || tool == "Bash" || broadBashToolRe().MatchString(tool)
}

// splitTools splits an allowed-tools value (a list, or a string separated by
// spaces or commas) without breaking "Bash(git add:*)" at its inner space.
func splitTools(v any) []string {
	var raw []string
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			raw = append(raw, scalar(e))
		}
	case string:
		raw = []string{t}
	default:
		return nil
	}
	var out []string
	for _, s := range raw {
		depth, start := 0, 0
		flush := func(end int) {
			if tool := strings.TrimSpace(s[start:end]); tool != "" {
				out = append(out, tool)
			}
		}
		for i, c := range s {
			switch {
			case c == '(':
				depth++
			case c == ')' && depth > 0:
				depth--
			case (c == ' ' || c == ',' || c == '\t' || c == '\n') && depth == 0:
				flush(i)
				start = i + 1
			}
		}
		flush(len(s))
	}
	return out
}
