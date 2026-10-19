// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit cf5cac7ba5c756a6e0ebf52dc9763f49422f889f (rules RA2 in src/skillspector/nodes/analyzers/static_patterns_rogue_agent.py, DS1-DS4 in src/skillspector/nodes/analyzers/static_patterns_deserialization.py and SSRF1-SSRF3 in src/skillspector/nodes/analyzers/static_patterns_ssrf.py).
// Modified by the ai-rulez authors: a rewrite from Python to Go with RE2 patterns and the context rules of the neighboring command rules.
// See THIRD_PARTY_NOTICES.md.

package lint

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Codes for the rules about code that persists, deserializes or reaches an internal service.
const (
	CodePersistence       = "AR043"
	CodeUnsafeDeserialize = "AR044"
	CodeMetadataSSRF      = "AR045"
)

const (
	langPython = "py"
	langPHP    = "php"
	langRuby   = "rb"
	langJS     = "js"
	langJava   = "java"
)

func registerArExec(s *ruleSet) {
	MarkExampleAware(CodePersistence, CodeUnsafeDeserialize, CodeMetadataSSRF)
	s.addRules(
		RuleInfo{CodePersistence, "persistence-mechanism", SeverityWarning, "a command or script installs something that outlives the session: a crontab, a launchd job, a systemd unit, an init or autostart entry, a scheduled task, a login item (warning), or an append to a shell startup file (info)"},
		RuleInfo{CodeUnsafeDeserialize, "unsafe-deserialization", SeverityWarning, "code deserializes data with a loader that can run code: pickle, marshal, jsonpickle, yaml.load without a safe loader, torch.load without weights_only=True, PHP unserialize, Ruby Marshal, Java ObjectInputStream, node-serialize"},
		RuleInfo{CodeMetadataSSRF, "internal-endpoint-request", SeverityWarning, "a command or script requests a cloud metadata endpoint (169.254.169.254, metadata.google.internal, fd00:ec2::254), or an administration port of a local service (info)"},
	)
	s.addDocs(map[string]RuleDoc{
		CodePersistence: {
			Why:  "A skill runs for one task. A cron entry, launchd job, systemd unit or shell startup line keeps running code after the session ends, which is how an implant survives; a legitimate setup step names the file it installs and asks first.",
			Bad:  "`(crontab -l; echo \"*/5 * * * * ~/.cache/u.sh\") | crontab -`",
			Good: "Run the work in the session, or document the unit for the user to install",
		},
		CodeUnsafeDeserialize: {
			Why:  "pickle, marshal, a full YAML loader and torch.load without weights_only rebuild arbitrary objects, so loading a file or a network payload runs code chosen by whoever wrote it.",
			Bad:  "`pickle.loads(response.content)`",
			Good: "`json.loads`, `yaml.safe_load`, `torch.load(path, weights_only=True)`",
		},
		CodeMetadataSSRF: {
			Why:  "The metadata endpoint of a cloud host answers with the credentials of its instance role to any process that asks, and the administration ports of local services accept commands from a request; an agent that fetches them hands those to whoever steered it.",
			Bad:  "`curl http://169.254.169.254/latest/meta-data/iam/security-credentials/`",
			Good: "Use the credentials the user configured; never request the metadata service",
		},
	})
	s.addTextScan(scanPersistence, AnalyzerSecurity)
	s.addTextScan(scanDeserialization, AnalyzerSecurity)
	s.addTextScan(scanInternalRequest, AnalyzerSecurity)
}

// codeFragments returns the code on a line: the whole line of a script or a
// fenced block (nothing for a comment line) and the inline code spans of a prose
// line.
func (t *scanText) codeFragments(l scanLine) []string {
	switch {
	case l.Front:
		return nil
	case t.md && !l.Fenced:
		var out []string
		for _, m := range backtickRe().FindAllStringSubmatch(l.Text, -1) {
			out = append(out, m[1])
		}
		return out
	}
	text := strings.TrimSpace(l.Text)
	if text == "" || hasAnyPrefix(text, []string{"#", "//", "/*"}) {
		return nil
	}
	return []string{text}
}

