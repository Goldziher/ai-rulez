package sbom

import (
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// subcommandRun is the "run" subcommand launchers take before a package.
const subcommandRun = "run"

// purlSegment percent-encodes one purl name or namespace segment ("@" becomes %40).
func purlSegment(s string) string {
	return strings.ReplaceAll(url.PathEscape(s), "@", "%40")
}

func purlSegments(p string) string {
	parts := strings.Split(p, "/")
	for i := range parts {
		parts[i] = purlSegment(parts[i])
	}
	return strings.Join(parts, "/")
}

// purl assembles pkg:<typ>/<namespace>/<name>@<version>?<qualifiers>#<subpath>.
// namespace is "/"-separated and may be empty; qualifiers are written sorted.
func purl(typ, namespace, name, version string, qualifiers map[string]string, subpath string) string {
	var b strings.Builder
	b.WriteString("pkg:" + typ + "/")
	if namespace != "" {
		b.WriteString(purlSegments(namespace) + "/")
	}
	b.WriteString(purlSegment(name))
	if version != "" {
		b.WriteString("@" + purlSegment(version))
	}
	keys := make([]string, 0, len(qualifiers))
	for k, v := range qualifiers {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for i, k := range keys {
		sep := "&"
		if i == 0 {
			sep = "?"
		}
		b.WriteString(sep + k + "=" + qualifierValue(qualifiers[k]))
	}
	if subpath = strings.Trim(subpath, "/"); subpath != "" && !strings.Contains(subpath, "..") {
		b.WriteString("#" + purlSegments(subpath))
	}
	return b.String()
}

// sourcePURL names a git source: pkg:github, pkg:gitlab and pkg:bitbucket for the
// hosts the purl specification knows, pkg:generic with a vcs_url qualifier for
// the rest. version is the commit when known, else the requested ref.
func sourcePURL(loc gitLocation, name, version, subpath string) string {
	host := strings.ToLower(loc.Host)
	owner, repo := path.Split(loc.Path)
	owner = strings.Trim(owner, "/")
	if owner != "" && repo != "" {
		switch host {
		case "github.com", "bitbucket.org":
			return purl(strings.SplitN(host, ".", 2)[0], strings.ToLower(owner), strings.ToLower(repo), version, nil, subpath)
		case "gitlab.com":
			return purl("gitlab", strings.ToLower(owner), strings.ToLower(repo), version, nil, subpath)
		}
	}
	return purl("generic", "", name, version, map[string]string{"vcs_url": "git+" + loc.HTTPS()}, subpath)
}

// launcher is what a heuristic recognizes in an MCP server command.
type launcher struct {
	purl, name, version string
	// requested is the version or tag the command line asked for ("" for none);
	// pinned reports that it names one release (exactVersion), in which case it
	// is also in the purl. An unpinned package has no version in its purl: a
	// scanner would match "latest" or a range against the wrong release.
	requested string
	pinned    bool
}

// detectLauncher guesses the package an MCP server command runs from the
// ecosystem launcher it uses: npx, bunx, pnpm/yarn dlx (npm), uvx and pipx run
// (PyPI), docker/podman run (OCI) and go run (Go). It is a heuristic over the
// command line, never an answer: an unrecognized command yields ok = false.
func detectLauncher(command string, args []string) (launcher, bool) {
	exe := strings.ToLower(filepath.Base(strings.ReplaceAll(command, `\`, "/")))
	for _, ext := range []string{".exe", ".cmd", ".bat"} {
		exe = strings.TrimSuffix(exe, ext)
	}
	l, ok := launchers[exe]
	if !ok || len(args) < len(l.subcommand) {
		return launcher{}, false
	}
	for i, word := range l.subcommand {
		if args[i] != word {
			return launcher{}, false
		}
	}
	return l.target(args[len(l.subcommand):])
}

// launcherCommand is how an ecosystem launcher names the package it runs: the
// subcommand words that come first, then the arguments target reads.
type launcherCommand struct {
	subcommand []string
	target     func(args []string) (launcher, bool)
}

// launchers maps a launcher executable to its command form.
var launchers = map[string]launcherCommand{
	"npx":    {target: npmTarget},
	"bunx":   {target: npmTarget},
	"pnpm":   {subcommand: []string{"dlx"}, target: npmTarget},
	"yarn":   {subcommand: []string{"dlx"}, target: npmTarget},
	"uvx":    {target: pypiTarget},
	"pipx":   {subcommand: []string{subcommandRun}, target: pypiTarget},
	"uv":     {subcommand: []string{"tool", subcommandRun}, target: pypiTarget},
	"docker": {subcommand: []string{subcommandRun}, target: ociTarget},
	"podman": {subcommand: []string{subcommandRun}, target: ociTarget},
	"go":     {subcommand: []string{subcommandRun}, target: goTarget},
}

// firstOperand returns the package operand of a launcher command line: the value
// of one of pkgFlags when present, else the first argument that is not a flag.
// valueFlags are the flags that take a separate value.
func firstOperand(args, pkgFlags, valueFlags []string) string {
	operand := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			if operand == "" && i+1 < len(args) {
				operand = args[i+1]
			}
			break
		}
		if !strings.HasPrefix(a, "-") {
			if operand == "" {
				operand = a
			}
			break // everything after the operand belongs to the server
		}
		name, value, hasValue := strings.Cut(a, "=")
		if slices.Contains(pkgFlags, name) {
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
			}
			return value
		}
		if !hasValue && slices.Contains(valueFlags, name) {
			i++
		}
	}
	return operand
}

func npmTarget(args []string) (launcher, bool) {
	op := firstOperand(args, []string{"-p", "--package"}, []string{"-c", "--call", "--registry", "--cache", "--node-options"})
	if op == "" || strings.ContainsAny(op, ":\\") || strings.HasPrefix(op, ".") || strings.HasPrefix(op, "/") {
		return launcher{}, false // a URL, tarball or path, not a registry package
	}
	name, version := op, ""
	if i := strings.LastIndex(op, "@"); i > 0 {
		name, version = op[:i], op[i+1:]
	}
	ns, base := "", name
	if i := strings.Index(name, "/"); i > 0 {
		ns, base = name[:i], name[i+1:]
	}
	if base == "" || strings.Contains(base, "/") {
		return launcher{}, false
	}
	pinned := exactVersion("npm", version)
	return launcher{purl: purl("npm", ns, base, pinnedVersion(version, pinned), nil, ""), name: name, version: pinnedVersion(version, pinned), requested: version, pinned: pinned}, true
}

func pypiTarget(args []string) (launcher, bool) {
	op := firstOperand(args, []string{"--from"}, []string{"-p", "--python", "--with", "-w", "--with-requirements", "--index", "--index-url", "-i", "--extra-index-url", "--python-preference", "--directory"})
	if op == "" || strings.ContainsAny(op, `:\/`) || strings.HasPrefix(op, ".") {
		return launcher{}, false
	}
	name, version := op, ""
	for _, sep := range []string{"==", "@"} {
		if n, v, ok := strings.Cut(name, sep); ok {
			name, version = n, v
			break
		}
	}
	if i := strings.IndexAny(name, "[<>=!~ "); i >= 0 {
		name = name[:i]
	}
	if name == "" {
		return launcher{}, false
	}
	norm := strings.ToLower(strings.NewReplacer("_", "-", ".", "-").Replace(name))
	pinned := exactVersion("pypi", version)
	return launcher{purl: purl("pypi", "", norm, pinnedVersion(version, pinned), nil, ""), name: norm, version: pinnedVersion(version, pinned), requested: version, pinned: pinned}, true
}

// dockerValueFlags are the docker run options that take a separate value.
var dockerValueFlags = []string{"-e", "--env", "--env-file", "-v", "--volume", "--mount", "-p", "--publish", "--name",
	"--network", "--net", "-w", "--workdir", "-u", "--user", "--entrypoint", "-l", "--label", "--platform", "--pull",
	"--memory", "-m", "--cpus", "--hostname", "-h", "--add-host", "--cap-add", "--cap-drop", "--security-opt",
	"--restart", "--log-driver", "--device", "--tmpfs", "--group-add", "--ulimit", "--workdir"}

func ociTarget(args []string) (launcher, bool) {
	image := firstOperand(args, nil, dockerValueFlags)
	if image == "" || strings.ContainsAny(image, "${}") {
		return launcher{}, false
	}
	ref, digest := image, ""
	if r, d, ok := strings.Cut(image, "@"); ok {
		// Only "@algo:hex" is a digest; "user:pass@host/img" carries credentials,
		// which must never reach the purl.
		if !ociDigest(d) {
			return launcher{}, false
		}
		ref, digest = r, d
	}
	tag := ""
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref, tag = ref[:i], ref[i+1:]
	}
	parts := strings.Split(ref, "/")
	registry := "docker.io"
	if len(parts) > 1 && (strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost") {
		registry, parts = parts[0], parts[1:]
	}
	if registry == "docker.io" && len(parts) == 1 {
		parts = []string{"library", parts[0]}
	}
	name := strings.ToLower(parts[len(parts)-1])
	if name == "" {
		return launcher{}, false
	}
	repo := registry + "/" + strings.ToLower(strings.Join(parts, "/"))
	version := digest
	if version == "" {
		version = tag
	}
	p := purl("oci", "", name, digest, map[string]string{"repository_url": repo, "tag": tag}, "")
	return launcher{purl: p, name: name, version: version, requested: version, pinned: digest != ""}, true
}

// ociDigest reports whether d looks like algo:value with no path or userinfo.
func ociDigest(d string) bool {
	algo, value, ok := strings.Cut(d, ":")
	return ok && algo != "" && value != "" && !strings.ContainsAny(d, "/@")
}

func goTarget(args []string) (launcher, bool) {
	op := firstOperand(args, nil, []string{"-C", "-o", "-p", "-tags", "-ldflags", "-gcflags", "-mod", "-modfile", "-overlay", "-pgo", "-asmflags", "-buildvcs", "-exec"})
	mod, version, ok := strings.Cut(op, "@")
	if !ok || version == "" || mod == "" || strings.HasPrefix(mod, ".") || !strings.Contains(mod, ".") {
		return launcher{}, false
	}
	ns, base := "", mod
	if i := strings.LastIndex(mod, "/"); i >= 0 {
		ns, base = mod[:i], mod[i+1:]
	}
	pinned := exactVersion("golang", version)
	return launcher{purl: purl("golang", ns, base, pinnedVersion(version, pinned), nil, ""), name: mod, version: pinnedVersion(version, pinned), requested: version, pinned: pinned}, true
}

// qualifierValue escapes a qualifier value, keeping "/" and ":" readable as the
// purl specification does.
func qualifierValue(v string) string {
	return strings.NewReplacer("%2F", "/", "%3A", ":").Replace(url.QueryEscape(v))
}

// pinnedVersion is v when pinned, else "".
func pinnedVersion(v string, pinned bool) string {
	if pinned {
		return v
	}
	return ""
}
