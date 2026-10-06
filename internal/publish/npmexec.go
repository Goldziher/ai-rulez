package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// defaultNPMRegistry is the registry npm uses when nothing configures another.
const defaultNPMRegistry = "https://registry.npmjs.org/"

// NPMExecuteOptions configure ExecuteNPM.
type NPMExecuteOptions struct {
	// Dir is the written dist directory the package is packed from.
	Dir string
	// Env is the complete environment of npm, built by the caller (runner.ScrubEnv).
	Env []string
	// ConfirmRegistry is the registry URL the operator confirmed. A registry chosen by the committed
	// [publish.npm] config (other than the public one) is refused unless it equals this.
	ConfirmRegistry string
	// Notice receives what the operator should see before the upload (the
	// effective registry); nil prints nothing.
	Notice func(msg string)
}

// ExecuteNPM packs and publishes through the npm CLI. It re-verifies the dist
// directory first, refuses a version the registry already has (npm versions are
// immutable) and runs npm from an empty temporary directory with the package and
// tarball given by absolute path and --userconfig/--globalconfig named
// explicitly, so a .npmrc committed in the repository (which could redirect the
// registry and receive NODE_AUTH_TOKEN) never applies. The packed tarball is
// digested and checked again just before it is published. ai-rulez never sees a
// credential: npm authenticates itself from the user's own configuration and
// the environment the caller passed.
func ExecuteNPM(ctx context.Context, r runner.Runner, plan Plan, opts NPMExecuteOptions) (string, error) {
	sess, err := startNPM(ctx, r, plan, opts)
	if err != nil {
		return "", err
	}
	defer sess.close()
	if err := sess.checkFree(); err != nil {
		return "", err
	}
	packDir := filepath.Join(sess.tmp, "pack")
	if err := os.Mkdir(packDir, 0o700); err != nil {
		return "", newError(CodeTarget, ExitFailed, "", "cannot create a temporary directory: %v", err)
	}
	pack := []string{"npm", "pack", "--ignore-scripts"}
	pack = append(pack, sess.flags...)
	pack = append(pack, "--pack-destination", packDir, filepath.Join(sess.abs, filepath.FromSlash(NPMPackageDir)))
	if res := sess.run(pack); res.Status != runner.StatusOK {
		return "", npmFailure("npm pack", res)
	}
	tarball := filepath.Join(packDir, path.Base(plan.NPM.Tarball))
	packed, err := readRegular(tarball)
	if err != nil {
		return "", newError(CodeTarget, ExitFailed, "", "npm pack did not write %s: %v", path.Base(plan.NPM.Tarball), err)
	}
	digest := Digest(packed)

	publishArgv := NPMPublishArgv(*plan.NPM)
	publishArgv[2] = tarball
	publishArgv = append(publishArgv, sess.flags...)
	again, err := readRegular(tarball)
	if err != nil || Digest(again) != digest {
		return "", newError(CodeTarget, ExitFailed, "rerun `ai-rulez publish --execute`", "the packed tarball changed between npm pack and npm publish")
	}
	if res := sess.run(publishArgv); res.Status != runner.StatusOK {
		return "", npmFailure("npm publish", res)
	}
	return plan.NPM.Package + "@" + plan.Version + " (tarball " + digest + ")", nil
}

// CheckNPM runs the checks ExecuteNPM starts with, publishing nothing: the plan,
// the dist directory, the effective registry and that the registry does not
// already hold the version. A multi-plugin publish calls it for every plugin
// before it uploads the first.
func CheckNPM(ctx context.Context, r runner.Runner, plan Plan, opts NPMExecuteOptions) error {
	sess, err := startNPM(ctx, r, plan, opts)
	if err != nil {
		return err
	}
	defer sess.close()
	return sess.checkFree()
}

// npmSession is one isolated npm invocation context.
type npmSession struct {
	plan  Plan
	abs   string
	tmp   string
	flags []string
	run   func(argv []string) runner.Result
}

func (s *npmSession) close() { os.RemoveAll(s.tmp) } //nolint:errcheck // best effort cleanup of our own directory