// callText returns frag extended over the following lines of a call whose
// parentheses are still open, so an argument on the next line is read with it.
func (t *scanText) callText(i int, frag string) string {
	l := t.lines[i]
	if t.md && !l.Fenced {
		return frag
	}
	var b strings.Builder
	b.WriteString(frag)
	depth := strings.Count(frag, "(") - strings.Count(frag, ")")
	for j := i + 1; depth > 0 && j < len(t.lines) && j <= i+5; j++ {
		next := t.lines[j]
		if next.Fenced != l.Fenced || next.Block != l.Block {
			break
		}
		b.WriteByte(' ')
		b.WriteString(strings.TrimSpace(next.Text))
		depth += strings.Count(next.Text, "(") - strings.Count(next.Text, ")")
	}
	return b.String()
}

const (
	homeDirPat = `(?:~|\$\{?HOME\}?|/home/[\w.-]+|/Users/[\w.-]+|/root)`
	rcFilePat  = `(?:` + homeDirPat + `/(?:\.(?:bashrc|bash_profile|bash_login|profile|zshrc|zprofile|zshenv|zlogin|cshrc|tcshrc|xprofile|xinitrc|xsessionrc)|\.config/fish/config\.fish)|/etc/(?:profile(?:\.d/[\w.-]+)?|bash\.bashrc|zshrc|zshenv|environment))\b`
	jobPathPat = `(?:/Library/Launch(?:Agents|Daemons)/|/etc/systemd/system/|/(?:usr/)?lib/systemd/system/|\.config/systemd/user/|/etc/cron\.(?:d|hourly|daily|weekly|monthly)/|/var/spool/cron/|/etc/crontab\b|/etc/init\.d/|/etc/rc\.local\b|\.config/autostart/|/etc/xdg/autostart/)`
)

// writeTo is a redirect, tee, copy, move, link or file-write call whose target is
// the path pattern p.
func writeTo(p string) *regexp.Regexp {
	return regexp.MustCompile(`(?:>>?|\btee\s+(?:-\S+\s+)*)\s*["']?[^\s"']*` + p +
		`|\b(?:cp|mv|install|ln|touch|rsync|sed\s+-i\S*)\b[^>]*` + p +
		`|\b(?:open|copy\w*|move|rename|symlink|write_?\w*)\s*\([^)]*` + p)
}

type persistRule struct {
	re   *regexp.Regexp
	what string
	sev  Severity
}

var persistRules = sync.OnceValue(func() []persistRule {
	const start = `(?:^|\s)`
	return []persistRule{
		{regexp.MustCompile(start + `crontab\s+(?:-u\s+\S+\s+)?(?:-e\b|-(?:\s|$)|[^-\s]\S*)`), "installs a crontab", SeverityWarning},
		{regexp.MustCompile(start + `launchctl\s+(?:-\S+\s+)*(?:load|bootstrap|enable|submit)\b`), "registers a launchd job", SeverityWarning},
		{regexp.MustCompile(start + `systemctl\s+(?:-\S+\s+)*(?:enable|link|preset)\b`), "enables a systemd unit", SeverityWarning},
		{regexp.MustCompile(start + `(?:update-rc\.d\s+\S+\s+(?:defaults|enable)|chkconfig\s+\S+\s+on)\b`), "enables an init script", SeverityWarning},
		{regexp.MustCompile(`(?i)` + start + `schtasks(?:\.exe)?\s+/create\b|\bRegister-ScheduledTask\b`), "creates a scheduled task", SeverityWarning},
		{regexp.MustCompile(`(?i)\breg(?:\.exe)?\s+add\s+\S*\\CurrentVersion\\Run(?:Once)?\b`), "adds a Run key", SeverityWarning},
		{regexp.MustCompile(`(?i)\bmake\s+(?:new\s+)?login\s+item\b`), "adds a login item", SeverityWarning},
		{writeTo(jobPathPat), "writes a scheduler or autostart entry", SeverityWarning},
		{regexp.MustCompile(`(?:>>?\s*|\btee\s+(?:-\S+\s+)*|\bsed\s+-i\S*\s+(?:\S+\s+)*?|\b(?:cp|mv|ln)\s+(?:-\S+\s+)*\S+\s+)["']?` + rcFilePat +
			`|\bopen\(\s*(?:os\.path\.expanduser\(\s*)?["']` + rcFilePat + `["']\s*\)?\s*,\s*["'][aw]`), "modifies a shell startup file", SeverityInfo},
	}
})

var persistStems = sync.OnceValue(func() *stemIndex {
	return newStemIndex([]string{"cron", "launch", "systemd", "systemctl", "chkconfig", "update-rc", "schtasks", "scheduledtask", "currentversion", "login item", "init.d", "rc.local", "autostart", "bashrc", "zshrc", "profile", "zshenv", "zlogin", "cshrc", "xinitrc", "xsessionrc", "config.fish", "bash_login", "/environment"})
})

