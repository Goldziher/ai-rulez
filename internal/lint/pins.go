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
)

// pinOf splits "pkg@1.2.3" / "@scope/pkg@1.2.3" into name and version ("" when none).
func pinOf(spec string) (name, version string) {
	if i := strings.LastIndex(spec, "@"); i > 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, ""
}

// npmPinProblem reports why an npm package spec is not pinned ("" when pinned
// or not a registry package).
func npmPinProblem(spec string) string {
	if spec == "" || strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, "~") ||
		strings.Contains(spec, "://") || strings.HasPrefix(spec, "file:") || strings.HasPrefix(spec, "git+") || strings.HasPrefix(spec, "github:") {
		return ""
	}
	_, ver := pinOf(spec)
	switch {
	case strings.ContainsAny(ver, "<>${}"):
		return "" // a placeholder in documentation
	case ver == "":
		return "no version is pinned"
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
	return "no version is pinned"
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
func pinProblem(argv []string, needYes, allowDocker bool) (pkg, reason string) { //nolint:gocyclo // linear checks over a documented schema; splitting them hides the rules
	for len(argv) > 0 && (strings.Contains(argv[0], "=") && !strings.HasPrefix(argv[0], "-")) { // VAR=value prefix
		argv = argv[1:]
	}
	for len(argv) > 0 && (argv[0] == cmdSudo || argv[0] == keyEnv || argv[0] == "time" || argv[0] == cmdExec) {
		argv = argv[1:]
	}
	if len(argv) == 0 {
		return "", ""
	}
	cmd := path.Base(argv[0])
	args := argv[1:]
	switch cmd {
	case "npx", "bunx", "pnpx":
		return npxProblem(args, needYes && cmd == "npx")
	case "pnpm", "yarn", "bun":
		if len(args) > 0 && (args[0] == "dlx" || args[0] == "x") {
			return npxProblem(args[1:], false)
		}
	case cmdNPM:
		if len(args) > 0 && (args[0] == cmdExec || args[0] == "x") {
			return npxProblem(args[1:], needYes)
		}
	case "uvx":
		return uvxProblem(args)
	case "uv":
		if len(args) > 1 && args[0] == "tool" && args[1] == wordRun {
			return uvxProblem(args[2:])
		}
		if len(args) > 0 && args[0] == "x" {
			return uvxProblem(args[1:])
		}
		if len(args) > 1 && args[0] == cmdPip && args[1] == wordInstall {
			return pipProblem(args[2:])
		}
	case "pipx":
		if len(args) > 0 && args[0] == wordRun {
			return pipxRunProblem(args[1:])
		}
	case cmdPip, "pip3":
		if len(args) > 0 && args[0] == wordInstall {
			return pipProblem(args[1:])
		}
	case "python", "python3":
		if len(args) > 2 && args[0] == "-m" && args[1] == cmdPip && args[2] == wordInstall {
			return pipProblem(args[3:])
		}
	case "go":
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
	case cmdDocker, "podman":
		if allowDocker {
			return dockerProblem(args)
		}
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
