package runner

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// baseEnv are the variables every scrubbed environment keeps: enough for a
// tool to find binaries, a home and temp directory, and a locale. The locale
// (LANG, LANGUAGE and every LC_*) is kept because the scanners and gh decide
// their output encoding from it (Python and others pick the stdout codec from
// LC_ALL/LC_CTYPE/LANG), and the caller parses that output. SHELL and LOGNAME
// are not kept: no caller needs either (USER covers identity, tools that run a
// shell use /bin/sh), and SHELL names a program the child could launch.
var baseEnv = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "TMPDIR": true, "TMP": true,
	"TEMP": true, "TZ": true, "LANG": true, "LANGUAGE": true,
	// Windows needs these for a process to start at all.
	"SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "USERPROFILE": true,
	"APPDATA": true, "LOCALAPPDATA": true, "PROGRAMDATA": true,
}

var (
	proxyEnv = map[string]bool{
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true, "NO_PROXY": true, "FTP_PROXY": true,
		"SOCKS_PROXY": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true, "CURL_CA_BUNDLE": true,
		"REQUESTS_CA_BUNDLE": true, "NODE_EXTRA_CA_CERTS": true,
	}
	credentialEnv = regexp.MustCompile(`(?i)(^|_)(TOKEN|SECRET|PASSWORD|PASSWD|PASSPHRASE|CREDENTIALS?|API_?KEY|ACCESS_?KEY|PRIVATE_?KEY|AUTH|PAT|DSN|WEBHOOK_URL|COOKIES?|SESSION)(_|$)`)
	// providerEnv are credential-style names that carry no obvious marker.
	providerEnv = map[string]bool{
		"AWS_ACCESS_KEY_ID": true, "AWS_SESSION_TOKEN": true, "GOOGLE_APPLICATION_CREDENTIALS": true,
		"AZURE_CLIENT_ID": true, "DATABASE_URL": true, "SSH_AUTH_SOCK": true, "NPM_CONFIG_USERCONFIG": true,
	}
	validEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ValidEnvName reports whether name is a portable environment variable name.
func ValidEnvName(name string) bool { return validEnvName.MatchString(name) }

// Sensitive reports whether passing the variable to a command that must not
// reach the network is unsafe: proxy settings, and anything that looks like a
// credential or a known provider key.
func Sensitive(name string) bool {
	up := strings.ToUpper(name)
	return proxyEnv[up] || providerEnv[up] || credentialEnv.MatchString(up)
}

// ScrubEnv builds a child environment from parent (KEY=VALUE entries, usually
// os.Environ()): only the base allowlist, any LC_* locale variable, and the
// names in pass survive (pass is trusted: reject Sensitive names before calling
// when the command must not reach the network). extra entries (KEY=VALUE) are added last and win.
// NO_COLOR=1 and TERM=dumb are set unless extra overrides them. The result is
// sorted, so the same inputs always give the same environment.
func ScrubEnv(parent, pass, extra []string) []string {
	allow := map[string]bool{}
	for _, n := range pass {
		allow[n] = true
	}
	out := map[string]string{"NO_COLOR": "1", "TERM": "dumb"}
	for _, kv := range parent {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		up := strings.ToUpper(name)
		if allow[name] || baseEnv[up] || (strings.HasPrefix(up, "LC_") && !Sensitive(name)) {
			out[name] = val
		}
	}
	for _, kv := range extra {
		if name, val, ok := strings.Cut(kv, "="); ok && name != "" {
			out[name] = val
		}
	}
	env := make([]string, 0, len(out))
	for name, val := range out {
		env = append(env, name+"="+val)
	}
	sort.Strings(env)
	return env
}

// HostEnv is os.Environ(), for callers that scrub the real environment.
func HostEnv() []string { return os.Environ() }
