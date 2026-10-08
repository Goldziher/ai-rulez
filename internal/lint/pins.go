package lint

import (
	"path"
	"regexp"
	"strings"
)

// shellWords splits a command line into words, honoring single and double quotes.
func shellWords(s string) []string {
	var out []string
	var cur strings.Builder
	inS, inD, has := false, false, false
	flush := func() {
		if has {
			out = append(out, cur.String())
		}
		cur.Reset()
		has = false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inD:
			inS, has = !inS, true
		case c == '"' && !inS:
			inD, has = !inD, true
		case c == '\\' && !inS && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			has = true
		case (c == ' ' || c == '\t') && !inS && !inD:
			flush()
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	flush()
	return out
}

var (
	versionPinRe = regexp.MustCompile(`^(?:==|=|@)?[v=]?[\^~]?\d`)
	hexRefRe     = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	// versionRangeRe matches an npm version that selects a range, not a release:
	// ^1.2.0, ~1.2, >=1, 1, 1.2, 1.x, *.
	versionRangeRe = regexp.MustCompile(`^(?:(?:[\^~]|>=?|<=?)\s*v?\d.*|[*xX]|v?\d+(?:\.(?:\d+|[xX*]))?(?:\.[xX*])?)$`)
)

// pinOf splits "pkg@1.2.3" / "@scope/pkg@1.2.3" into name and version ("" when none).
func pinOf(spec string) (name, version string) {
	if i := strings.LastIndex(spec, "@"); i > 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, ""
}

// gitShorthandProblem handles npm's github:owner/repo style specs: pinned only
// with a #<commit or version> suffix.
func gitShorthandProblem(spec string) (why string, isShorthand bool) {
	host, rest, ok := strings.Cut(spec, ":")
	if !ok || (host != FormatGitHub && host != "gitlab" && host != "bitbucket") {
		return "", false
	}
	if _, ref, has := strings.Cut(rest, "#"); has && (hexRefRe.MatchString(ref) || versionPinRe.MatchString(ref)) {
		return "", true
	}
	return "the " + host + " shorthand has no #<commit> pin, so it follows the default branch", true
}

// npmPinProblem reports why an npm package spec is not pinned ("" when pinned
// or not a registry package).
func npmPinProblem(spec string) string {
	if spec == "" || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, "~") ||
		strings.Contains(spec, "://") || strings.HasPrefix(spec, "file:") || strings.HasPrefix(spec, "git+") {
		return ""
	}
	if why, isShorthand := gitShorthandProblem(spec); isShorthand {
		return why
	}
	_, ver := pinOf(spec)
	switch {
	case versionRangeRe.MatchString(ver):
		return "@" + ver + " is a moving version range"
	case strings.ContainsAny(ver, "<>${}"):
		return "" // a placeholder in documentation
	case ver == "":
		return whyNoVersionPinned
	case versionPinRe.MatchString(ver):
		return ""
	default:
		return "@" + ver + " is a moving tag"
	}
}

// pythonPinProblem reports why a Python requirement is not pinned.
func pythonPinProblem(spec string) string {
	switch {
	case spec == "" || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, "-"):
		return ""
	case strings.Contains(spec, "=="):
		return ""
	}
	if strings.HasPrefix(spec, "git+") {
		if i := strings.LastIndex(spec, "@"); i > strings.Index(spec, "://")+2 {
			ref := spec[i+1:]
			if j := strings.IndexAny(ref, "#?"); j >= 0 {
				ref = ref[:j]
			}
			if hexRefRe.MatchString(ref) || versionPinRe.MatchString(ref) {
				return ""
			}
			return "the git ref @" + ref + " is a moving ref"
		}
		return "the git requirement has no @<commit> pin"
	}
	if strings.HasPrefix(spec, "http://") || strings.HasPrefix(spec, "https://") {
		return "it installs from a URL"
	}
	if _, ver := pinOf(spec); ver != "" { // uvx pkg@1.2.3 / pkg@latest
		if versionPinRe.MatchString(ver) {
			return ""
		}
		return "@" + ver + " is a moving tag"
	}
	return whyNoVersionPinned
}

