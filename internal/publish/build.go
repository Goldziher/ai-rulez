package publish

import (
	"bytes"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
)

// Dist file names.
const (
	SumsFile  = "SHA256SUMS"
	PlanFile  = "publish-plan.json"
	LockFile  = "ai-rulez.lock"
	NotesFile = "RELEASE_NOTES.md"
	EmitDir   = "emit"
)

// Upload targets.
const (
	TargetGitHubRelease = "github-release"
	TargetNPM           = "npm"
	TargetOCI           = "oci"
)

// Targets lists every upload target, in the order documented.
var Targets = []string{TargetGitHubRelease, TargetNPM, TargetOCI}

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

// EmitRequest asks for emitters (internal/publish/emit) to run.
type EmitRequest struct {
	// Names are the emitters, by name; duplicates run once.
	Names []string
	// Experimental allows an emitter whose format is not verified against vendor
	// documentation. Without it such an emitter is refused.
	Experimental bool
	// Base is everything the emitters read besides the plugins, which Build adds.
	Base emit.Input
	// Options are per-emitter settings, by emitter name.
	Options map[string]map[string]string
}

// Input is everything Build needs; nothing is read from the environment.
type Input struct {
	Name, Version  string
	Description    string
	AIRulezVersion string
	Runtimes       []string
	Files          []File
	Lock           []byte
	LockVersion    int
	LockTree       string
	Source         Source
	// Mtime is the fixed modification time (Unix seconds) of every archive entry.
	Mtime int64
	// Target is "" (build only) or one of Targets.
	Target string
	// Tag and Repo are used by the github-release target; Repo also names the
	// repository a pinned marketplace points at.
	Tag, Repo string
	// Channel names the release channel; it selects the pinned index directory
	// and is the npm dist-tag.
	Channel string
	NPM     NPMOptions
	// OCIRepository is "host/path" without a tag; the tag is the version.
	OCIRepository string
	Templates     []Template
	// Pin asks for a marketplace index pinned to the release commit.
	Pin  *Pin
	Emit *EmitRequest
	// PreviousLock is the lock of the previous release, for the release notes;
	// PreviousLabel names it (the tag it came from).
	PreviousLock  []byte
	PreviousLabel string
	// PreviousExplicit is set when the previous release was named (--since): a
	// lock that cannot be parsed is then an error, not a note.
	PreviousExplicit bool
	// RequireSignature makes Build fail unless Sign is set (AR9N7).
	RequireSignature bool
	// Sign signs the release archive and the statement binding its name,
	// version and digests; nil builds an unsigned release.
	Sign func(SignRequest) (*SignResult, error)
	// SBOM is the CycloneDX document to ship; empty ships none.
	SBOM     []byte
	Approval *ApprovalInfo
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
	Channel       string     `json:"channel,omitempty"`
	Tag           string     `json:"tag,omitempty"`
	Repo          string     `json:"repo,omitempty"`
	Ref           string     `json:"ref,omitempty"`
	OCIDigest     string     `json:"oci_digest,omitempty"`
	NPM           *NPMPlan   `json:"npm,omitempty"`
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
	// Warnings are notes for the operator (an experimental emitter, a field a
	// format cannot express); they never fail the build.
	Warnings []string
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

// ValidSince reports whether --since is a plain tag name: it is passed to git as
// part of a revision, so an option, a range or a revision expression is refused.
func ValidSince(tag string) bool {
	return tagPattern.MatchString(tag) && !strings.Contains(tag, "..") && !strings.HasSuffix(tag, "/")
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

// targetPlan is what validating a target resolves: the npm package of the npm
// target, or the reference of the oci target.
type targetPlan struct {
	npm    *NPMPlan
	ociRef string
}

// validateInput checks the target-independent and target-specific fields.
func validateInput(in Input) (targetPlan, error) {
	if err := ValidateName(in.Name, in.Version); err != nil {
		return targetPlan{}, err
	}
	if in.Channel != "" && !ValidChannel(in.Channel) {
		return targetPlan{}, newError(CodeConfig, ExitFailed, "channels are lower-case letters, digits and '-'", "invalid channel name %q", in.Channel)
	}
	if len(in.Lock) == 0 {
		return targetPlan{}, newError(CodePreflight, ExitGate, "run `ai-rulez lock`", "ai-rulez.lock is missing or empty")
	}
	if in.RequireSignature && in.Sign == nil {
		return targetPlan{}, newError(CodeUnsigned, ExitGate, "sign with --sign-key FILE or --sign-keyless", "require_signature is set and the bundle is not being signed")
	}
	switch in.Target {
	case "":
		return targetPlan{}, nil
	case TargetGitHubRelease:
		return targetPlan{}, ValidateTarget(in.Tag, in.Repo)
	case TargetNPM:
		if in.RequireSignature {
			return targetPlan{}, newError(CodeUnsigned, ExitGate, "publish the signed archive with --to github-release or --to oci, or drop require_signature",
				"require_signature is set, but the npm tarball that --to npm publishes is packed by npm and carries no signature")
		}
		plan, err := ValidateNPM(in.NPM, in.Name, in.Version)
		if err != nil {
			return targetPlan{}, err
		}
		if strings.Contains(in.Version, "-") && in.Channel == "" {
			return targetPlan{}, newError(CodeConfig, ExitFailed, "pass --channel NAME (the npm dist-tag), such as next",
				"npm refuses to publish the prerelease %s without a dist-tag", in.Version)
		}
		plan.Tag = in.Channel
		return targetPlan{npm: &plan}, nil
	case TargetOCI:
		ref, err := ociReference(in.OCIRepository, in.Version)
		return targetPlan{ociRef: ref}, err
	}
	return targetPlan{}, newError(CodeConfig, ExitFailed, "use one of: "+strings.Join(Targets, ", "), "unknown target %q", in.Target)
}

// Build assembles the dist directory in memory.
func Build(in Input) (*Dist, error) {
	tp, err := validateInput(in)
	if err != nil {
		return nil, err
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
		Bundle:   BundleInfo{File: bundleName, Digest: Digest(archive), Size: len(archive)},
		Approval: in.Approval,
	}
	d := &Dist{Files: map[string][]byte{bundleName: archive, LockFile: in.Lock}}
	roles := map[string]string{bundleName: "bundle", manifestName: "manifest", LockFile: "lock"}

	if len(in.SBOM) > 0 {
		sbomName := base + ".sbom.cdx.json"
		manifest.SBOM = &SBOMInfo{Format: SBOMCycloneDX, File: sbomName, Digest: Digest(in.SBOM)}
		d.Files[sbomName], roles[sbomName] = in.SBOM, "sbom"
	}
	if in.Sign != nil {
		st, err := ReleaseStatement(manifest)
		if err != nil {
			return nil, err
		}
		sig, err := in.Sign(SignRequest{Archive: archive, Statement: st})
		if err != nil {
			return nil, oops.Wrapf(err, "sign the release archive")
		}
		sigName := bundleName + ".sigstore.json"
		manifest.Signature = &SignatureInfo{Type: SignatureSigstoreBundle, File: sigName, Signer: sig.Signer}
		d.Files[sigName], roles[sigName] = sig.Bundle, "signature"
		if len(sig.Attestation) > 0 {
			attName := base + ".attestation.sigstore.json"
			manifest.Signature.Attestation = attName
			d.Files[attName], roles[attName] = sig.Attestation, "attestation"
		}
	}
	manifestBytes, err := manifest.Marshal()
	if err != nil {
		return nil, err
	}
	d.Manifest = manifest
	d.Files[manifestName] = manifestBytes

	if err := addExtras(d, roles, in, manifest); err != nil {
		return nil, err
	}
	if err := addTargetFiles(d, roles, in, manifestBytes, tp); err != nil {
		return nil, err
	}
	if in.Target == TargetNPM && in.Sign != nil {
		d.Warnings = append(d.Warnings, "the signature covers the release archive, not the npm tarball that --to npm publishes")
	}
	notes, noteWarnings, err := releaseNotes(in, manifest)
	if err != nil {
		return nil, err
	}
	d.Warnings = append(d.Warnings, noteWarnings...)
	d.Files[NotesFile], roles[NotesFile] = notes, "notes"

	sums := make([]SumEntry, 0, len(d.Files))
	for p, data := range d.Files {
		sums = append(sums, SumEntry{Path: p, Digest: Digest(data)})
	}
	d.Files[SumsFile], roles[SumsFile] = FormatSums(sums), "checksums"

	d.Plan = buildPlan(in, d, roles, tp)
	planBytes, err := marshalJSON(d.Plan)
	if err != nil {
		return nil, err
	}
	d.Files[PlanFile] = planBytes
	return d, nil
}

// addExtras adds the templates, the pinned marketplace index and the emitters.
func addExtras(d *Dist, roles map[string]string, in Input, m Manifest) error {
	emitted, err := renderTemplates(in, m)
	if err != nil {
		return err
	}
	for name, data := range emitted {
		d.Files[name], roles[name] = data, "emitted"
	}
	files, warnings, err := BuildExtras(Extras{
		Channel: in.Channel, Pin: in.Pin, PinCommit: in.Source.Commit, PinDirty: in.Source.Dirty, Emit: in.Emit,
		Plugins: []emit.Plugin{emitPlugin(in, m)},
	})
	if err != nil {
		return err
	}
	for name, data := range files {
		d.Files[name] = data
		roles[name] = "emitted"
		if strings.HasPrefix(name, MarketplaceDir+"/") {
			roles[name] = "marketplace"
		}
	}
	d.Warnings = append(d.Warnings, warnings...)
	return nil
}

// emitPlugin describes the bundle to an emitter.
func emitPlugin(in Input, m Manifest) emit.Plugin {
	files := make([]emit.File, 0, len(in.Files))
	for _, f := range in.Files {
		files = append(files, emit.File{Path: f.Path, Data: f.Data})
	}
	p := emit.Plugin{
		Name: in.Name, Description: in.Description, Version: in.Version, Runtimes: m.Runtimes,
		BundleFile: m.Bundle.File, BundleDigest: m.Bundle.Digest, Files: files,
	}
	if in.Emit != nil {
		for _, pl := range in.Emit.Base.Plugins {
			if pl.Name == in.Name {
				p.Category, p.Keywords = pl.Category, pl.Keywords
			}
		}
	}
	return p
}

// addTargetFiles adds the files a target needs besides the common ones.
func addTargetFiles(d *Dist, roles map[string]string, in Input, manifestBytes []byte, tp targetPlan) error {
	m := d.Manifest
	switch in.Target {
	case TargetNPM:
		files, err := npmPackageFiles(in, m, *tp.npm)
		if err != nil {
			return err
		}
		for name, data := range files {
			d.Files[name], roles[name] = data, "npm-package"
		}
	case TargetOCI:
		packed, err := packOCI(m, manifestBytes, d.Files, in.Mtime)
		if err != nil {
			return err
		}
		d.Files[OCIManifestFile], roles[OCIManifestFile] = packed.Manifest, "oci-manifest"
	}
	return nil
}

// uploadList is what a github release carries: the archive, the manifest, the
// lock and the checksums, plus the signature and the SBOM when there are any.
func uploadList(m Manifest) []string {
	up := []string{m.Bundle.File, m.Name + "-" + m.Version + ".manifest.json", LockFile, SumsFile}
	if m.Signature != nil {
		up = append(up, m.Signature.File)
		if m.Signature.Attestation != "" {
			up = append(up, m.Signature.Attestation)
		}
	}
	if m.SBOM != nil {
		up = append(up, m.SBOM.File)
	}
	return up
}

func buildPlan(in Input, d *Dist, roles map[string]string, tp targetPlan) Plan {
	plan := Plan{
		SchemaVersion: SchemaVersion, Name: in.Name, Version: in.Version, Target: in.Target, Channel: in.Channel,
		Preflight: []Step{{"validate-strict", "ok"}, {"lock-check", "ok"}, {"verify-plugin", "ok"}, {"secret-scan", "ok"}},
		Commands:  []Command{},
	}
	for _, p := range d.Paths() {
		plan.Artifacts = append(plan.Artifacts, Artifact{Path: p, Role: roles[p], Digest: Digest(d.Files[p]), Size: len(d.Files[p])})
	}
	switch in.Target {
	case TargetGitHubRelease:
		plan.Tag, plan.Repo = in.Tag, in.Repo
		plan.Upload = uploadList(d.Manifest)
		plan.Commands = []Command{{Argv: ReleaseCreateArgv(in.Name, in.Version, in.Tag, in.Repo, plan.Upload), Cwd: "."}}
		plan.Credentials = "gh authentication (GH_TOKEN or `gh auth login`); not read by ai-rulez"
	case TargetNPM:
		plan.NPM = tp.npm
		plan.Commands = []Command{{Argv: NPMPackArgv(), Cwd: "."}, {Argv: NPMPublishArgv(*tp.npm), Cwd: "."}}
		plan.Credentials = "npm authentication (npm login, or NODE_AUTH_TOKEN in an .npmrc); not read by ai-rulez"
	case TargetOCI:
		plan.Ref = tp.ociRef
		plan.OCIDigest = Digest(d.Files[OCIManifestFile])
		plan.Upload = ociUploadList(d.Manifest)
		plan.Credentials = "registry credentials from the Docker credential store (`docker login`), read only for the registry named by ref"
	}
	return plan
}

// releaseNotes renders RELEASE_NOTES.md. A previous lock that cannot be parsed
// leaves the changes section out with a warning, unless it was named explicitly:
// a bad tag from a past release must not block a new one.
func releaseNotes(in Input, m Manifest) (notes []byte, warnings []string, err error) {
	var sb strings.Builder
	sb.WriteString("# " + in.Name + " " + in.Version + "\n\n")
	sb.WriteString("- Bundle: `" + m.Bundle.File + "` (" + m.Bundle.Digest + ")\n")
	sb.WriteString("- Runtimes: " + strings.Join(m.Runtimes, ", ") + "\n")
	sb.WriteString("- Lock tree: " + m.Lock.Tree + "\n")
	if in.Source.Commit != "" {
		sb.WriteString("- Source commit: " + in.Source.Commit + "\n")
	}
	if m.Signature != nil {
		sb.WriteString("- Signature: `" + m.Signature.File + "`\n")
	}
	if m.SBOM != nil {
		sb.WriteString("- SBOM: `" + m.SBOM.File + "`\n")
	}
	if len(in.PreviousLock) > 0 {
		prev, err := parseLock(in.PreviousLock, in.PreviousLabel)
		switch {
		case err != nil && in.PreviousExplicit:
			return nil, nil, err
		case err != nil:
			warnings = append(warnings, "the release notes omit the changes section: the lock at "+in.PreviousLabel+" cannot be read")
		default:
			cur, err := parseLock(in.Lock, in.Name)
			if err != nil {
				return nil, nil, err
			}
			label := in.PreviousLabel
			if label == "" {
				label = "the previous release"
			}
			sb.WriteString(notesSection(label, DiffLocks(prev, cur)))
		}
	}
	sb.WriteString("\nVerify the download with `ai-rulez publish verify <dir>` or `sha256sum -c SHA256SUMS`.\n")
	return []byte(sb.String()), warnings, nil
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
