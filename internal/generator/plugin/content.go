package plugin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
)

// contentLayout describes where a runtime bundles its skills/commands/agents,
// relative to the plugin's content root for that runtime.
type contentLayout struct {
	// Root is the runtime's content root relative to baseDir. Empty means the
	// project root (used by Claude/Kimi, which share top-level skills/ etc.).
	Root string
	// SkillsRoot, when set, is the directory (relative to baseDir) that receives
	// each skill's <name>/ directory, overriding Root/skills. The Pi runtime uses
	// it to write skills wherever pi.skills_dir points.
	SkillsRoot string
	// Skills/Commands/Agents toggle which content kinds this runtime bundles.
	Skills   bool
	Commands bool
	Agents   bool
}

// bundleContent emits verbatim copies of the plugin's skills/commands/agents
// into the runtime's content directories. Content files are passed through byte
// for byte from their source path (frontmatter + body), never re-rendered, so a
// bundled SKILL.md is identical to the authored one. Builtin content (loaded
// from embedded sources, path prefixed with "builtin://") is skipped.
func bundleContent(m *Manifest, baseDir string, layout contentLayout) ([]config.OutputFile, error) {
	var outputs []config.OutputFile
	root := filepath.Join(baseDir, layout.Root)
	skillsRoot := filepath.Join(root, "skills")
	if layout.SkillsRoot != "" {
		skillsRoot = filepath.Join(baseDir, layout.SkillsRoot)
	}

	if layout.Skills {
		for i := range m.Skills {
			skillOutputs, err := passthroughSkill(m, &m.Skills[i], filepath.Join(skillsRoot, m.Skills[i].Name), m.IncludeEvals)
			if err != nil {
				return nil, err
			}
			outputs = append(outputs, skillOutputs...)
		}
		if m.IncludeEvals {
			evalOutputs, err := passthroughEvals(m, filepath.Join(root, config.EvalsDirName))
			if err != nil {
				return nil, err
			}
			outputs = append(outputs, evalOutputs...)
		}
	}
	if layout.Commands {
		for i := range m.Commands {
			out, err := passthroughContent(&m.Commands[i], filepath.Join(root, "commands", m.Commands[i].Name+".md"))
			if err != nil {
				return nil, err
			}
			outputs = appendIf(outputs, out)
		}
	}
	if layout.Agents {
		for i := range m.Agents {
			out, err := passthroughContent(&m.Agents[i], filepath.Join(root, "agents", m.Agents[i].Name+".md"))
			if err != nil {
				return nil, err
			}
			outputs = appendIf(outputs, out)
		}
	}
	return outputs, nil
}

// passthroughSkill copies the complete skill directory so references, scripts,
// and assets used by SKILL.md remain valid in the generated plugin. The skill's
// evals/ directory is copied only when includeEvals is set: eval cases are
// authoring material, not something consumers of the bundle need. Each
// [plugin] shared_resources directory is copied in afterwards, and SKILL.md's
// links to it are rewritten to the in-skill copy. A symlink inside the skill
// directory is followed or refused under the policy plain generate applies to
// skill resources (config.Config.AdmitProjectSymlink).
func passthroughSkill(m *Manifest, content *config.ContentFile, destinationDir string, includeEvals bool) ([]config.OutputFile, error) {
	if content.Path == "" || strings.HasPrefix(content.Path, "builtin://") {
		return nil, nil
	}
	walk := &skillWalk{
		sourceDir:   filepath.Dir(content.Path),
		destination: destinationDir,
		evals:       includeEvals,
		project:     m.Config,
		own:         map[string]bool{},
		visited:     map[string]bool{},
	}
	if walk.project == nil {
		walk.project = &config.Config{BaseDir: m.SourceDir}
	}
	if err := walk.visit(walk.sourceDir, false); err != nil {
		return nil, err
	}
	outputs, own := walk.outputs, walk.own
	if err := appendSharedResources(m, content.Name, destinationDir, own, &outputs); err != nil {
		return nil, err
	}
	rewriteSharedLinks(m, filepath.Join(destinationDir, skillManifestName), outputs)
	return outputs, nil
}

// skillWalk collects the files of one skill directory, following the symlinks
// the project policy admits.
type skillWalk struct {
	sourceDir   string
	destination string
	evals       bool // copy the evals/ directory too
	project     *config.Config
	outputs     []config.OutputFile
	own         map[string]bool // bundle-relative paths collected, slash-separated
	visited     map[string]bool // real paths of the directories being walked, against link cycles
}

// visit collects the files under dir. viaLink is set below a followed directory
// link, whose files live outside the skill directory.
func (w *skillWalk) visit(dir string, viaLink bool) error {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		if w.visited[resolved] {
			return nil // a link cycle inside the project
		}
		w.visited[resolved] = true
		defer delete(w.visited, resolved)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return oops.With("path", dir).Wrapf(err, "walk skill directory")
	}
	for _, entry := range entries {
		if err := w.visitEntry(dir, entry, viaLink); err != nil {
			return err
		}
	}
	return nil
}