// flag-with-value sets for the launchers below.
var (
	uvxValueFlags    = map[string]bool{"--with": true, "--with-requirements": true, "--python": true, "-p": true, "--index": true, "--index-url": true, "--extra-index-url": true, "--env-file": true, "--with-editable": true}
	dockerValueFlags = map[string]bool{"-e": true, "--env": true, "-v": true, "--volume": true, "-p": true, "--publish": true, "--name": true, "--network": true, "--net": true, "-u": true, "--user": true, "-w": true, "--workdir": true, "--env-file": true, "--entrypoint": true, "--mount": true, "--platform": true, "-l": true, "--label": true, "--add-host": true, "--memory": true, "-m": true, "--cpus": true, "--pull": true}
)

// pinProblem inspects a command line (argv[0] first) and reports a launcher that
// runs a package without pinning it: npx, bunx, pnpm dlx, npm exec, uvx, uv tool
// run, pipx run, go run/install pkg@latest, pip install from a URL or an
// unpinned git requirement, and (allowDocker) docker run of an untagged image.
// needYes makes npx count only with -y/--yes (an interactive npx asks first);
// a moving @tag is reported either way.
func pinProblem(argv []string, needYes, allowDocker bool) (pkg, reason string) {
	for len(argv) > 0 && (strings.Contains(argv[0], "=") && !strings.HasPrefix(argv[0], "-")) { // VAR=value prefix
		argv = argv[1:]
	}
	for len(argv) > 0 && (argv[0] == cmdSudo || argv[0] == keyEnv || argv[0] == cmdTime || argv[0] == cmdExec) {
		argv = argv[1:]
	}
	if len(argv) == 0 {
		return "", ""
	}
	check, ok := pinCheckers[path.Base(argv[0])]
	if !ok {
		return "", ""
	}
	return check(path.Base(argv[0]), argv[1:], needYes, allowDocker)
}

// pinChecker checks the arguments that follow a launcher's name; cmd is the
// launcher name, since several names share one checker.
type pinChecker func(cmd string, args []string, needYes, allowDocker bool) (pkg, reason string)

var pinCheckers = map[string]pinChecker{
	toolNpx:    npxLauncher,
	"bunx":     npxLauncher,
	"pnpx":     npxLauncher,
	toolPnpm:   dlxLauncher,
	toolYarn:   dlxLauncher,
	toolBun:    dlxLauncher,
	cmdNPM:     npmLauncher,
	toolUvx:    func(_ string, args []string, _, _ bool) (pkg, reason string) { return uvxProblem(args) },
	"uv":       uvLauncher,
	toolPipx:   pipxLauncher,
	cmdPip:     pipLauncher,
	"pip3":     pipLauncher,
	toolPython: pythonLauncher,
	"python3":  pythonLauncher,
	"go":       goLauncher,
	cmdDocker:  dockerLauncher,
	"podman":   dockerLauncher,
}

func npxLauncher(cmd string, args []string, needYes, _ bool) (pkg, reason string) {
	return npxProblem(args, needYes && cmd == toolNpx)
}

func dlxLauncher(_ string, args []string, _, _ bool) (pkg, reason string) {
	if len(args) > 0 && (args[0] == "dlx" || args[0] == "x") {
		return npxProblem(args[1:], false)
	}
	return "", ""
}

func npmLauncher(_ string, args []string, needYes, _ bool) (pkg, reason string) {
	if len(args) > 0 && (args[0] == cmdExec || args[0] == "x") {
		return npxProblem(args[1:], needYes)
	}
	return "", ""
}

func uvLauncher(_ string, args []string, _, _ bool) (pkg, reason string) {
	if len(args) > 1 && args[0] == "tool" && args[1] == wordRun {
		return uvxProblem(args[2:])
	}
	if len(args) > 0 && args[0] == "x" {
		return uvxProblem(args[1:])
	}
	if len(args) > 1 && args[0] == cmdPip && args[1] == wordInstall {
		return pipProblem(args[2:])
	}
	return "", ""
}

func pipxLauncher(_ string, args []string, _, _ bool) (pkg, reason string) {
	if len(args) > 0 && args[0] == wordRun {
		return pipxRunProblem(args[1:])
	}
	return "", ""
}

