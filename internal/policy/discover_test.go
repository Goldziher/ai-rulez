package policy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

const minimalPolicy = "policy_version = 1\nname = \"%s\"\n[lint.severity_floor]\nAR008 = \"warning\"\n"

func writePolicy(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func envWith(path string) ambient.Env {
	return ambient.MapEnv{Vars: map[string]string{EnvPolicy: path}, Home: "/h"}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	flagFile := writePolicy(t, dir, "flag.toml", strings.Replace(minimalPolicy, "%s", "flag", 1))
	envFile := writePolicy(t, dir, "env.toml", strings.Replace(minimalPolicy, "%s", "env", 1))
	managed := writePolicy(t, dir, "managed.toml", strings.Replace(minimalPolicy, "%s", "managed", 1))
	bad := writePolicy(t, dir, "bad.toml", "policy_version = 1\n[lint]\nrequired_code = [\"AR001\"]\n")
	newer := writePolicy(t, dir, "newer.toml", "policy_version = 2\n")
	missing := filepath.Join(dir, "nope.toml")

	tests := []struct {
		name    string
		opts    DiscoverOptions
		origins []string
		wantErr string
		errType any
	}{
		{"nothing set", DiscoverOptions{Env: envWith(""), ManagedPaths: []string{missing}}, nil, "", nil},
		{"flag only", DiscoverOptions{Flag: flagFile, Env: envWith(""), ManagedPaths: []string{missing}}, []string{"flag"}, "", nil},
		{"env only", DiscoverOptions{Env: envWith(envFile), ManagedPaths: []string{missing}}, []string{"env"}, "", nil},
		{"managed only", DiscoverOptions{Env: envWith(""), ManagedPaths: []string{managed}}, []string{"managed"}, "", nil},
		{"all three, strongest first", DiscoverOptions{Flag: flagFile, Env: envWith(envFile), ManagedPaths: []string{managed}}, []string{"flag", "env", "managed"}, "", nil},
		{"the same file twice is one layer", DiscoverOptions{Flag: flagFile, Env: envWith(flagFile), ManagedPaths: []string{missing}}, []string{"flag"}, "", nil},
		{"flag set but unreadable fails closed", DiscoverOptions{Flag: missing, Env: envWith(""), ManagedPaths: []string{managed}}, nil, "AR742", &UnavailableError{}},
		{"env set but unreadable fails closed", DiscoverOptions{Env: envWith(missing), ManagedPaths: []string{managed}}, nil, "AR742", &UnavailableError{}},
		{"a directory is not a policy", DiscoverOptions{Flag: dir, Env: envWith("")}, nil, "AR742", &UnavailableError{}},
		{"present but invalid managed file fails closed", DiscoverOptions{Env: envWith(""), ManagedPaths: []string{bad}}, nil, "AR743", &ParseError{}},
		{"unknown key is invalid", DiscoverOptions{Flag: bad, Env: envWith("")}, nil, "required_code", &ParseError{}},
		{"a newer policy is invalid", DiscoverOptions{Flag: newer, Env: envWith("")}, nil, "upgrade ai-rulez", &ParseError{}},
		{"an unpinned URL is refused", DiscoverOptions{Flag: "https://policy.example.org/p.toml", Env: envWith("")}, nil, "AR741", nil},
		{"an http URL is refused", DiscoverOptions{Flag: "http://policy.example.org/p.toml", Env: envWith("")}, nil, "must be https", &ParseError{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			layers, err := Discover(tt.opts)
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				switch tt.errType.(type) {
				case *UnavailableError:
					var u *UnavailableError
					assert.True(t, errors.As(err, &u))
				case *ParseError:
					var p *ParseError
					assert.True(t, errors.As(err, &p))
				}
				return
			}
			require.NoError(t, err)
			var got []string
			for _, l := range layers {
				got = append(got, l.Origin)
			}
			assert.Equal(t, tt.origins, got)
		})
	}
}

