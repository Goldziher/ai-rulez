package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
)

// The scoped write path of create_skill and update_skill. With scope set, the
// tools write <scope>/<harness skills directory>/<name>/SKILL.md in the
// repository, where a harness working in that directory loads it. That is the
// harness's own file: it is not ai-rulez content and is not generated down.
// Instruction files and rules stay read-only over MCP.
//
// A scoped write is refused unless the caller passes allow_write, the scope is
// an existing directory of the project reached through no symlink, the harness
// is a preset the configuration enables, the target is not a generated copy,
// and the content passes the security scan at the strictest trust level.

const (
	// scopedMode is the mode of a written skill file.
	scopedMode = 0o644

	argDomain = "domain"
)

// scopedSkillHandler routes a create_skill or update_skill call: with scope it
// writes the harness's skill file, without it the tool edits ai-rulez content.
func (s *Server) scopedSkillHandler(create bool, content handlerFunc) handlerFunc {
	return func(ctx context.Context, req *handlers.ToolRequest) (*sdkmcp.CallToolResult, error) {
		if req.GetString("scope", "") == "" {
			return content(ctx, req)
		}
		out, err := s.writeScopedSkill(ctx, req, create)
		if err != nil {
			return handlers.ToolError(err)
		}
		return handlers.ToolSuccess(out)
	}
}

type scopedTarget struct {
	cfg     *config.Config
	scope   string
	harness string
	name    string
	// skillDir and file are project-relative, slash separated; below is skillDir
	// without the scope.
	skillDir, file, below string
}

// writeScopedSkill checks a scoped write against every rule and performs it.
func (s *Server) writeScopedSkill(ctx context.Context, req *handlers.ToolRequest, create bool) (map[string]any, error) {
	if err := checkScopedRequest(req); err != nil {
		return nil, err
	}
	t, err := resolveScopedTarget(ctx, req)
	if err != nil {
		return nil, err
	}
	s.scopedMu.Lock()
	defer s.scopedMu.Unlock()

	root, err := os.OpenRoot(t.cfg.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "open the project root")
	}
	defer root.Close() //nolint:errcheck // read handle; writes are closed individually
	body, err := t.prepare(root, req, create)
	if err != nil {
		return nil, err
	}
	if err := writeInRoot(root, t.file, body); err != nil {
		return nil, err
	}
	op := "update_skill"
	if create {
		op = "create_skill"
	}
	out := map[string]any{
		keyOK: true, keyOp: op, argPath: t.file, "name": t.name, "scope": t.scope, "harness": t.harness,
		"message": "Skill written to " + t.file + "; it is the harness's file, not ai-rulez content",
	}
	if lockPresent(t.cfg) {
		out["lock"] = "ai-rulez.lock exists: the new file is reported as drift until you run `ai-rulez lock`"
	}
	return out, nil
}

// checkScopedRequest refuses what a scoped write cannot take: ai-rulez content
// arguments, and a call without the consent flag.
func checkScopedRequest(req *handlers.ToolRequest) error {
	// These arguments address ai-rulez content, which a scoped write does not touch.
	for _, arg := range []string{argDomain, "priority", "targets", "local"} {
		if v, given := req.GetArguments()[arg]; given && !isZeroArg(v) {
			return oops.Errorf("%s is for ai-rulez content and cannot be combined with scope", arg)
		}
	}
	if !req.GetBool("allow_write", false) {
		return oops.
			Hint("Pass allow_write: true to confirm writing a harness file into the repository").
			Errorf("writing a skill into scope %q needs allow_write", req.GetString("scope", ""))
	}
	return nil
}

// prepare runs every check that precedes the write and returns the file body.
// Nothing is created before all of them pass.
func (t *scopedTarget) prepare(root *os.Root, req *handlers.ToolRequest, create bool) ([]byte, error) {
	if err := checkScopeDirectory(root, t.scope); err != nil {
		return nil, err
	}
	existing, exists, err := checkTargetPath(root, t)
	if err != nil {
		return nil, err
	}
	if t.generatedCopy(existing) {
		return nil, oops.
			Hint("Edit the source under .ai-rulez/ and run ai-rulez generate").
			Errorf("%s is a generated copy: ai-rulez wrote it and would overwrite the change", t.file)
	}
	switch {
	case create && exists:
		return nil, oops.Hint("Use update_skill to replace it").Errorf("%s already exists", t.file)
	case !create && !exists:
		return nil, oops.Hint("Use create_skill to add it").Errorf("the skill %s does not exist", t.file)
	}
	body, err := scopedContent(req, t, create)
	if err != nil {
		return nil, err
	}
	if err := scanBeforeWrite(t, body); err != nil {
		return nil, err
	}
	return body, nil
}