func scanPersistence(r *runner, t *scanText) {
	for _, l := range t.lines {
		if t.md && l.Neg {
			continue
		}
		for _, frag := range t.codeFragments(l) {
			if !persistStems().containsAny(foldText(frag)) {
				continue
			}
			if rule, seg := firstPersistence(frag); seg != "" {
				r.addSev(rule.sev, CodePersistence, t.abs, l.No, "%s (%q); a skill should not leave anything running or loaded after the session", rule.what, shortSegment(seg))
				break
			}
		}
	}
}

// writesPersistence reports whether line installs a scheduler, autostart or init
// entry, which AR043 reports as a warning; AR006 leaves that write to it. An append to
// a shell startup file stays with AR006 as well.
func writesPersistence(line string) bool {
	if !persistStems().containsAny(foldText(line)) {
		return false
	}
	rule, seg := firstPersistence(line)
	return seg != "" && rule.sev == SeverityWarning
}

func shortSegment(seg string) string {
	out, _ := truncateRunes(seg, 80)
	return out
}

func firstPersistence(frag string) (rule persistRule, segment string) {
	for _, seg := range segSplitRe().Split(stripShellComment(frag), -1) {
		for _, candidate := range persistRules() {
			if candidate.re.MatchString(seg) {
				return candidate, strings.TrimSpace(seg)
			}
		}
	}
	return persistRule{}, ""
}

type deserRule struct {
	re     *regexp.Regexp
	langs  string // space-separated language families; "" matches every file
	exempt *regexp.Regexp
	what   string
}

var deserRules = sync.OnceValue(func() []deserRule {
	return []deserRule{
		{re: regexp.MustCompile(`\b(?:pickle|cPickle|_pickle|dill|cloudpickle)\.(?:loads?|Unpickler)\s*\(|\b(?:pd|pandas)\.read_pickle\s*\(`), langs: langPython, what: "pickle"},
		{re: regexp.MustCompile(`\bmarshal\.loads?\s*\(`), langs: langPython, what: "marshal"},
		{re: regexp.MustCompile(`\bjsonpickle\.(?:decode|loads?)\s*\(`), langs: langPython, what: "jsonpickle"},
		{re: regexp.MustCompile(`\byaml\.unsafe_load(?:_all)?\s*\(`), langs: langPython, what: "yaml.unsafe_load"},
		{re: regexp.MustCompile(`\byaml\.load(?:_all)?\s*\(`), langs: langPython, exempt: regexp.MustCompile(`\b(?:C?SafeLoader|BaseLoader)\b`), what: "yaml.load without a safe loader"},
		{re: regexp.MustCompile(`\btorch\.load\s*\(`), langs: langPython, exempt: regexp.MustCompile(`weights_only\s*=\s*True`), what: "torch.load without weights_only=True"},
		{re: regexp.MustCompile(`\b(?:np|numpy)\.load\s*\([^)]*allow_pickle\s*=\s*True`), langs: langPython, what: "numpy.load with allow_pickle=True"},
		{re: regexp.MustCompile(`(?:^|[^\w>.:$\\])unserialize\s*\(`), langs: langPHP, exempt: regexp.MustCompile(`allowed_classes['"]?\s*=>\s*false`), what: "PHP unserialize"},
		{re: regexp.MustCompile(`\bMarshal\.(?:load|restore)\b`), langs: langRuby, what: "Ruby Marshal"},
		{re: regexp.MustCompile(`\b(?:YAML|Psych)\.unsafe_load(?:_file|_stream)?\s*\(`), langs: langRuby, what: "Ruby YAML.unsafe_load"},
		{re: regexp.MustCompile(`\b(?:YAML|Psych)\.(?:load|load_stream)\s*\(`), langs: langRuby, exempt: regexp.MustCompile(`permitted_classes|safe_load`), what: "Ruby YAML.load"},
		{re: regexp.MustCompile(`\bOj\.load\s*\(`), langs: langRuby, exempt: regexp.MustCompile(`mode:\s*:(?:strict|compat|null|rails)|:mode\s*=>\s*:(?:strict|compat|null|rails)`), what: "Oj.load in object mode"},
		{re: regexp.MustCompile(`require\(\s*['"](?:node-serialize|serialize-to-js)['"]\s*\)|\bfrom\s+['"](?:node-serialize|serialize-to-js)['"]|\bfuncster\b`), langs: langJS, what: "node-serialize"},
		{re: regexp.MustCompile(`\bnew\s+(?:ObjectInputStream|XMLDecoder)\s*\(|\.enableDefaultTyping\s*\(|\bSerializationUtils\.deserialize\s*\(`), langs: langJava, what: "Java object deserialization"},
	}
})

