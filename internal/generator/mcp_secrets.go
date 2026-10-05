package generator

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// literalSecrets lists the secret values a server carries outside env and
// headers: credentials in a URL (user info, secret-looking query values) and the
// values of secret-looking flags in args (--token=VALUE, --api-key VALUE). They
// end up verbatim in generated MCP configs, so those files are guarded like env
// secrets.
func literalSecrets(server *config.MCPServer) []string {
	if server == nil {
		return nil
	}
	var secrets []string
	secrets = append(secrets, urlSecrets(server.URL)...)
	envKeys := make([]string, 0, len(server.Env))
	for key := range server.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		secrets = append(secrets, urlSecrets(server.Env[key])...)
	}
	for i, arg := range server.Args {
		secrets = append(secrets, urlSecrets(arg)...)
		secrets = append(secrets, authHeaderSecrets(arg)...)
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if hasValue {
			secrets = append(secrets, urlSecrets(value)...)
		}
		if !isSecretFlag(name) {
			continue
		}
		switch {
		case hasValue && value != "" && !isNumeric(value):
			secrets = append(secrets, value)
		case !hasValue && i+1 < len(server.Args) && !strings.HasPrefix(server.Args[i+1], "-") && !isNumeric(server.Args[i+1]):
			secrets = append(secrets, server.Args[i+1])
		}
	}
	return secrets
}

var (
	// secretFlagSuffixes are the name endings that make a flag carry a credential.
	secretFlagSuffixes = [...]string{
		"token", "-key", "apikey", "secret", "password", "passwd", "auth", "credential", "credentials",
	}
	// plainFlagSuffixes and plainFlagPrefixes name settings that merely mention a
	// credential word (--api-key-file, --max-tokens, --no-token-cache).
	plainFlagSuffixes = [...]string{"-file", "-path", "-dir", "-limit", "-count", "-size", "-cache", "-ttl", "-timeout"}
	plainFlagPrefixes = [...]string{"max-", "min-", "no-"}

	authHeaderPattern = regexp.MustCompile(`(?i)\bauthorization\s*[:=]\s*(?:(?:bearer|basic|token|digest)\s+)?(\S+)`)
)

// isSecretFlag reports whether a CLI flag name takes a credential value. It is
// stricter than the env-name check so that --max-tokens or --keyring are not
// mistaken for secrets.
func isSecretFlag(name string) bool {
	name = strings.ReplaceAll(strings.ToLower(name), "_", "-")
	for _, prefix := range plainFlagPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	for _, suffix := range plainFlagSuffixes {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}
	for _, suffix := range secretFlagSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// authHeaderSecrets returns the credential of an "Authorization: <scheme> X" text.
func authHeaderSecrets(arg string) []string {
	m := authHeaderPattern.FindStringSubmatch(arg)
	if len(m) < 2 || m[1] == "" {
		return nil
	}
	return []string{strings.Trim(m[1], `"'`)}
}

func isNumeric(value string) bool {
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

// urlSecrets returns the credentials embedded in value when it is a URL: the
// password (or the user name when it stands alone, as in https://TOKEN@host) and
// the values of secret-looking query parameters.
func urlSecrets(value string) []string {
	if !strings.Contains(value, "://") {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil {
		return nil
	}
	var secrets []string
	if u.User != nil {
		if password, ok := u.User.Password(); ok && password != "" {
			secrets = append(secrets, password)
		} else if name := u.User.Username(); name != "" && isWebScheme(u.Scheme) {
			// A lone user name is a token only on web URLs, not ssh://git@host.
			secrets = append(secrets, name)
		}
	}
	for _, pair := range strings.Split(u.RawQuery, "&") {
		name, raw, ok := strings.Cut(pair, "=")
		if !ok || raw == "" || !isSecretQueryName(name) {
			continue
		}
		secrets = append(secrets, raw)
		if decoded, decodeErr := url.QueryUnescape(raw); decodeErr == nil && decoded != raw {
			secrets = append(secrets, decoded)
		}
	}
	return secrets
}

func isWebScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "http", "https", "ws", "wss":
		return true
	}
	return false
}

func isSecretQueryName(name string) bool {
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}
	switch strings.ToUpper(name) {
	case "AUTH", "SIG", "SIGNATURE", "PWD", "PASS":
		return true
	}
	return config.IsSensitiveEnvName(name)
}

// hasLiteralSecrets reports whether a server carries URL or flag credentials.
func hasLiteralSecrets(server *config.MCPServer) bool { return len(literalSecrets(server)) > 0 }
