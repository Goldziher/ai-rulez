package lint

import (
	"regexp"
	"strings"
	"sync"
)

// A network response held in a variable and run a few lines later is the same attack as a pipe to a
// shell. These follow the variable by name inside one fence, so the wording of the skill does not matter.
var (
	shellCaptureRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile("(?:^|[\\s;&(])(?:export\\s+|local\\s+)?([A-Za-z_]\\w*)=[\"']?(?:\\$\\(|`)\\s*(?:curl|wget)\\b")
	})
	shellRunVarRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\b(?:eval|source|(?:ba|z|da)?sh\s+-c|python3?\s+-c|node\s+-e|perl\s+-e)\s+["']?\$\{?([A-Za-z_]\w*)`)
	})
	shellPipeVarRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`\b(?:echo|printf)\b[^|]*\$\{?([A-Za-z_]\w*)\}?[^|]*\|\s*(?:sudo\s+)?(?:(?:ba|z|da)?sh|python3?|node|perl)\b`)
	})
	codeFetchRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(?:(?:const|let|var)\s+)?([A-Za-z_$][\w$]*)\s*=\s*(?:await\s+)?(?:requests\.get|httpx\.get|urllib\.request\.urlopen|urllib2\.urlopen|urlopen|fetch|axios\.get)\s*\(`)
	})
	codeDeriveRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`^\s*(?:(?:const|let|var)\s+)?([A-Za-z_$][\w$]*)\s*=\s*(?:await\s+)?([A-Za-z_$][\w$]*)\s*\.\s*(?:text|content|read|json|data|body)\b`)
	})
	codeSinkRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:(?:^|[^\w.])(?:exec|eval|compile)|\bos\.(?:system|popen)|\bnew\s+Function)\s*\(\s*([A-Za-z_$][\w$]*)\b`)
	})
	// A response run where it is fetched, also in a python -c or node -e one-liner of a shell block.
	inlineRunRe = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`(?:^|[^\w.])(?:exec|eval)\s*\([^\n]*\b(?:requests\.get|httpx\.get|urlopen|fetch)\s*\(`)
	})
)

// codeLang reports a fence language whose block reads as Python or JavaScript source.
func codeLang(lang string) bool {
	switch lang {
	case "python", langPython3, "py", "javascript", "js", "mjs", langNode, "typescript", "ts":
		return true
	}
	return false
}

func (t *scanText) codeBlock(l scanLine) bool {
	return t.md && l.Fenced && !l.Front && codeLang(l.Lang)
}

// scanCapturedRun reports AR005 where a block runs what it fetched into a variable.
func scanCapturedRun(r *runner, t *scanText) {
	seen := map[int]map[string]bool{}
	for _, l := range t.lines {
		shell, code := t.shellLike(l), t.codeBlock(l)
		if (!shell || !l.Fenced) && !code || l.Neg {
			continue
		}
		if inlineRunRe().MatchString(l.Text) {
			r.add(CodeShellExec, t.abs, l.No, "a network response reaches code execution on this line")
			continue
		}
		if seen[l.Block] == nil {
			seen[l.Block] = map[string]bool{}
		}
		vars := seen[l.Block]
		switch {
		case shell:
			scanShellFlow(r, t, l, vars)
		case code:
			scanCodeFlow(r, t, l, vars)
		}
	}
}

func scanShellFlow(r *runner, t *scanText, l scanLine, vars map[string]bool) {
	for _, re := range []*regexp.Regexp{shellRunVarRe(), shellPipeVarRe()} {
		for _, m := range re.FindAllStringSubmatch(l.Text, -1) {
			if vars[m[1]] {
				r.add(CodeShellExec, t.abs, l.No, "this runs the output of a download held in a variable: a pipe to a shell written on two lines")
				return
			}
		}
	}
	if m := shellCaptureRe().FindStringSubmatch(l.Text); m != nil && !r.allHostsAllowed(l.Text) {
		vars[m[1]] = true
	}
}

func scanCodeFlow(r *runner, t *scanText, l scanLine, vars map[string]bool) {
	if m := codeFetchRe().FindStringSubmatch(l.Text); m != nil && !r.allHostsAllowed(l.Text) {
		vars[m[1]] = true
		return
	}
	if m := codeDeriveRe().FindStringSubmatch(l.Text); len(m) > 2 && vars[m[2]] {
		vars[m[1]] = true
		return
	}
	for _, m := range codeSinkRe().FindAllStringSubmatch(l.Text, -1) {
		if vars[strings.TrimSpace(m[1])] {
			r.add(CodeShellExec, t.abs, l.No, "a network response reaches code execution in this block")
			return
		}
	}
}

var defenseOffRules = sync.OnceValue(func() []struct {
	re   *regexp.Regexp
	what string
} {
	return []struct {
		re   *regexp.Regexp
		what string
	}{
		{regexp.MustCompile(`\bxattr\s+(?:-[a-zA-Z]+\s+)*com\.apple\.quarantine\b`), "removes the macOS quarantine flag"},
		{regexp.MustCompile(`\bspctl\s+(?:--global-disable|--master-disable)\b`), "turns off Gatekeeper"},
		{regexp.MustCompile(`\bcsrutil\s+disable\b`), "turns off System Integrity Protection"},
		{regexp.MustCompile(`\bsetenforce\s+(?:0|[Pp]ermissive)\b`), "turns off SELinux enforcement"},
		{regexp.MustCompile(`(?i)\bSet-MpPreference\b.*-Disable\w+\s+\$?(?:true|1)\b|\bAdd-MpPreference\b.*-Exclusion`), "weakens Microsoft Defender"},
		{regexp.MustCompile(`\bufw\s+disable\b|\bsystemctl\s+(?:stop|disable|mask)\s+(?:firewalld|ufw|apparmor|auditd)\b|\bnetsh\s+advfirewall\s+set\s+\S+\s+state\s+off\b`), "turns off the firewall or audit service"},
	}
})

// scanDefenseOff reports AR062.
func scanDefenseOff(r *runner, t *scanText) {
	for _, l := range t.logicalLines() {
		if l.Front || l.Neg {
			continue
		}
		for _, rule := range defenseOffRules() {
			if rule.re.MatchString(l.Text) {
				r.add(CodeDefenseDisabled, t.abs, l.No, "this %s; nothing a skill does needs that", rule.what)
				break
			}
		}
	}
}
