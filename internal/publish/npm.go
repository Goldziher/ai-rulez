package publish

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// NPM dist layout: the package directory npm packs, and the tarball it writes.
const (
	NPMDir        = "npm"
	NPMPackageDir = NPMDir + "/package"
)

// npmMaxName is npm's limit on a package name, scope included.
const npmMaxName = 214

var (
	npmNamePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	npmScopePattern = regexp.MustCompile(`^@[a-z0-9][a-z0-9._-]*$`)
	npmSemver       = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
	npmTagPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)
)

// NPMOptions configure the npm target.
type NPMOptions struct {
	// Scope is "@acme"; required, so a package never lands in the public
	// unscoped namespace by accident.
	Scope string
	// Access is "restricted" (default) or "public".
	Access string
	// Registry is an https URL; empty uses the npm client's registry.
	Registry string
}

// NPMPlan is the npm part of the publish plan: everything the argv is built from.
type NPMPlan struct {
	Package  string `json:"package"`
	Tarball  string `json:"tarball"`
	Access   string `json:"access"`
	Registry string `json:"registry,omitempty"`
	// Tag is the dist-tag: the channel, or empty for npm's default (latest).
	Tag string `json:"tag,omitempty"`
}

// npmPackageName is "<scope>/<name>".
func npmPackageName(scope, name string) string { return scope + "/" + name }

// NPMTarball is the file `npm pack` writes for a package: the scope's "@" is
// dropped and "/" becomes "-" (npm 7 and later).
func NPMTarball(pkg, version string) string {
	flat := strings.ReplaceAll(strings.TrimPrefix(pkg, "@"), "/", "-")
	return NPMDir + "/" + flat + "-" + version + ".tgz"
}

// NPMPackArgv packs the package directory into dist/npm. Scripts never run.
func NPMPackArgv() []string {
	return []string{"npm", "pack", "--ignore-scripts", "--pack-destination", NPMDir, NPMPackageDir}
}

// NPMPublishArgv publishes the tarball.
func NPMPublishArgv(p NPMPlan) []string {
	argv := []string{"npm", "publish", p.Tarball, "--access", p.Access, "--ignore-scripts"}
	if p.Registry != "" {
		argv = append(argv, "--registry", p.Registry)
	}
	if p.Tag != "" {
		argv = append(argv, "--tag", p.Tag)
	}
	return argv
}

// NPMViewArgv asks the registry whether the exact version already exists.
func NPMViewArgv(p NPMPlan, version string) []string {
	argv := []string{"npm", "view", p.Package + "@" + version, "version"}
	if p.Registry != "" {
		argv = append(argv, "--registry", p.Registry)
	}
	return argv
}

// ValidateNPM checks the npm options and the plugin name and version, which become the package's.
func ValidateNPM(o NPMOptions, name, version string) (NPMPlan, error) {
	switch {
	case !npmScopePattern.MatchString(o.Scope):
		return NPMPlan{}, newError(CodeConfig, ExitFailed, "set [publish.npm] scope = \"@acme\" or pass --npm-scope", "the npm target needs a scope such as @acme, got %q", o.Scope)
	case !npmNamePattern.MatchString(name):
		return NPMPlan{}, newError(CodeConfig, ExitFailed, "npm package names are lower-case", "plugin name %q is not a valid npm package name", name)
	case len(o.Scope)+1+len(name) > npmMaxName:
		return NPMPlan{}, newError(CodeConfig, ExitFailed, "", "the npm package name exceeds %d characters", npmMaxName)
	case !npmSemver.MatchString(version):
		return NPMPlan{}, newError(CodeConfig, ExitFailed, "npm needs a semantic version", "plugin version %q is not a semantic version", version)
	}
	access := o.Access
	if access == "" {
		access = "restricted"
	}
	if access != "restricted" && access != "public" {
		return NPMPlan{}, newError(CodeConfig, ExitFailed, "", "invalid npm access %q (use restricted or public)", access)
	}
	if o.Registry != "" && !strings.HasPrefix(o.Registry, "https://") {
		return NPMPlan{}, newError(CodeConfig, ExitFailed, "", "the npm registry must be an https:// URL")
	}
	pkg := npmPackageName(o.Scope, name)
	return NPMPlan{Package: pkg, Tarball: NPMTarball(pkg, version), Access: access, Registry: o.Registry}, nil
}

// checkNPMPlan validates a plan read from disk against the manifest's name and
// version before its argv is trusted.
func checkNPMPlan(p NPMPlan, name, version string) error {
	scope, base, ok := strings.Cut(p.Package, "/")
	if !ok || base != name {
		return oops.Errorf("package %q is not a scoped package named %s", p.Package, name)
	}
	want, err := ValidateNPM(NPMOptions{Scope: scope, Access: p.Access, Registry: p.Registry}, name, version)
	if err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the code
	}
	if p.Tarball != want.Tarball {
		return oops.Errorf("tarball is %q, expected %q", p.Tarball, want.Tarball)
	}
	if p.Tag != "" && !npmTagPattern.MatchString(p.Tag) {
		return oops.Errorf("invalid dist-tag %q", p.Tag)
	}
	return nil
}

