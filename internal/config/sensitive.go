package config

import "strings"

// sensitiveEnvNameParts mark an env or header name as credential-carrying.
var sensitiveEnvNameParts = [...]string{
	"TOKEN",
	"SECRET",
	"PASSWORD",
	"KEY",
	"CREDENTIAL",
}

// sensitiveHeaderNames are credential-carrying headers whose names do not
// contain one of sensitiveEnvNameParts.
var sensitiveHeaderNames = [...]string{"AUTHORIZATION", "PROXY-AUTHORIZATION", "COOKIE"}

// IsSensitiveHeaderName reports whether an HTTP header name carries a credential.
func IsSensitiveHeaderName(name string) bool {
	upper := strings.ToUpper(name)
	for _, header := range sensitiveHeaderNames {
		if upper == header {
			return true
		}
	}
	return IsSensitiveEnvName(name)
}

// IsSensitiveEnvName reports whether an environment variable (or key) name
// looks like it holds a secret.
func IsSensitiveEnvName(name string) bool {
	upper := strings.ToUpper(name)
	for _, part := range sensitiveEnvNameParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}