// startNPM validates the plan and the dist directory and prepares the empty
// working directory, the explicit config files and the registry check.
func startNPM(ctx context.Context, r runner.Runner, plan Plan, opts NPMExecuteOptions) (*npmSession, error) {
	if plan.Target != TargetNPM || plan.NPM == nil || len(plan.Commands) != 2 {
		return nil, newError(CodeTarget, ExitFailed, "", "the plan has no npm commands to run")
	}
	want := []Command{{Argv: NPMPackArgv(), Cwd: "."}, {Argv: NPMPublishArgv(*plan.NPM), Cwd: "."}}
	if !reflect.DeepEqual(plan.Commands, want) {
		return nil, newError(CodeTarget, ExitFailed, "rebuild the dist directory with `ai-rulez publish`", "the plan's npm commands differ from the ones publish builds")
	}
	abs, err := filepath.Abs(opts.Dir)
	if err != nil {
		return nil, newError(CodeTarget, ExitFailed, "", "cannot resolve the dist directory: %v", err)
	}
	if err := requireVerifiedDist(abs); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "ai-rulez-npm-*")
	if err != nil {
		return nil, newError(CodeTarget, ExitFailed, "", "cannot create a temporary directory: %v", err)
	}
	cfg, err := newNPMConfig(tmp, opts.Env)
	if err != nil {
		os.RemoveAll(tmp) //nolint:errcheck // best effort cleanup of our own directory
		return nil, err
	}
	r = runner.Or(r)
	sess := &npmSession{
		plan: plan, abs: abs, tmp: tmp, flags: cfg.flags(*plan.NPM),
		run: func(argv []string) runner.Result {
			return r.Run(ctx, runner.Spec{Argv: argv, Dir: tmp, Env: opts.Env, Timeout: uploadTimeout})
		},
	}
	registry, err := sess.effectiveRegistry()
	if err == nil {
		err = checkNPMRegistry(*plan.NPM, registry, opts.ConfirmRegistry)
	}
	if err != nil {
		sess.close()
		return nil, err
	}
	if opts.Notice != nil {
		opts.Notice("npm registry: " + registry)
	}
	return sess, nil
}

// effectiveRegistry asks npm itself which registry the package goes to, under the same isolated flags the pack,
// view and publish calls get: environment variables (npm_config_registry, npm_config_@scope:registry, ...), the
// user's npmrc and the built-in default all count the way npm applies them, which a reimplementation would miss.
func (s *npmSession) effectiveRegistry() (string, error) {
	argv := append([]string{"npm", "config", "list", "--json", "-l"}, s.flags...)
	if s.plan.NPM.Registry != "" {
		argv = append(argv, "--registry", s.plan.NPM.Registry)
	}
	res := s.run(argv)
	if res.Status != runner.StatusOK {
		return "", npmFailure("npm config list", res)
	}
	var conf map[string]any
	if err := json.Unmarshal(res.Stdout, &conf); err != nil {
		return "", newError(CodeTarget, ExitFailed, "", "cannot read the effective npm registry from npm config list: output is not JSON")
	}
	scope, _, _ := strings.Cut(s.plan.NPM.Package, "/")
	for _, key := range []string{scope + ":registry", "registry"} {
		if v, ok := conf[key].(string); ok && v != "" {
			return v, nil
		}
	}
	return defaultNPMRegistry, nil
}

// checkNPMRegistry requires an https registry, and a registry that the committed [publish.npm] config chose
// (plan.Registry) to be confirmed by value: that registry receives the user's npm token, so a repository must
// not be able to name one silently. Registries from the user's own npm configuration are theirs and need no
// confirmation.
func checkNPMRegistry(p NPMPlan, registry, confirmed string) error {
	if !strings.HasPrefix(registry, "https://") {
		return newError(CodeTarget, ExitFailed, "set [publish.npm] registry to an https URL, or fix the registry in your npm configuration",
			"the effective npm registry %q is not an https:// URL", registry)
	}
	if p.Registry != "" && normalizeRegistry(p.Registry) != normalizeRegistry(defaultNPMRegistry) &&
		normalizeRegistry(confirmed) != normalizeRegistry(p.Registry) {
		return newError(CodeTarget, ExitFailed, "check the registry, then pass --confirm-registry "+p.Registry,
			"the committed [publish.npm] config sends your npm credentials to %s: confirm that registry by name", p.Registry)
	}
	return nil
}

func normalizeRegistry(u string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(u), "/")) }

// checkFree refuses a version the registry already holds (npm versions are immutable).
func (s *npmSession) checkFree() error {
	view := s.run(append(NPMViewArgv(*s.plan.NPM, s.plan.Version), s.flags...))
	switch view.Status {
	case runner.StatusOK:
		if strings.TrimSpace(string(view.Stdout)) != "" {
			return newError(CodeTarget, ExitFailed, "bump [plugin] version; npm versions cannot be replaced",
				"%s@%s already exists in the registry", s.plan.NPM.Package, s.plan.Version)
		}
	case runner.StatusUnavailable:
		return npmMissing()
	case runner.StatusExit:
		if !npmNotFound(view) {
			return npmFailure("npm view", view)
		}
	default:
		return npmFailure("npm view", view)
	}
	return nil
}

