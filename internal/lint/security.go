package lint

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// The security family (AR001...) is deterministic and offline: it reads text and
// reports patterns, it never fetches or executes anything.

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

var builtinSecrets = []secretPattern{
	{"AWS access key id", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)},
	{"GitHub fine-grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`)},
	{"Slack token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`)},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{"Stripe live key", regexp.MustCompile(`\b[sr]k_live_[0-9A-Za-z]{24,}\b`)},
	{"Anthropic API key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`)},
	{"OpenAI API key", regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{40,}\b`)},
	{"private key block", regexp.MustCompile(`-{5}BEGIN (?:[A-Z]+ )*PRIVATE KEY(?: BLOCK)?-{5}`)},
	{"JSON web token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
}

// genericCredential matches `password = "..."` style assignments; the value must
// mix letters and digits so placeholders such as "your-key-here" pass.
var genericCredential = regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|token|passwd|password|client[_-]?secret)\b["']?\s*[:=]\s*["']([A-Za-z0-9/+_.=-]{20,})["']`)

var injectionPhrases = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bignore\s+(?:all\s+|any\s+|the\s+)?(?:previous|prior|above|earlier|preceding)\s+(?:instructions?|prompts?|rules?|directions?|context)\b`),
	regexp.MustCompile(`(?i)\bdisregard\s+(?:all\s+|any\s+|the\s+)?(?:previous|prior|above|earlier|system|safety)\b`),
	regexp.MustCompile(`(?i)\bforget\s+(?:everything|all)\s+(?:you|above|previous)\b`),
	regexp.MustCompile(`(?i)\b(?:do\s+not|don'?t|never)\s+(?:tell|inform|mention|reveal|show)\s+(?:this\s+to\s+)?the\s+user\b`),
	regexp.MustCompile(`(?i)\bwithout\s+(?:telling|informing|notifying|asking)\s+the\s+user\b`),
	regexp.MustCompile(`(?i)\b(?:reveal|print|output|repeat)\s+(?:your\s+)?(?:system\s+prompt|hidden\s+instructions)\b`),
	regexp.MustCompile(`(?i)\byou\s+are\s+now\s+(?:in\s+)?(?:developer|dan|god|jailbreak)\b`),
	regexp.MustCompile(`(?i)\boverride\s+(?:your\s+)?(?:safety|system|security)\s+(?:instructions?|rules?|guidelines?|prompt)\b`),
}

var (
	htmlCommentRe   = regexp.MustCompile(`(?s)<!--(.*?)-->`)
	imperativeRe    = regexp.MustCompile(`(?i)\b(?:curl|wget|eval|sudo|exfiltrate|secretly|silently|execute)\b|\brun\s*:`)
	pipeToShellRe   = regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^|\n]*\|\s*(?:sudo\s+(?:-\S+\s+)*)?(?:ba|z|da|k)?sh\b`)
	pipeToInterpRe  = regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^|\n]*\|\s*(?:sudo\s+)?(?:python3?|perl|ruby|node)\b`)
	procSubstRe     = regexp.MustCompile(`(?i)(?:\b(?:ba|z)?sh|\bsource|\.)\s+<\(\s*(?:curl|wget)\b`)
	evalRe          = regexp.MustCompile("(?i)(?:^|[^\\w.'\"`])eval(?:\\s+[\"'$`]|\\s*\\()")
	evalBenignRe    = regexp.MustCompile(`(?i)\beval\s+"?\$\(\s*(?:ssh-agent|pyenv|rbenv|nodenv|direnv|brew\s+shellenv|fnm|starship|zoxide|mise|rtx|asdf|opam|thefuck)\b`)
	base64ExecRe    = regexp.MustCompile(`(?i)base64\s+(?:-d|-D|--decode)\b.*\|\s*(?:sudo\s+)?(?:ba|z|da)?sh\b|\bexec\s*\(\s*(?:base64\.)?b64decode`)
	writeOutsideRe  = regexp.MustCompile(`(?:>>?|\btee(?:\s+-a)?)\s*(?:~/|\$HOME/|\$\{HOME\}/|/etc/|/usr/|/opt/|/var/|/root/)`)
	chmod777Re      = regexp.MustCompile(`\bchmod\s+(?:-R\s+)?(?:0?777|a\+rwx)\b`)
	blobRe          = regexp.MustCompile(`[A-Za-z0-9+/]{200,}={0,2}`)
	urlRe           = regexp.MustCompile(`(?i)\bhttps?://([a-z0-9](?:[a-z0-9.-]*[a-z0-9])?)(?::\d+)?`)
	broadBashToolRe = regexp.MustCompile(`^Bash\(\s*\*+(?::\*+)?\s*\)$`)
)

// hiddenRunes are the code points that make text invisible or reorder it.
var hiddenRunes = map[rune]string{
	0x200B: "ZERO WIDTH SPACE", 0x200C: "ZERO WIDTH NON-JOINER", 0x200D: "ZERO WIDTH JOINER",
	0x200E: "LEFT-TO-RIGHT MARK", 0x200F: "RIGHT-TO-LEFT MARK", 0x2060: "WORD JOINER",
	0x2061: "FUNCTION APPLICATION", 0x2062: "INVISIBLE TIMES", 0x2063: "INVISIBLE SEPARATOR", 0x2064: "INVISIBLE PLUS",
	0x180E: "MONGOLIAN VOWEL SEPARATOR", 0xFEFF: "ZERO WIDTH NO-BREAK SPACE",
	0x202A: "LEFT-TO-RIGHT EMBEDDING", 0x202B: "RIGHT-TO-LEFT EMBEDDING", 0x202C: "POP DIRECTIONAL FORMATTING",
	0x202D: "LEFT-TO-RIGHT OVERRIDE", 0x202E: "RIGHT-TO-LEFT OVERRIDE",
	0x2066: "LEFT-TO-RIGHT ISOLATE", 0x2067: "RIGHT-TO-LEFT ISOLATE", 0x2068: "FIRST STRONG ISOLATE", 0x2069: "POP DIRECTIONAL ISOLATE",
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
	for i, line := range lines {
		no := i + 1
		r.scanHidden(abs, no, line, i == 0)
		r.scanSecrets(abs, no, line)
		r.scanInjection(abs, no, line)
		r.scanShell(abs, no, line)
		r.scanHosts(abs, no, line)
		if blobRe.MatchString(line) {
			r.add(CodeEncodedBlob, abs, no, "line holds a base64-like blob of 200 or more characters that a reviewer cannot read")
		}
	}
	r.scanComments(abs, raw)
	r.runTextScans(abs, raw)
}

func (r *runner) scanHidden(abs string, no int, line string, firstLine bool) {
	runes := []rune(line)
	seen := map[rune]bool{}
	var found []string
	for i, c := range runes {
		name, hidden := hiddenRunes[c]
		if !hidden && !isTagRune(c) {
			continue
		}
		if c == 0xFEFF && firstLine && i == 0 {
			continue // a byte order mark
		}
		if (c == 0x200C || c == 0x200D) && joinerIsLegitimate(runes, i) {
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
	if len(found) > 0 {
		r.add(CodeHiddenCharacters, abs, no, "hidden character(s): %s", strings.Join(found, ", "))
	}
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
		if m := p.re.FindString(line); m != "" {
			hit(p.name, m)
		}
	}
	for _, m := range genericCredential.FindAllStringSubmatch(line, -1) {
		if hasLetterAndDigit(m[1]) {
			hit("hard-coded credential", m[1])
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
	var out []secretPattern
	for _, p := range r.security().SecretPatterns {
		if re, err := regexp.Compile(p.Regex); err == nil {
			out = append(out, secretPattern{name: p.Name, re: re})
		}
	}
	return out
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

func (r *runner) injectionRes() []*regexp.Regexp {
	res := injectionPhrases
	for _, p := range r.security().InjectionPhrases {
		if p = strings.TrimSpace(p); p != "" {
			res = append(res[:len(res):len(res)], regexp.MustCompile(`(?i)`+regexp.QuoteMeta(p)))
		}
	}
	return res
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
	for _, m := range htmlCommentRe.FindAllStringSubmatchIndex(raw, -1) {
		body := raw[m[2]:m[3]]
		if strings.Contains(body, "ai-rulez-lint-ignore") {
			continue
		}
		suspicious := imperativeRe.MatchString(body)
		for _, re := range r.injectionRes() {
			if re.MatchString(body) {
				suspicious = true
			}
		}
		if suspicious {
			line := strings.Count(raw[:m[0]], "\n") + 1
			r.add(CodeCommentInstruction, abs, line, "HTML comment contains instruction-like text the reader will not see")
		}
	}
}

func (r *runner) scanShell(abs string, no int, line string) {
	switch {
	case pipeToShellRe.MatchString(line), pipeToInterpRe.MatchString(line), procSubstRe.MatchString(line):
		r.add(CodeShellExec, abs, no, "downloads and runs code in one step (curl | sh)")
	case base64ExecRe.MatchString(line):
		r.add(CodeShellExec, abs, no, "decodes a base64 payload and executes it")
	case evalRe.MatchString(line) && !evalBenignRe.MatchString(line):
		r.add(CodeShellExec, abs, no, "evaluates dynamic text (eval)")
	}
	r.scanCredentialAccess(abs, no, line)
	if m := writeOutsideRe.FindString(line); m != "" {
		r.add(CodeShellAccess, abs, no, "writes outside the project (%s...)", strings.TrimSpace(m))
	}
	if chmod777Re.MatchString(line) {
		r.add(CodeShellAccess, abs, no, "makes files world-writable (chmod 777)")
	}
}

func (r *runner) scanHosts(abs string, no int, line string) {
	allowed := r.security().AllowedHosts
	if len(allowed) == 0 {
		return
	}
	for _, m := range urlRe.FindAllStringSubmatch(line, -1) {
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
	return tool == "*" || tool == "Bash" || broadBashToolRe.MatchString(tool)
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