const (
	keyOK = "success"
	keyOp = "operation"
)

func isZeroArg(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case []any:
		return len(x) == 0
	}
	return false
}

// resolveScopedTarget validates the names and finds the harness directory.
func resolveScopedTarget(ctx context.Context, req *handlers.ToolRequest) (*scopedTarget, error) {
	name := req.GetString("name", "")
	if !validSkillSegment(name) {
		return nil, oops.Errorf("the skill name %q is not a valid name: use letters, digits, dots, dashes and underscores", name)
	}
	scope, err := repoindex.CleanPath(req.GetString("scope", ""))
	if err != nil {
		return nil, err //nolint:wrapcheck // names the scope
	}
	wd := req.GetString(argWorkingDirectory, "")
	if wd == "" {
		wd = "."
	}
	abs, err := filepath.Abs(wd)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve the working directory")
	}
	cfg, err := config.LoadConfig(ctx, abs, config.WithoutRemote())
	if err != nil {
		return nil, oops.Wrapf(err, "load the project configuration")
	}
	harness, dir, err := scopedHarness(cfg, req.GetString("harness", ""))
	if err != nil {
		return nil, err
	}
	below := path.Join(dir, name)
	skillDir := path.Join(scope, below)
	return &scopedTarget{cfg: cfg, scope: scope, harness: harness, name: name, below: below, skillDir: skillDir, file: path.Join(skillDir, skillMarkdown)}, nil
}

// scopedHarness picks the enabled preset whose skills directory is written: the
// named one, else the first enabled one that has such a directory.
func scopedHarness(cfg *config.Config, wanted string) (harness, dir string, err error) {
	var enabled []string
	for i := range cfg.Presets {
		enabled = append(enabled, cfg.Presets[i].GetName())
	}
	for _, name := range enabled {
		if wanted != "" && name != wanted {
			continue
		}
		if dirs := importer.SkillDirectories([]string{name}); len(dirs) > 0 {
			return name, dirs[0], nil
		}
	}
	if wanted != "" {
		return "", "", oops.
			Hint("The enabled presets are "+strings.Join(enabled, ", ")).
			Errorf("the harness %q is not enabled in the configuration or keeps no skills directory", wanted)
	}
	return "", "", oops.Errorf("no preset enabled in the configuration keeps skills in a directory")
}

// checkScopeDirectory requires the scope to be an existing directory below the
// project root that no symlink leads through.
func checkScopeDirectory(root *os.Root, scope string) error {
	if scope == repoindex.RootScope {
		return nil
	}
	cur := ""
	for _, part := range strings.Split(scope, "/") {
		cur = path.Join(cur, part)
		info, err := root.Lstat(filepath.FromSlash(cur))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return oops.Errorf("the scope %q does not exist in the project", scope)
		case err != nil:
			return oops.Wrapf(err, "check the scope %q", scope)
		case info.Mode()&fs.ModeSymlink != 0:
			return oops.Errorf("%s is a symlink: a scoped write does not follow symlinks", cur)
		case !info.IsDir():
			return oops.Errorf("the scope %q is not a directory (%s)", scope, cur)
		}
	}
	return nil
}

// checkTargetPath walks the part of the target that may exist: no component may
// be a symlink. It returns the current content of the skill file, nil when
// there is none, and whether it exists.
func checkTargetPath(root *os.Root, t *scopedTarget) (existing []byte, exists bool, err error) {
	rest := t.below
	cur := t.scope
	if cur == repoindex.RootScope {
		cur = ""
	}
	exists = true
	for _, part := range strings.Split(rest, "/") {
		cur = path.Join(cur, part)
		info, lerr := root.Lstat(filepath.FromSlash(cur))
		if errors.Is(lerr, fs.ErrNotExist) {
			exists = false
			break
		}
		if lerr != nil {
			return nil, false, oops.Wrapf(lerr, "check %s", cur)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, false, oops.Errorf("%s is a symlink: a scoped write does not follow symlinks", cur)
		}
		if !info.IsDir() {
			return nil, false, oops.Errorf("%s is not a directory", cur)
		}
	}
	if exists {
		info, lerr := root.Lstat(filepath.FromSlash(t.file))
		switch {
		case errors.Is(lerr, fs.ErrNotExist):
			exists = false
		case lerr != nil:
			return nil, false, oops.Wrapf(lerr, "check %s", t.file)
		case info.Mode()&fs.ModeSymlink != 0:
			return nil, false, oops.Errorf("%s is a symlink: a scoped write does not follow symlinks", t.file)
		case !info.Mode().IsRegular():
			return nil, false, oops.Errorf("%s is not a regular file", t.file)
		default:
			if existing, err = root.ReadFile(filepath.FromSlash(t.file)); err != nil {
				return nil, false, oops.Wrapf(err, "read %s", t.file)
			}
		}
	}
	return existing, exists, nil
}