func TestManagedPathsByPlatform(t *testing.T) {
	tests := []struct {
		goos string
		env  map[string]string
		want string
	}{
		{"linux", nil, "/etc/ai-rulez/policy.toml"},
		{"darwin", nil, "/Library/Application Support/ai-rulez/policy.toml"},
		{"windows", map[string]string{"ProgramData": `D:\PD`}, `D:\PD\ai-rulez\policy.toml`},
		{"windows", nil, `C:\ProgramData\ai-rulez\policy.toml`},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			got := managedPaths(DiscoverOptions{GOOS: tt.goos, Env: ambient.MapEnv{Vars: tt.env}})
			assert.Equal(t, []string{tt.want}, got)
		})
	}
}

func TestPolicyFileLimits(t *testing.T) {
	dir := t.TempDir()
	big := writePolicy(t, dir, "big.toml", "policy_version = 1\n# "+strings.Repeat("x", maxPolicyBytes))
	_, err := Discover(DiscoverOptions{Flag: big, Env: envWith("")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "larger than")
}

func TestDigestIgnoresLineEndingStyle(t *testing.T) {
	assert.Equal(t, digest([]byte("a\nb\n")), digest([]byte("a\r\nb\r\n")))
	assert.NotEqual(t, digest([]byte("a\n")), digest([]byte("b\n")))
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"no version", "name = \"x\"\n", "policy_version is required"},
		{"extends", "policy_version = 1\nextends = [\"x\"]\n", "extends is not supported"},
		{"bad severity", "policy_version = 1\n[lint.severity_floor]\nAR001 = \"off\"\n", "not a severity"},
		{"unknown rule floor", "policy_version = 1\n[lint.severity_floor]\nAR000 = \"error\"\n", "unknown rule"},
		{"unknown required rule", "policy_version = 1\n[lint]\nrequired_codes = [\"nope\"]\n", "unknown rule"},
		{"bad host", "policy_version = 1\n[sources]\nallowed_hosts = [\"https://github.com\"]\n", "not a host pattern"},
		{"path in a host list", "policy_version = 1\n[lint.security]\nallowed_hosts = [\"github.com/org\"]\n", "hosts only"},
		{"scan_imports off", "policy_version = 1\n[lint.security]\nscan_imports = \"off\"\n", "not allowed in a policy"},
		{"not toml", "policy_version =\n", "policy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Parse("p.toml", []byte(tt.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParseNormalizesAndResolvesNames(t *testing.T) {
	// Arrange
	body := "policy_version = 1\nname = \"n\"\n[sources]\nallowed_hosts = [\"GitHub.com/Example-Org\", \"github.com/example-org\"]\n" +
		"[lint]\nrequired_codes = [\"secret-detected\", \"AR001\"]\n[lint.severity_floor]\nsecret-detected = \"error\"\n" +
		"[telemetry]\nallow_network = false\n[guard]\ngenerated = true\n"
	// Act
	name, p, err := Parse("p.toml", []byte(body))
	// Assert
	require.NoError(t, err)
	assert.Equal(t, "n", name)
	assert.Equal(t, []string{"github.com/example-org"}, p.Sources.Allowed.Items)
	assert.Equal(t, []string{"AR001"}, p.Lint.RequiredCodes)
	assert.Equal(t, map[string]string{"AR001": "error"}, p.Lint.SeverityFloor)
	assert.True(t, p.Telemetry.Disabled)
	assert.True(t, p.Guard.Generated)
	assert.False(t, p.LLM.Disabled)
}

func TestParseEmptyAllowlistIsSetAndEmpty(t *testing.T) {
	_, p, err := Parse("p.toml", []byte("policy_version = 1\n[sources]\nallowed_hosts = []\n"))
	require.NoError(t, err)
	assert.True(t, p.Sources.Allowed.Set)
	assert.Empty(t, p.Sources.Allowed.Items)
}