func (w *skillWalk) visitEntry(dir string, entry os.DirEntry, viaLink bool) error {
	path := filepath.Join(dir, entry.Name())
	linked := entry.Type()&os.ModeSymlink != 0
	isDir, isFile := entry.IsDir(), entry.Type().IsRegular()
	if linked {
		// The policy plain generate applies to a skill resource: an in-project
		// link is followed, any other is refused with a warning naming the file.
		info, ok := w.project.AdmitProjectSymlink(path)
		if !ok {
			return nil
		}
		isDir, isFile = info.IsDir(), info.Mode().IsRegular()
	}
	switch {
	case isDir:
		return w.visit(path, viaLink || linked)
	case isFile:
		return w.copyFile(path, linked || viaLink)
	}
	return nil
}

// copyFile collects one file. outside is set for a file reached through an
// admitted link: it lives outside the skill directory, inside the project.
func (w *skillWalk) copyFile(path string, outside bool) error {
	relativePath, err := filepath.Rel(w.sourceDir, path)
	if err != nil {
		return oops.With("path", path).Wrapf(err, "resolve skill file path")
	}
	if !w.evals && isEvalsPath(relativePath) {
		return nil
	}
	destination := filepath.Join(w.destination, relativePath)
	var out config.OutputFile
	if outside {
		var info os.FileInfo
		if info, err = os.Stat(path); err != nil {
			return oops.With("path", path).Wrapf(err, "stat symlinked skill file")
		}
		out, err = passthroughLinkedFile(path, destination, info)
	} else {
		out, err = passthroughFile(w.sourceDir, relativePath, destination)
	}
	if err != nil {
		return err
	}
	w.outputs = append(w.outputs, out)
	w.own[filepath.ToSlash(relativePath)] = true
	return nil
}