// generatedCopy reports whether the target is an output ai-rulez wrote: listed
// in a generated manifest (for a skill, any file of its directory), or carrying
// a Content-Hash header that verifies.
func (t *scopedTarget) generatedCopy(existing []byte) bool {
	d := &discovery{cfg: t.cfg, generated: map[string]bool{}}
	for _, p := range generator.NewGenerator(t.cfg).GeneratedPaths() {
		d.generated[filepath.Clean(p)] = true
	}
	return d.isGenerated(filepath.Join(t.cfg.BaseDir, filepath.FromSlash(t.file)), existing)
}

// scopedContent is the file body: the content given, else a skill built from
// the description (create only).
func scopedContent(req *handlers.ToolRequest, t *scopedTarget, create bool) ([]byte, error) {
	content := req.GetString("content", "")
	if strings.TrimSpace(content) != "" {
		return []byte(content), nil
	}
	if !create {
		return nil, oops.Errorf("update_skill needs the new content")
	}
	desc := strings.TrimSpace(req.GetString("description", ""))
	if desc == "" {
		return nil, oops.Errorf("give the skill content, or a description to build its SKILL.md from")
	}
	if strings.ContainsAny(desc, "\r\n") {
		return nil, oops.Errorf("the description must be one line")
	}
	return []byte(fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n\n%s\n", t.name, desc, t.name, desc)), nil
}

// scanBeforeWrite runs the security scan install_skill gives a skill, at the
// strictest trust level: content an agent writes is not content its author vouched for.
func scanBeforeWrite(t *scopedTarget, body []byte) error {
	findings := lint.ScanServed(t.cfg, t.name, []lint.ServedFile{{Path: skillMarkdown, Content: body}}, config.TrustError)
	if len(findings) == 0 {
		return nil
	}
	parts := make([]string, 0, len(findings))
	for i, f := range findings {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more", len(findings)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("%s %s at line %d", f.Code, f.Name, f.Line))
	}
	return oops.Errorf("refused by the security scan: %s", strings.Join(parts, "; "))
}

// writeInRoot writes rel (slash separated, below root) with a temporary file
// and a rename, creating the missing directories. Every step goes through the
// os.Root, so a link swapped in after the checks cannot lead out of the project.
func writeInRoot(root *os.Root, rel string, body []byte) error {
	dir := filepath.FromSlash(path.Dir(rel))
	cur := ""
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		if err := root.Mkdir(cur, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return oops.Wrapf(err, "create %s", cur)
		}
	}
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return oops.Wrapf(err, "name a temporary file")
	}
	tmp := filepath.Join(dir, ".SKILL.md.tmp-"+hex.EncodeToString(suffix[:]))
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, scopedMode)
	if err != nil {
		return oops.Wrapf(err, "create a temporary file for %s", rel)
	}
	_, writeErr := f.Write(body)
	chmodErr := f.Chmod(scopedMode)
	closeErr := f.Close()
	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		_ = root.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return oops.Wrapf(err, "write %s", rel)
	}
	if err := root.Rename(tmp, filepath.FromSlash(rel)); err != nil {
		_ = root.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return oops.Wrapf(err, "replace %s", rel)
	}
	return nil
}

// lockPresent reports whether the project has an ai-rulez.lock.
func lockPresent(cfg *config.Config) bool {
	dir := cfg.ConfigDir
	if dir == "" {
		dir = cfg.BaseDir
	}
	_, err := os.Stat(filepath.Join(dir, lockfile.FileName))
	return err == nil
}
