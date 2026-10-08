package preflight

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain text is untouched", in: "npx -y server", want: "npx -y server"},
		{name: "escape sequence", in: "echo \x1b[2J\x1b[Hok", want: `echo \x1b[2J\x1b[Hok`},
		{name: "carriage return and newline", in: "a\rb\nc", want: `a\rb\nc`},
		{name: "C1 control", in: "a\u009bb", want: `a\x9bb`},
		{name: "bidi override", in: "a\u202eb", want: `a\u202eb`},
		{name: "zero width", in: "a\u200bb", want: `a\u200bb`},
		{name: "invalid utf8", in: "a\xffb", want: `a\xffb`},
		{name: "printable unicode kept", in: "café", want: "café"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Sanitize(tt.in)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRedact(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		notWant string
	}{
		{name: "token flag with equals", in: "srv --token=abc123secret", notWant: "abc123secret"},
		{name: "password flag with space", in: "srv --password hunter2", notWant: "hunter2"},
		{name: "api key flag", in: "srv --api-key sk_live_zzz", notWant: "sk_" + "live_zzz"},
		{name: "bearer", in: `curl -H "Authorization: Bearer abcdef12345"`, notWant: "abcdef12345"},
		{name: "bearer alone", in: "run Bearer abcdef12345", notWant: "abcdef12345"},
		{name: "url userinfo", in: "git clone https://user:pw1234@host/repo", notWant: "pw1234"},
		{name: "secret env prefix", in: "GITHUB_TOKEN=ghx12345 npx srv", notWant: "ghx12345"},
		{name: "known prefix", in: "srv sk-abcdefghijklmnop", notWant: "sk-abcdefghijklmnop"},
		{name: "long hex", in: "srv 0123456789abcdef0123456789abcdef", notWant: "0123456789abcdef0123456789abcdef"},
		{name: "long base64", in: "srv QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU2", notWant: "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVowMTIzNDU2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := Redact(tt.in)

			// Assert
			assert.NotContains(t, got, tt.notWant)
			assert.Contains(t, got, redacted)
		})
	}

	t.Run("ordinary commands and paths are kept", func(t *testing.T) {
		in := "./scripts/guard.sh --verbose npx -y tools-server --config ./some/long/path/to/a/config/file.toml"
		assert.Equal(t, in, Redact(in))
	})
}

func TestEnvValue(t *testing.T) {
	assert.Equal(t, "NODE_OPTIONS=--require ./x.js", EnvValue("NODE_OPTIONS", "--require ./x.js"))
	assert.Equal(t, "API_TOKEN="+redacted, EnvValue("API_TOKEN", "abc"))
	assert.Equal(t, "ANTHROPIC_BASE_URL=https://"+redacted+"@evil.test/", EnvValue("ANTHROPIC_BASE_URL", "https://u:p@evil.test/"))
}

func TestDisplayTruncates(t *testing.T) {
	got := Display(strings.Repeat("a ", 200), 20)
	assert.True(t, strings.HasSuffix(got, "..."))
	assert.LessOrEqual(t, len([]rune(got)), 23)
}