func pipLauncher(_ string, args []string, _, _ bool) (pkg, reason string) {
	if len(args) > 0 && args[0] == wordInstall {
		return pipProblem(args[1:])
	}
	return "", ""
}

func pythonLauncher(_ string, args []string, _, _ bool) (pkg, reason string) {
	if len(args) > 2 && args[0] == "-m" && args[1] == cmdPip && args[2] == wordInstall {
		return pipProblem(args[3:])
	}
	return "", ""
}

func goLauncher(_ string, args []string, _, _ bool) (pkg, reason string) {
	if len(args) > 1 && (args[0] == wordRun || args[0] == wordInstall) {
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				continue
			}
			if _, ver := pinOf(a); ver == "latest" || ver == "master" || ver == "main" {
				return a, "@" + ver + " is a moving ref"
			}
			break
		}
	}
	return "", ""
}

func dockerLauncher(_ string, args []string, _, allowDocker bool) (pkg, reason string) {
	if allowDocker {
		return dockerProblem(args)
	}
	return "", ""
}

func npxProblem(args []string, needYes bool) (pkg, reason string) {
	yes := !needYes
	var explicit []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-y" || a == "--yes":
			yes = true
		case a == "-p" || a == "--package":
			if i+1 < len(args) {
				explicit = append(explicit, args[i+1])
				i++
			}
		case strings.HasPrefix(a, "--package="):
			explicit = append(explicit, strings.TrimPrefix(a, "--package="))
		case a == "--":
		case strings.HasPrefix(a, "-"):
		default:
			if len(explicit) == 0 {
				explicit = append(explicit, a)
			}
			i = len(args)
		}
	}
	for _, spec := range explicit {
		if why := npmPinProblem(spec); why != "" && (yes || strings.Contains(why, "moving")) {
			return spec, why
		}
	}
	return "", ""
}

func uvxProblem(args []string) (pkg, reason string) {
	var from string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--from" && i+1 < len(args):
			from = args[i+1]
			i++
		case strings.HasPrefix(a, "--from="):
			from = strings.TrimPrefix(a, "--from=")
		case uvxValueFlags[a]:
			i++
		case strings.HasPrefix(a, "-"):
		default:
			if from == "" {
				from = a
			}
			i = len(args)
		}
	}
	if why := pythonPinProblem(from); why != "" {
		return from, why
	}
	return "", ""
}

func pipxRunProblem(args []string) (pkg, reason string) {
	var spec string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--spec" && i+1 < len(args):
			spec = args[i+1]
			i++
		case strings.HasPrefix(a, "--spec="):
			spec = strings.TrimPrefix(a, "--spec=")
		case a == "--python" || a == "--pip-args":
			i++
		case strings.HasPrefix(a, "-"):
		default:
			if spec == "" {
				spec = a
			}
			i = len(args)
		}
	}
	if why := pythonPinProblem(spec); why != "" {
		return spec, why
	}
	return "", ""
}

func pipProblem(args []string) (pkg, reason string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-i", "--index-url", "--extra-index-url", "-f", "--find-links", "-r", "--requirement", "-c", "--constraint", "-e", "--editable", "--target", "-t", "--prefix":
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		if strings.HasPrefix(a, "git+") || strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
			if why := pythonPinProblem(a); why != "" {
				return a, why
			}
		}
	}
	return "", ""
}

func dockerProblem(args []string) (pkg, reason string) {
	if len(args) == 0 || args[0] != wordRun {
		return "", ""
	}
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case dockerValueFlags[a]:
			i++
		case strings.HasPrefix(a, "-"):
		default:
			if strings.Contains(a, "@sha256:") {
				return "", ""
			}
			name := a
			tag := ""
			if j := strings.LastIndex(a, ":"); j > strings.LastIndex(a, "/") {
				name, tag = a[:j], a[j+1:]
			}
			switch tag {
			case "":
				return name, "the image has no tag or digest"
			case "latest":
				return name, ":latest is a moving tag"
			}
			return "", ""
		}
	}
	return "", ""
}