// appendSharedResources copies each [plugin] shared_resources directory into the
// skill's bundle directory, under a subdirectory named after the shared
// directory. A destination path the skill already owns is an error: silently
// overwriting it would hide which copy a SKILL.md link resolves to.
func appendSharedResources(m *Manifest, skillName, destinationDir string, own map[string]bool, outputs *[]config.OutputFile) error {
	if len(m.SharedResources) == 0 {
		return nil
	}
	for _, resource := range m.SharedResources {
		err := filepath.WalkDir(resource.Dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return oops.With("path", path).Wrapf(walkErr, "walk shared resource directory")
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			relativePath, err := filepath.Rel(resource.Dir, path)
			if err != nil {
				return oops.With("path", path).Wrapf(err, "resolve shared resource path")
			}
			destRel := filepath.Join(resource.Name, relativePath)
			if own[filepath.ToSlash(destRel)] {
				return oops.
					With("skill", skillName).
					With("shared", resource.Name).
					With("path", filepath.ToSlash(destRel)).
					Hint("Rename the skill's own file or the shared file; the bundle must not hold two copies at one path").
					Errorf("skill %q already has %q, which shared_resources %q also provides", skillName, filepath.ToSlash(destRel), resource.Name)
			}
			out, err := passthroughFile(resource.Dir, relativePath, filepath.Join(destinationDir, destRel))
			if err != nil {
				return err
			}
			*outputs = append(*outputs, out)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// rewriteSharedLinks rewrites a bundled SKILL.md's relative links that climb out
// of the skill to a shared_resources directory (../../references/x.md) into the
// copy inside the skill (references/x.md), so the link keeps resolving once the
// skill is installed on its own.
func rewriteSharedLinks(m *Manifest, skillManifestPath string, outputs []config.OutputFile) {
	if len(m.SharedResources) == 0 {
		return
	}
	for i := range outputs {
		if outputs[i].Path != skillManifestPath {
			continue
		}
		outputs[i].RawContent = []byte(rewriteSharedRefs(string(outputBytes(outputs[i])), m.SharedResources))
		outputs[i].Content = ""
	}
}

// rewriteSharedRefs replaces each "../<shared>/" run that points at a shared
// directory with the in-skill copy path "<shared>/".
func rewriteSharedRefs(body string, resources []config.SharedResource) string {
	for _, resource := range resources {
		pattern := regexp.MustCompile(`(?:\.\./)+` + regexp.QuoteMeta(resource.Name) + `/`)
		body = pattern.ReplaceAllString(body, resource.Name+"/")
	}
	return body
}

// passthroughContent reads a content file's source bytes verbatim and returns an
// OutputFile targeting dstPath. Returns a zero OutputFile (skipped) for builtin
// content, which has no on-disk source to copy.
func passthroughContent(cf *config.ContentFile, dstPath string) (config.OutputFile, error) {
	if cf.Path == "" || strings.HasPrefix(cf.Path, "builtin://") {
		return config.OutputFile{}, nil
	}
	data, err := os.ReadFile(cf.Path)
	if err != nil {
		return config.OutputFile{}, oops.With("path", cf.Path).Wrapf(err, "read content source")
	}
	return config.OutputFile{Path: dstPath, RawContent: data}, nil
}

// ensureWithinProject refuses a passthrough source that resolves outside the
// project. The path guards in config.validatePluginPaths are lexical — they
// reject "..", absolute paths and drive letters in the declared string — so a
// symlink escapes them entirely: neither "bootstrap.sh" (linked at ~/.ssh/id_rsa)
// nor "vendor/passwd" (where vendor links to /etc) contains a traversal
// sequence. os.Stat and os.ReadFile then follow the link.
//
// That matters because passthrough bytes are published: they land in a plugin
// bundle that consumers install, and a hook script is executed by the installing
// runtime. Whoever builds the bundle would be exfiltrating a local file into it.
// config.LoadResources already refuses symlinked skill resources for the same
// reason; this is the bundling counterpart of that defense.
//
// Symlinks that stay inside the project are allowed — they are an ordinary
// repository layout, and the bytes were publishable either way.
func ensureWithinProject(sourceDir, abs string) error {
	// Resolve the root too: a project legitimately living under a symlinked
	// path (/tmp -> /private/tmp on macOS) would otherwise fail every check.
	root, err := filepath.EvalSymlinks(sourceDir)
	if err != nil {
		return oops.With("path", sourceDir).Wrapf(err, "resolve project directory")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return oops.With("path", abs).Wrapf(err, "resolve passthrough file")
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || safefs.RelEscapes(rel) {
		return oops.
			With("path", abs).
			With("resolved_to", resolved).
			Hint("Point it at a file inside the project; a symlink out of the project would copy that file into the published bundle").
			Errorf("passthrough file resolves outside the project")
	}
	return nil
}

// passthroughFile reads srcPath (resolved relative to the source project when
// not absolute) and returns an OutputFile writing its bytes verbatim to dstPath,
// preserving the executable bit for scripts.
func passthroughFile(sourceDir, srcPath, dstPath string) (config.OutputFile, error) {
	abs := srcPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(sourceDir, srcPath)
	}
	if err := ensureWithinProject(sourceDir, abs); err != nil {
		return config.OutputFile{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return config.OutputFile{}, oops.With("path", abs).Wrapf(err, "stat passthrough file")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return config.OutputFile{}, oops.With("path", abs).Wrapf(err, "read passthrough file")
	}
	return config.OutputFile{Path: dstPath, RawContent: data, Mode: info.Mode().Perm()}, nil
}

// passthroughLinkedFile reads the file a symlink that the project policy admitted
// resolves to, and returns an OutputFile writing its bytes verbatim to dstPath
// with the target's mode. The caller has already checked the target is inside the
// project, so unlike passthroughFile no further containment check is made here.
func passthroughLinkedFile(path, dstPath string, info os.FileInfo) (config.OutputFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.OutputFile{}, oops.With("path", path).Wrapf(err, "read symlinked skill file")
	}
	return config.OutputFile{Path: dstPath, RawContent: data, Mode: info.Mode().Perm()}, nil
}

// appendIf appends out to outputs unless out is the zero value (a skipped file).
func appendIf(outputs []config.OutputFile, out config.OutputFile) []config.OutputFile {
	if out.Path == "" {
		return outputs
	}
	return append(outputs, out)
}

// isEvalsPath reports whether a skill-relative path is inside its evals/ dir.
func isEvalsPath(relativePath string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(relativePath), "/")
	return first == config.SkillKindEvals
}

// passthroughEvals copies the project-level eval tree to destinationDir. A
// missing tree is not an error. When the manifest limits evals to its own
// skills, only the <skill-name>/ subdirectories of bundled skills are copied.
func passthroughEvals(m *Manifest, destinationDir string) ([]config.OutputFile, error) {
	if m.EvalsDir == "" {
		return nil, nil
	}
	info, err := os.Stat(m.EvalsDir)
	if err != nil || !info.IsDir() {
		return nil, nil //nolint:nilerr // an absent eval tree simply contributes nothing
	}
	bundled := make(map[string]bool, len(m.Skills))
	for i := range m.Skills {
		bundled[m.Skills[i].Name] = true
	}

	var outputs []config.OutputFile
	err = filepath.WalkDir(m.EvalsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return oops.With("path", path).Wrapf(walkErr, "walk eval directory")
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relativePath, err := filepath.Rel(m.EvalsDir, path)
		if err != nil {
			return oops.With("path", path).Wrapf(err, "resolve eval file path")
		}
		if m.EvalsPerSkillOnly {
			skill, rest, nested := strings.Cut(filepath.ToSlash(relativePath), "/")
			if !nested || rest == "" || !bundled[skill] {
				return nil
			}
		}
		out, err := passthroughFile(m.EvalsDir, relativePath, filepath.Join(destinationDir, relativePath))
		if err != nil {
			return err
		}
		outputs = append(outputs, out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return outputs, nil
}
