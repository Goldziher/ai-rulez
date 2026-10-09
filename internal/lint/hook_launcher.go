package lint

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// launcher describes how an interpreter takes its script: the flags that make
// the rest of the line inline code (no script file), and the flags that
// consume the next word.
type launcher struct {
	inline  map[string]bool
	withArg map[string]bool
	// shell marks sh-like interpreters, where a short-flag cluster holding c (-ec) is inline code.
	shell bool
	// evalCluster marks interpreters where a cluster ending in e or E (-ne, -pe) is inline code.
	evalCluster bool
}

var hookLaunchers = map[string]launcher{
	"sh":       shellLauncher,
	shellBash:  shellLauncher,
	shellZsh:   shellLauncher,
	"dash":     shellLauncher,
	toolPython: {inline: set("-c", "-m"), withArg: set("-W", "-X", "-Q")},
	"node":     {inline: set("-e", "-p", "--eval", "--print"), withArg: set("-r", "--require", "--import", "--loader", "--experimental-loader", "--input-type")},
	"ruby":     {inline: set("-e"), withArg: set("-r", "-I", "-C"), evalCluster: true},
	"perl":     {inline: set("-e", "-E"), withArg: set("-I", "-M", "-m"), evalCluster: true},
}

var shellLauncher = launcher{inline: set("-c"), withArg: set("-o", "+o", "-O", "+O"), shell: true}

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}

var (
	versionedInterpRe = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^(python|ruby|perl|node)[0-9.]+$`) })
	envAssignRe       = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`) })
)

// interpreterOf maps a command word to a launcher key (python3.12 is python).
func interpreterOf(word string) string {
	base := filepath.Base(word)
	if m := versionedInterpRe().FindStringSubmatch(base); m != nil {
		base = m[1]
	}
	if base == "python3" {
		base = toolPython
	}
	if _, ok := hookLaunchers[base]; ok {
		return base
	}
	return ""
}

// launcherScript returns the script file a command passes to a common
// interpreter (`bash -e x.sh`, `/usr/bin/env python3 x.py`), or "" when it runs
// inline code, reads stdin, or names a path that cannot be resolved against the
// project (absolute, ~, or containing a variable).
func launcherScript(words []string) string {
	i := 0
	for i < len(words) {
		w := words[i]
		switch {
		case envAssignRe().MatchString(w):
		case filepath.Base(w) == "env":
			for i+1 < len(words) && (strings.HasPrefix(words[i+1], "-") || envAssignRe().MatchString(words[i+1])) {
				i++
				if words[i] == "-u" || words[i] == "-C" || words[i] == "-S" {
					i++
				}
			}
		case w == "nohup" || w == cmdTime || w == "exec" || w == hookTypeCommand:
		default:
			interp := interpreterOf(w)
			if interp == "" {
				return ""
			}
			return scriptArg(hookLaunchers[interp], words[i+1:])
		}
		i++
	}
	return ""
}

func scriptArg(l launcher, args []string) string {
	for j := 0; j < len(args); j++ {
		a := args[j]
		switch {
		case a == "--":
			if j+1 < len(args) {
				return resolvable(args[j+1])
			}
			return ""
		case l.inline[a]:
			return ""
		case l.withArg[a]:
			j++
		case len(a) > 1 && (a[0] == '-' || (l.shell && a[0] == '+')):
			if l.runsInline(a) {
				return ""
			}
		default:
			return resolvable(a)
		}
	}
	return ""
}

// runsInline reports whether the short-option cluster a (-lc, -e) makes the
// interpreter run code from its arguments instead of a script file.
func (l launcher) runsInline(a string) bool {
	if a[0] != '-' || a[1] == '-' {
		return false
	}
	if l.shell && strings.Contains(a[1:], "c") {
		return true
	}
	return l.evalCluster && (strings.HasSuffix(a, "e") || strings.HasSuffix(a, "E"))
}

// resolvable keeps a script word that names a file relative to the project.
func resolvable(s string) string {
	if s == "" || s == "-" || filepath.IsAbs(s) || strings.HasPrefix(s, "/") || strings.ContainsAny(s, "$`~*?") || strings.Contains(s, "://") {
		return ""
	}
	return s
}

// checkLauncherScripts reports (AR501) a script an interpreter is asked to run
// that does not exist. The execute bit is not checked: an interpreter reads the
// file whatever its mode.
func (r *runner) checkLauncherScripts(file, event, command string) {
	for _, seg := range segSplitRe().Split(stripShellComment(command), -1) {
		script := launcherScript(shellWords(strings.TrimSpace(seg)))
		if script == "" {
			continue
		}
		rel := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(script)), "./")
		found := ""
		for _, cand := range []string{rel, joinRel(r.baseRel, rel)} {
			if r.tree.Exists(cand) {
				found = cand
				break
			}
		}
		if found == "" {
			r.add(CodeHookMissing, file, hookLine(r.docs[file], script), "%s hook runs %q, which does not exist", event, script)
			continue
		}
		r.dep(file, filepath.Join(r.tree.Top, filepath.FromSlash(found)))
	}
}
