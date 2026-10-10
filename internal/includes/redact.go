package includes

import (
	"github.com/Goldziher/ai-rulez/v5/internal/urlredact"
)

// HasCredentials reports whether a git URL carries a credential (`user:token@`
// or a credential query parameter) that must not be written to config.toml or
// passed to git. See internal/urlredact.
func HasCredentials(raw string) bool { return urlredact.HasCredentials(raw) }

// RedactURL hides URL userinfo and the values of query-string parameters, so a
// repository address can be logged, returned to a caller or put into an error
// context without leaking the credential embedded in it. See internal/urlredact.
func RedactURL(s string) string { return urlredact.URL(s) }
