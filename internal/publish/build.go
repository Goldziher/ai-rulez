package publish

import (
	"bytes"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/samber/oops"
)

// Dist file names.
const (
	SumsFile  = "SHA256SUMS"
	PlanFile  = "publish-plan.json"
	LockFile  = "ai-rulez.lock"
	NotesFile = "RELEASE_NOTES.md"
	EmitDir   = "emit"
	// TargetGitHubRelease is the only upload target of this release.
	TargetGitHubRelease = "github-release"
)

var (
	namePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]*$`)
	tagPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	repoPattern    = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9.-]*/)?[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9_.][A-Za-z0-9_.-]*$`)
)

// Template is an operator-supplied text/template rendered into dist/emit.
type Template struct {
	// Name is the file name; a trailing ".tmpl" is dropped from the output name.
	Name string
	Body string
}

// Input is everything Build needs; nothing is read from the environment.
type Input struct {
	Name, Version  string
	AIRulezVersion string
	Runtimes       []string
	Files          []File
	Lock           []byte
	LockVersion    int
	LockTree       string
	Source         Source
	// Mtime is the fixed modification time (Unix seconds) of every archive entry.
	Mtime int64
	// Target is "" (build only) or TargetGitHubRelease.
	Target string
	// Tag and Repo are used by the github-release target.
	Tag, Repo string
	Templates []Template
}

// Artifact is one dist file in the plan.
type Artifact struct {
	Path   string `json:"path"`
	Role   string `json:"role"`
	Digest string `json:"digest"`
	Size   int    `json:"size"`
}

// Command is one process --execute starts: a fixed argv, no shell.
type Command struct {
	Argv []string `json:"argv"`
	Cwd  string   `json:"cwd"`
}

// Step is the outcome of one preflight gate. Only passed gates reach a plan.
type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Plan is publish-plan.json: the artifacts with their digests and the commands
// --execute would run. It holds no timestamps and no local paths.
type Plan struct {
	SchemaVersion int        `json:"schema_version"`
	Name          string     `json:"name"`
	Version       string     `json:"version"`
	Target        string     `json:"target,omitempty"`
	Tag           string     `json:"tag,omitempty"`
	Repo          string     `json:"repo,omitempty"`
	Preflight     []Step     `json:"preflight"`
	Artifacts     []Artifact `json:"artifacts"`
	Upload        []string   `json:"upload,omitempty"`
	Commands      []Command  `json:"commands"`
	Credentials   string     `json:"credentials,omitempty"`
}

// Dist is a built, not yet written, dist directory.
type Dist struct {
	Manifest Manifest
	Plan     Plan
	// Files maps a dist-relative slash path to its bytes.
	Files map[string][]byte
}

