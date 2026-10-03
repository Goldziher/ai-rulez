package generator

import (
	"net/url"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
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
	for i, arg := range server.Args {
		secrets = append(secrets, urlSecrets(arg)...)
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if !isSecretFlag(name) {
			continue
		}
		switch {
		case hasValue && value != "":
			secrets = append(secrets, value)
		case !hasValue && i+1 < len(server.Args) && !strings.HasPrefix(server.Args[i+1], "-"):
			secrets = append(secrets, server.Args[i+1])
		}
	}
	return secrets
}

func isSecretFlag(name string) bool {
	return config.IsSensitiveEnvName(strings.ReplaceAll(name, "-", "_"))
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
		} else if name := u.User.Username(); name != "" {
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