// requireVerifiedDist fails unless the dist directory still verifies, so the
// package that is packed is the one that was reviewed.
func requireVerifiedDist(dir string) error {
	res, err := Verify(dir)
	if err != nil {
		return err
	}
	if res.OK() {
		return nil
	}
	p := res.Problems[0]
	return newError(CodeTarget, ExitFailed, "rebuild the dist directory with `ai-rulez publish`",
		"the dist directory no longer verifies (%d problem(s)); %s: %s", len(res.Problems), p.Path, p.Message)
}

// npmConfig names the config files npm is told to read.
type npmConfig struct {
	user, global string
}

// newNPMConfig points npm at the user's own npmrc (NPM_CONFIG_USERCONFIG, else
// ~/.npmrc, where `npm login` keeps credentials) and at an empty global one
// unless NPM_CONFIG_GLOBALCONFIG names it. A missing user file is replaced by an
// empty one so npm does not look elsewhere.
func newNPMConfig(tmp string, env []string) (npmConfig, error) {
	empty := filepath.Join(tmp, "empty.npmrc")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		return npmConfig{}, newError(CodeTarget, ExitFailed, "", "cannot write a temporary file: %v", err)
	}
	cfg := npmConfig{user: userNPMRC(env), global: envValue(env, "NPM_CONFIG_GLOBALCONFIG")}
	if cfg.user == "" {
		cfg.user = empty
	} else if _, err := os.Stat(cfg.user); err != nil {
		cfg.user = empty
	}
	if cfg.global == "" {
		cfg.global = empty
	}
	return cfg, nil
}

func userNPMRC(env []string) string {
	if p := envValue(env, "NPM_CONFIG_USERCONFIG"); p != "" {
		return p
	}
	if home := envValue(env, "HOME"); home != "" {
		return filepath.Join(home, ".npmrc")
	}
	return ""
}

// flags are the config and registry flags every npm call gets. A registry named
// by the plan is also given as the scope's registry, which would otherwise win
// over --registry for a scoped package.
func (c npmConfig) flags(p NPMPlan) []string {
	out := []string{"--userconfig", c.user, "--globalconfig", c.global}
	if scope, _, ok := strings.Cut(p.Package, "/"); ok && p.Registry != "" {
		out = append(out, "--"+scope+":registry="+p.Registry)
	}
	return out
}

// NPMEffectiveRegistry is the registry npm will use for the package: the plan's
// registry, else the scope's registry or the default registry from the
// environment or the user's npmrc, else the public registry. Project files are
// never consulted: the execute step does not read them.
func NPMEffectiveRegistry(p NPMPlan, env []string) string {
	if p.Registry != "" {
		return p.Registry
	}
	scope, _, _ := strings.Cut(p.Package, "/")
	conf := parseNPMRC(userNPMRC(env))
	if v := conf[scope+":registry"]; v != "" {
		return v
	}
	for _, name := range []string{"NPM_CONFIG_REGISTRY", "npm_config_registry"} {
		if v := envValue(env, name); v != "" {
			return v
		}
	}
	if v := conf["registry"]; v != "" {
		return v
	}
	return defaultNPMRegistry
}

// parseNPMRC reads the key = value lines of an npmrc; an unreadable file is empty.
func parseNPMRC(file string) map[string]string {
	out := map[string]string{}
	if file == "" {
		return out
	}
	data, err := readRegular(file)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}

// envValue returns the value of name in a KEY=VALUE list; "" when unset.
func envValue(env []string, name string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// npmE404 matches npm's error code in its text output ("npm error code E404").
var npmE404 = regexp.MustCompile(`\bE404\b`)

// npmNotFound reports whether a failed `npm view --json` said the version does
// not exist: the error code E404 in npm's JSON error (or, from an older npm, its
// text). Words such as "not found" or a bare 404 in other output (a proxy page,
// a missing command) do not count: they would let a broken registry look like a
// free version.
func npmNotFound(res runner.Result) bool {
	for _, out := range [][]byte{res.Stdout, res.Stderr} {
		var doc struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(bytes.TrimSpace(out), &doc) == nil && doc.Error.Code == "E404" {
			return true
		}
		if npmE404.Match(out) {
			return true
		}
	}
	return false
}