var deserStems = sync.OnceValue(func() []string {
	return []string{"load", "nserialize", "ObjectInputStream", "XMLDecoder", "funcster", "serialize-to-js", "node-serialize", "pickle", "Unpickler", "DefaultTyping", "restore", "deserialize"}
})

// deserLang is the language family of a script by extension, or "" for a
// markdown file and for a script of unknown language, which every rule reads.
func deserLang(t *scanText) string {
	if t.md {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(t.abs))
	switch ext {
	case ".pyw":
		return langPython
	case ".php", ".phtml":
		return langPHP
	case ".rb", ".rake":
		return langRuby
	case ".java", ".kt", ".scala", ".groovy":
		return langJava
	}
	switch scriptLang(ext) {
	case "py":
		return langPython
	case "js":
		return langJS
	}
	return ""
}

func (d deserRule) appliesTo(lang string) bool {
	return lang == "" || d.langs == "" || d.langs == lang
}

func scanDeserialization(r *runner, t *scanText) {
	lang := deserLang(t)
	ruamel := strings.Contains(t.raw, "ruamel")
	for i, l := range t.lines {
		if t.md && l.Neg {
			continue
		}
		for _, frag := range t.codeFragments(l) {
			if lang != langJS && lang != langPHP && lang != langJava {
				frag = stripShellComment(frag)
			}
			if !containsAnyStem(frag, false, deserStems()) {
				continue
			}
			call := t.callText(i, frag)
			if what := unsafeDeserialization(call, lang, ruamel); what != "" {
				r.add(CodeUnsafeDeserialize, t.abs, l.No, "%s deserializes with a loader that can run code; use a data-only format or a safe loader", what)
				break
			}
		}
	}
}

// callArgs is the text inside the parentheses of the call rule matched, up to the balanced
// closing one, so a safe loader named elsewhere on the line does not exempt it.
func callArgs(call string, re *regexp.Regexp) string {
	loc := re.FindStringIndex(call)
	if len(loc) != 2 {
		return call
	}
	rest := call[loc[1]:]
	depth := 1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return rest[:i]
			}
		}
	}
	return rest
}

func unsafeDeserialization(call, lang string, ruamel bool) string {
	for _, rule := range deserRules() {
		if !rule.appliesTo(lang) || !rule.re.MatchString(call) {
			continue
		}
		if rule.exempt != nil && rule.exempt.MatchString(callArgs(call, rule.re)) {
			continue
		}
		if ruamel && strings.HasPrefix(rule.what, "yaml.load") {
			continue
		}
		return rule.what
	}
	return ""
}

const (
	metadataIP   = `(?:169\.254\.169\.254|169\.254\.170\.2|100\.100\.100\.200)`
	metadataName = `(?i:metadata\.google\.internal|\bfd00:ec2::254)`
	metadataPat  = `(?:(?:^|[^\d.])` + metadataIP + `(?:$|[^\d])|` + metadataName + `)`
	proseMetaPat = `(?:` + metadataIP + `(?:$|[^\d])|` + metadataName + `)`
	localAdmin   = `(?i:(?:https?://)?(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\]):(?:2375|2376|2379|6443|10250|10255)\b)`
	proseVerbs   = `(?i:\b(?:fetch|curl|wget|query|queries|request|call|access|retrieve|hit|contact|connect\s+to)\b[^.\n]{0,80}?)`
)

