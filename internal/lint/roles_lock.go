package lint

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// LockDrift is one difference between the sources or outputs and ai-rulez.lock.
// The caller computes it (the lint package must not import the generator) and
// passes it in only when a lock exists and [lock] enforce is set.
type LockDrift struct {
	// Output is true for a generated-output difference (AR982), false for an
	// authored-source difference (AR981).
	Output bool
	// Path is the item's source path, or the output path, relative to the repo.
	Path string
	// Message is a complete sentence.
	Message string
}

// WithLockDrift supplies the lock drift to report as AR981 and AR982.
func WithLockDrift(drift []LockDrift) Option {
	return func(r *runner) { r.lockDrift = drift }
}

// configLine finds the line of the first config line containing needle, for
// anchoring a finding that has no file of its own.
func (r *runner) configLine(needle string) (path string, line int) {
	path = r.configFilePath()
	if path == "" {
		path = filepath.Join(r.rootAbs(), ".ai-rulez", "config.toml")
	}
	line = 1
	data, err := os.ReadFile(path)
	if err != nil {
		return path, line
	}
	lines := strings.Split(string(data), "\n")
	if _, ok := r.docs[path]; !ok {
		r.docs[path] = doc{lines: lines}
	}
	for i, l := range lines {
		if strings.Contains(l, needle) {
			return path, i + 1
		}
	}
	return path, line
}

// checkRoles reports role problems (AR971 to AR973).
func (r *runner) checkRoles() {
	for _, p := range r.cfg.RoleProblems() {
		code := CodeRoleReferenceUnknown
		switch p.Kind {
		case config.RoleProblemExtends:
			code = CodeRoleExtendsInvalid
		case config.RoleProblemUnreachable:
			code = CodeRoleUnreachable
		}
		path, line := r.configLine(`name = "` + p.Role + `"`)
		r.add(code, path, line, "%s", p.Message)
	}
}

// checkLockDrift reports content that no longer matches ai-rulez.lock.
func (r *runner) checkLockDrift() {
	for _, d := range r.lockDrift {
		code := CodeLockSourceDrift
		if d.Output {
			code = CodeLockOutputDrift
		}
		abs := d.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(r.rootAbs(), filepath.FromSlash(d.Path))
		}
		r.add(code, abs, 1, "%s", d.Message)
	}
}
