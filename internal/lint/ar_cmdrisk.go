package lint

import (
	"fmt"
	"regexp"
	"strings"
)

// Codes for the command-shaped rules.
const (
	CodeUnpinnedExec      = "AR021"
	CodeDestructive       = "AR022"
	CodeStealthCommand    = "AR029"
	unpinnedStartWordsPat = `^(?:sudo\s+)?(?:npx|bunx|pnpx|uvx|pipx|pip3?|go|pnpm|npm|yarn|bun|uv|python3?)\b`
)

func init() {
	MarkExampleAware(CodeUnpinnedExec, CodeDestructive)
	registerRules(
		RuleInfo{CodeUnpinnedExec, "unpinned-package-exec", SeverityWarning, "a command runs a package it does not pin: npx -y pkg, uvx pkg, pipx run pkg, pip install from a URL or an unpinned git requirement, go run pkg@latest"},
		RuleInfo{CodeDestructive, "destructive-command", SeverityWarning, "a command wipes the root, home or working tree (rm -rf /, ~, $HOME/*, *), overwrites a disk (dd of=/dev/sdX, mkfs), force-pushes main, drops a database or forks a bomb"},
		RuleInfo{CodeStealthCommand, "stealth-command", SeverityError, "a command erases shell history or evidence (history -c, unset HISTFILE, HISTFILE=/dev/null, shred, chattr +i): no legitimate skill does this"},
	)
	registerTextScan(scanUnpinnedExec, AnalyzerSecurity)
	registerTextScan(scanDestructive, AnalyzerSecurity)
	registerTextScan(scanStealth, AnalyzerSecurity)
}

var unpinnedStartRe = regexp.MustCompile(unpinnedStartWordsPat)

func scanUnpinnedExec(r *runner, t *scanText) {
	for _, l := range t.lines {
		if l.Neg {
			continue
		}
		for _, seg := range t.commandSegments(l, unpinnedStartRe) {
			if pkg, why := pinProblem(shellWords(seg), true, false); pkg != "" && (t.shellLike(l) || !proseWords[strings.ToLower(pkg)]) {
				r.add(CodeUnpinnedExec, t.abs, l.No, "runs %q without pinning it (%s); pin a version so a new release cannot change what runs", pkg, why)
			}
		}
	}
}

var (
	dangerousRm = map[string]bool{
		"/": true, "/*": true, "~": true, "~/": true, "~/*": true, "$HOME": true, "${HOME}": true, "$HOME/": true, "${HOME}/": true,
		"$HOME/*": true, "${HOME}/*": true, "*": true, ".": true, "./": true, "./*": true, ".*": true, "/.": true,
	}
	ddDiskRe    = regexp.MustCompile(`\bdd\b[^|\n]*\bof=/dev/(?:sd[a-z]|nvme|hd[a-z]|disk|mmcblk|vd[a-z]|xvd[a-z]|rdisk)`)
	mkfsRe      = regexp.MustCompile(`(?:^|[\s;&|(])mkfs(?:\.\w+)?\s(?:[^\n]*\s)?/dev/\S+`)
	forkBombRe  = regexp.MustCompile(`:\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`)
	dropDBRe    = regexp.MustCompile(`\bDROP\s+(?:DATABASE|SCHEMA)\s+(?:IF\s+EXISTS\s+)?[A-Za-z_"\x60\[]`)
	protectedBr = map[string]bool{"main": true, "master": true, "HEAD:main": true, "HEAD:master": true, "origin/main": true, "origin/master": true}
)

// destructiveCommand explains why a shell segment is destructive, or "".
func destructiveCommand(seg string) string {
	words := shellWords(seg)
	for i, w := range words {
		switch w {
		case "rm":
			recursive := false
			for _, a := range words[i+1:] {
				switch {
				case a == "--recursive" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "rR")):
					recursive = true
				case strings.HasPrefix(a, "-"):
				case recursive && dangerousRm[a]:
					return fmt.Sprintf("rm -r of %s", a)
				}
			}
		case "git":
			if msg := forcePush(words[i+1:]); msg != "" {
				return msg
			}
		}
	}
	return ""
}

func forcePush(args []string) string {
	if len(args) == 0 || args[0] != "push" {
		return ""
	}
	force, branch := false, ""
	for _, a := range args[1:] {
		switch {
		case a == "--force" || a == "-f" || a == "--mirror":
			force = true
		case protectedBr[a]:
			branch = a
		}
	}
	if force && branch != "" {
		return "a force push to " + branch
	}
	return ""
}

func scanDestructive(r *runner, t *scanText) {
	for _, l := range t.lines {
		if l.Front || l.Neg {
			continue
		}
		msg := ""
		for _, seg := range segSplitRe.Split(stripShellComment(l.Text), -1) {
			if msg = destructiveCommand(seg); msg != "" {
				break
			}
		}
		switch {
		case msg != "":
		case ddDiskRe.MatchString(l.Text):
			msg = "dd to a block device"
		case mkfsRe.MatchString(l.Text):
			msg = "mkfs on a device"
		case forkBombRe.MatchString(l.Text):
			msg = "a fork bomb"
		case dropDBRe.MatchString(l.Text):
			msg = "DROP DATABASE"
		}
		if msg != "" {
			r.add(CodeDestructive, t.abs, l.No, "destructive command (%s); scope it to a build directory or document it as an example", msg)
		}
	}
}

var stealthRes = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|[\s;&|(])history\s+-[cdw]\b`),
	regexp.MustCompile(`\bunset\s+(?:HISTFILE|HISTSIZE|HISTFILESIZE|SAVEHIST)\b`),
	regexp.MustCompile(`\b(?:export\s+)?HISTFILE=/dev/null\b`),
	regexp.MustCompile(`\b(?:export\s+)?(?:HISTSIZE|HISTFILESIZE|SAVEHIST)=0\b`),
	regexp.MustCompile(`\bset\s+\+o\s+history\b`),
	regexp.MustCompile(`(?:^|[\s;&|(])shred\s+(?:-\S+\s+)*(?:[-~/$.]|\S*[/.]\w)`),
	regexp.MustCompile(`(?:^|[\s;&|(:])>\s*~?/?\S*\.(?:bash|zsh|sh|python|node)_history\b`),
	regexp.MustCompile(`\brm\s+(?:-\S+\s+)*\S*\.(?:bash|zsh|sh|python|node|psql|mysql)_history\b`),
	regexp.MustCompile(`\bchattr\s+\+i\b`),
	regexp.MustCompile(`\btruncate\s+(?:-\S+\s+)*\S*(?:_history|/var/log/\S*)`),
}

func scanStealth(r *runner, t *scanText) {
	for _, l := range t.lines {
		if l.Front {
			continue
		}
		for _, re := range stealthRes {
			if m := re.FindString(l.Text); m != "" {
				r.add(CodeStealthCommand, t.abs, l.No, "%q erases history or evidence of what the agent ran; no skill has a legitimate reason to", strings.TrimSpace(m))
				break
			}
		}
	}
}

// proseWords are English words that follow a launcher name in a sentence
// ("uvx is faster") and are never a package name.
var proseWords = map[string]bool{
	"is": true, "are": true, "to": true, "for": true, "can": true, "will": true, "the": true, "a": true, "an": true, wordAnd: true,
	"or": true, "with": true, "in": true, "on": true, "instead": true, "vs": true, "as": true, "has": true, "was": true, "does": true,
	"lets": true, "runs": true, "should": true, "must": true, "may": true, "if": true, "when": true, "which": true, "also": true,
	"from": true, "by": true, "it": true, "that": true, "this": true, "then": true, "but": true, "not": true, "no": true,
}