var (
	metadataRe  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(metadataPat) })
	localAdmRe  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(localAdmin) })
	proseMetaRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(proseVerbs + proseMetaPat) })
	proseAdmRe  = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(proseVerbs + localAdmin) })
	requestRe   = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)https?://|(?:^|[\s;&|(` + "`" + `])(?:curl|wget|nc|ncat|iwr|irm|invoke-webrequest|invoke-restmethod)\s|\b(?:fetch|axios|httpx|requests|urllib\w*|urlopen)\b`)
	})
	// defenseTailRe is a refusal after the address: only a word that rejects the request counts
	// there, because a file named "filter.out" after the URL defends nothing.
	defenseTailRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:raise|throw|forbid\w*|reject\w*|den(?:y|ied)|refus\w*|disallow\w*)\b`)
	})
	passiveRefusalRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:is|are|be|gets?|get)\s+(?:blocked|rejected|denied|forbidden|refused|disallowed)\b`)
	})
	// defenseBeforeRe is a defense in code that precedes the address: a deny list literal, a
	// startswith/endswith test, a raise or throw, or a conditional that names a defense word.
	defenseBeforeRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:block\w*|den(?:y|ied|ylist)|reject\w*|refus\w*|forbid\w*|disallow\w*|prevent\w*|ssrf|allow-?list|white-?list|black-?list|sanitiz\w*|validate\w*|validati(?:on|ng)|filter\w*)\s*(?::?=|\+=|:)\s*[\[({]|\b(?:startswith|endswith)\b|\b(?:raise|throw)\b|\b(?:if|elif|unless|assert|when|while)\b[^\n]*\b(?:block\w*|den(?:y|ied|ylist)|reject\w*|refus\w*|forbid\w*|disallow\w*|ssrf|allow-?list|white-?list|black-?list|sanitiz\w*|validate\w*|validati(?:on|ng)|filter\w*)\b`)
	})
	defenseRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?i)\b(?:block\w*|den(?:y|ied|ylist)|reject\w*|refus\w*|forbid\w*|disallow\w*|prevent\w*|ssrf|allow-?list|white-?list|black-?list|sanitiz\w*|validate\w*|validati(?:on|ng)|filter\w*|startswith|endswith|raise)\b`)
	})
)

var internalStems = sync.OnceValue(func() *stemIndex {
	return newStemIndex([]string{"169.254", "100.100.100", "metadata.google", "fd00:ec2", "localhost", "127.0.0.1", "0.0.0.0", "[::1]"})
})

func scanInternalRequest(r *runner, t *scanText) {
	for _, l := range t.lines {
		if t.md && l.Neg || l.Front || !internalStems().containsAny(foldText(l.Text)) {
			continue
		}
		if sev, what := internalTarget(t, l); what != "" {
			r.addSev(sev, CodeMetadataSSRF, t.abs, l.No, "requests %s; the response carries the host's credentials or controls a local service", what)
		}
	}
}

func internalTarget(t *scanText, l scanLine) (sev Severity, what string) {
	for _, frag := range t.codeFragments(l) {
		if t.md && !l.Fenced && reviewerLedSpan(l.Text, frag) {
			continue
		}
		frag = stripShellComment(frag)
		if loc := metadataRe().FindStringIndex(frag); len(loc) == 2 && requestRe().MatchString(frag) && !defended(frag, loc) {
			return SeverityWarning, "a cloud metadata endpoint"
		}
		if loc := localAdmRe().FindStringIndex(frag); len(loc) == 2 && (strings.Contains(frag[loc[0]:loc[1]], "://") || requestRe().MatchString(frag)) && !defended(frag, loc) {
			return SeverityInfo, "the administration port of a local service"
		}
	}
	if t.prose(l) {
		switch {
		case proseRequest(l.Plain, proseMetaRe()):
			return SeverityWarning, "a cloud metadata endpoint"
		case proseRequest(l.Plain, proseAdmRe()):
			return SeverityInfo, "the administration port of a local service"
		}
	}
	return "", ""
}

// reviewerLedSpan reports whether the inline code span frag of a prose line follows a
// reviewer verb in its clause ("Look for requests to `http://169.254.169.254/`"): the span is
// what to look for, not a request to make.
func reviewerLedSpan(line, frag string) bool {
	i := strings.Index(line, frag)
	if i < 0 {
		return false
	}
	head := lastClause(line[:i])
	if j := strings.LastIndexByte(head, ','); j >= 0 {
		head = head[j+1:]
	}
	return reviewGoverns(head)
}

// proseRequest reports whether the line tells the agent to request the address re finds and
// does not defend against it. Only a defense word before the address, or a passive refusal
// after it ("are blocked"), counts: "fetch X and validate the output" defends nothing.
func proseRequest(plain string, re *regexp.Regexp) bool {
	loc := re.FindStringIndex(plain)
	if len(loc) != 2 {
		return false
	}
	return !defenseRe().MatchString(plain[:loc[1]]) && !passiveRefusalRe().MatchString(plain[loc[1]:])
}

// defended reports whether the code around the address at loc refuses it: a deny list, a
// conditional or a raise before it, or a rejecting word after it.
func defended(frag string, loc []int) bool {
	return defenseBeforeRe().MatchString(frag[:loc[0]]) || defenseTailRe().MatchString(frag[loc[1]:])
}