// Paths returns the dist-relative paths in sorted order.
func (d *Dist) Paths() []string {
	out := make([]string, 0, len(d.Files))
	for p := range d.Files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ValidateName checks the plugin name and version, which become file names.
func ValidateName(name, version string) error {
	if name == "" || !namePattern.MatchString(name) {
		return newError(CodeSource, ExitGate, "set a file-name safe [plugin] name (letters, digits, '.', '_', '-')", "plugin name %q cannot name release files", name)
	}
	if version == "" {
		return newError(CodeSource, ExitGate, "set [plugin] version; semver bumps stay manual", "[plugin] version is not set")
	}
	if !versionPattern.MatchString(version) {
		return newError(CodeSource, ExitGate, "", "plugin version %q cannot name release files", version)
	}
	return nil
}

// ValidateTarget checks --tag and --repo before they reach an argv.
func ValidateTarget(tag, repo string) error {
	if !tagPattern.MatchString(tag) || strings.Contains(tag, "..") || strings.HasSuffix(tag, "/") {
		return newError(CodeTarget, ExitFailed, "", "invalid release tag %q", tag)
	}
	if !repoPattern.MatchString(repo) {
		return newError(CodeTarget, ExitFailed, "pass --repo OWNER/REPO", "cannot publish to %q: not an OWNER/REPO (or HOST/OWNER/REPO) repository", repo)
	}
	return nil
}

// Build assembles the dist directory in memory.
func Build(in Input) (*Dist, error) {
	if err := ValidateName(in.Name, in.Version); err != nil {
		return nil, err
	}
	if in.Target != "" && in.Target != TargetGitHubRelease {
		return nil, newError(CodeTarget, ExitFailed, "only github-release is supported", "unknown target %q", in.Target)
	}
	if in.Target == TargetGitHubRelease {
		if err := ValidateTarget(in.Tag, in.Repo); err != nil {
			return nil, err
		}
	}
	if len(in.Lock) == 0 {
		return nil, newError(CodePreflight, ExitGate, "run `ai-rulez lock`", "ai-rulez.lock is missing or empty")
	}
	archive, err := BuildArchive(in.Files, in.Mtime)
	if err != nil {
		return nil, err
	}
	base := in.Name + "-" + in.Version
	bundleName, manifestName := base+".tar.gz", base+".manifest.json"

	entries := make([]FileEntry, 0, len(in.Files))
	for _, f := range in.Files {
		entries = append(entries, FileEntry{Path: f.Path, Size: len(f.Data), Digest: Digest(f.Data)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	runtimes := append([]string{}, in.Runtimes...)
	sort.Strings(runtimes)
	manifest := Manifest{
		SchemaVersion: SchemaVersion, Name: in.Name, Version: in.Version, AIRulezVer: in.AIRulezVersion,
		Source:   in.Source,
		Lock:     LockInfo{Version: in.LockVersion, Tree: in.LockTree, FileDigest: Digest(in.Lock)},
		Runtimes: runtimes, Files: entries,
		Bundle: BundleInfo{File: bundleName, Digest: Digest(archive), Size: len(archive)},
	}
	manifestBytes, err := manifest.Marshal()
	if err != nil {
		return nil, err
	}
	d := &Dist{Manifest: manifest, Files: map[string][]byte{
		bundleName: archive, manifestName: manifestBytes, LockFile: in.Lock,
	}}
	roles := map[string]string{bundleName: "bundle", manifestName: "manifest", LockFile: "lock"}
	emitted, err := renderTemplates(in, manifest)
	if err != nil {
		return nil, err
	}
	for name, data := range emitted {
		d.Files[name] = data
		roles[name] = "emitted"
	}
	d.Files[NotesFile] = releaseNotes(in, manifest)
	roles[NotesFile] = "notes"

	sums := make([]SumEntry, 0, len(d.Files))
	for p, data := range d.Files {
		sums = append(sums, SumEntry{Path: p, Digest: Digest(data)})
	}
	d.Files[SumsFile] = FormatSums(sums)
	roles[SumsFile] = "checksums"

	d.Plan = buildPlan(in, d, roles, bundleName, manifestName)
	planBytes, err := marshalJSON(d.Plan)
	if err != nil {
		return nil, err
	}
	d.Files[PlanFile] = planBytes
	return d, nil
}

func buildPlan(in Input, d *Dist, roles map[string]string, bundleName, manifestName string) Plan {
	plan := Plan{
		SchemaVersion: SchemaVersion, Name: in.Name, Version: in.Version, Target: in.Target,
		Preflight: []Step{{"validate-strict", "ok"}, {"lock-check", "ok"}, {"verify-plugin", "ok"}, {"secret-scan", "ok"}},
		Commands:  []Command{},
	}
	paths := d.Paths()
	for _, p := range paths {
		plan.Artifacts = append(plan.Artifacts, Artifact{Path: p, Role: roles[p], Digest: Digest(d.Files[p]), Size: len(d.Files[p])})
	}
	if in.Target == TargetGitHubRelease {
		plan.Tag, plan.Repo = in.Tag, in.Repo
		plan.Upload = []string{bundleName, manifestName, LockFile, SumsFile}
		plan.Commands = []Command{{Argv: ReleaseCreateArgv(in.Name, in.Version, in.Tag, in.Repo, plan.Upload), Cwd: "."}}
		plan.Credentials = "gh authentication (GH_TOKEN or `gh auth login`); not read by ai-rulez"
	}
	return plan
}

func releaseNotes(in Input, m Manifest) []byte {
	var sb strings.Builder
	sb.WriteString("# " + in.Name + " " + in.Version + "\n\n")
	sb.WriteString("- Bundle: `" + m.Bundle.File + "` (" + m.Bundle.Digest + ")\n")
	sb.WriteString("- Runtimes: " + strings.Join(m.Runtimes, ", ") + "\n")
	sb.WriteString("- Lock tree: " + m.Lock.Tree + "\n")
	if in.Source.Commit != "" {
		sb.WriteString("- Source commit: " + in.Source.Commit + "\n")
	}
	sb.WriteString("\nVerify the download with `ai-rulez publish verify <dir>` or `sha256sum -c SHA256SUMS`.\n")
	return []byte(sb.String())
}

// templateData is what an emitter template sees: plain values, no methods.
type templateData struct {
	Name, Version, Tag, Repo, Commit, AIRulezVersion string
	Runtimes                                         []string
	BundleFile, BundleDigest, LockTree, LockDigest   string
	BundleSize                                       int
	Files                                            []FileEntry
}

func renderTemplates(in Input, m Manifest) (map[string][]byte, error) {
	out := map[string][]byte{}
	data := templateData{
		Name: m.Name, Version: m.Version, Tag: in.Tag, Repo: in.Repo, Commit: m.Source.Commit,
		AIRulezVersion: m.AIRulezVer, Runtimes: m.Runtimes, BundleFile: m.Bundle.File,
		BundleDigest: m.Bundle.Digest, BundleSize: m.Bundle.Size, LockTree: m.Lock.Tree,
		LockDigest: m.Lock.FileDigest, Files: m.Files,
	}
	funcs := template.FuncMap{"json": func(v any) (string, error) {
		b, err := marshalJSON(v)
		return strings.TrimSuffix(string(b), "\n"), err
	}}
	for _, t := range in.Templates {
		name := strings.TrimSuffix(t.Name, ".tmpl")
		if name == "" || strings.ContainsAny(name, "/\\") || !ValidPath(name) {
			return nil, newError(CodeBundleUnsafe, ExitFailed, "", "template %q has no usable output name", t.Name)
		}
		dest := EmitDir + "/" + name
		if _, dup := out[dest]; dup {
			return nil, newError(CodeBundleUnsafe, ExitFailed, "", "two templates render to %s", dest)
		}
		tpl, err := template.New(t.Name).Funcs(funcs).Option("missingkey=error").Parse(t.Body)
		if err != nil {
			return nil, oops.With("template", t.Name).Wrapf(err, "parse template")
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, data); err != nil {
			return nil, oops.With("template", t.Name).Wrapf(err, "render template")
		}
		out[dest] = buf.Bytes()
	}
	return out, nil
}