// npmPackageJSON is the package.json of the npm package. The ai-rulez key ties
// the package to the release archive and the lock.
type npmPackageJSON struct {
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	Description string         `json:"description,omitempty"`
	Repository  *npmRepository `json:"repository,omitempty"`
	Files       []string       `json:"files"`
	AIRulez     npmAIRulez     `json:"ai-rulez"`
	PublishCfg  npmPublishCfg  `json:"publishConfig"`
}

type npmRepository struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type npmAIRulez struct {
	BundleDigest string   `json:"bundle_digest"`
	LockTree     string   `json:"lock_tree"`
	Runtimes     []string `json:"runtimes"`
}

type npmPublishCfg struct {
	Access string `json:"access"`
}

// npmPackageFiles builds the package directory: the bundle files at its root
// and a package.json whose files allow-list is the bundle's top-level entries.
// A bundle that already ships a package.json (the opencode runtime does) cannot
// become an npm package without overwriting it, so it is refused.
func npmPackageFiles(in Input, m Manifest, plan NPMPlan) (map[string][]byte, error) {
	top := map[string]bool{}
	files := map[string][]byte{}
	for _, f := range in.Files {
		if f.Path == "package.json" {
			return nil, newError(CodeConfig, ExitFailed, "drop the opencode runtime with --runtime, or publish to another target",
				"the bundle already has a package.json; the npm target writes its own")
		}
		top[strings.SplitN(f.Path, "/", 2)[0]] = true
		files[NPMPackageDir+"/"+f.Path] = f.Data
	}
	allow := make([]string, 0, len(top))
	for name := range top {
		allow = append(allow, name)
	}
	sort.Strings(allow)
	pkg := npmPackageJSON{
		Name: plan.Package, Version: in.Version, Description: in.Description, Files: allow,
		AIRulez:    npmAIRulez{BundleDigest: m.Bundle.Digest, LockTree: m.Lock.Tree, Runtimes: m.Runtimes},
		PublishCfg: npmPublishCfg{Access: plan.Access},
	}
	if in.Repo != "" {
		if host, slug, ok := splitRepo(in.Repo); ok {
			pkg.Repository = &npmRepository{Type: "git", URL: "git+https://" + host + "/" + slug + ".git"}
		}
	}
	data, err := marshalJSON(pkg)
	if err != nil {
		return nil, err
	}
	files[NPMPackageDir+"/package.json"] = data
	return files, nil
}

// NPMExecuteOptions configure ExecuteNPM.
type NPMExecuteOptions struct {
	// Dir is the written dist directory, the working directory of npm.
	Dir string
	// Env is the complete environment of npm, built by the caller (runner.ScrubEnv).
	Env []string
}

// ExecuteNPM packs and publishes through the npm CLI. It refuses a version the
// registry already has (npm versions are immutable). ai-rulez never sees a
// credential: npm authenticates itself from its own configuration and the
// environment the caller passed.
func ExecuteNPM(ctx context.Context, r runner.Runner, plan Plan, opts NPMExecuteOptions) (string, error) {
	if plan.Target != TargetNPM || plan.NPM == nil || len(plan.Commands) != 2 {
		return "", newError(CodeTarget, ExitFailed, "", "the plan has no npm commands to run")
	}
	r = runner.Or(r)
	run := func(argv []string) runner.Result {
		return r.Run(ctx, runner.Spec{Argv: argv, Dir: opts.Dir, Env: opts.Env, Timeout: uploadTimeout})
	}
	view := run(NPMViewArgv(*plan.NPM, plan.Version))
	switch view.Status {
	case runner.StatusOK:
		if strings.TrimSpace(string(view.Stdout)) != "" {
			return "", newError(CodeTarget, ExitFailed, "bump [plugin] version; npm versions cannot be replaced",
				"%s@%s already exists in the registry", plan.NPM.Package, plan.Version)
		}
	case runner.StatusUnavailable:
		return "", npmMissing()
	case runner.StatusExit:
		low := strings.ToLower(string(view.Stderr))
		if !strings.Contains(low, "e404") && !strings.Contains(low, "404") && !strings.Contains(low, "not found") {
			return "", npmFailure("npm view", view)
		}
	default:
		return "", npmFailure("npm view", view)
	}
	for _, c := range plan.Commands {
		res := run(c.Argv)
		if res.Status != runner.StatusOK {
			return "", npmFailure(strings.Join(c.Argv[:2], " "), res)
		}
	}
	return plan.NPM.Package + "@" + plan.Version, nil
}

func npmMissing() error {
	return newError(CodeTarget, ExitFailed, "install Node.js and npm (https://nodejs.org) and authenticate with `npm login`", "npm was not found on PATH")
}

func npmFailure(what string, res runner.Result) error {
	if res.Status == runner.StatusUnavailable {
		return npmMissing()
	}
	detail := strings.TrimSpace(string(res.Stderr))
	if detail == "" && res.Err != nil {
		detail = res.Err.Error()
	}
	return newError(CodeTarget, ExitFailed, "", "%s failed (%s): %s", what, res.Status, Redact(detail))
}
