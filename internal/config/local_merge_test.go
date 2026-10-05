package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func decodeDoc(t *testing.T, src string) map[string]any {
	t.Helper()
	doc := map[string]any{}
	if err := toml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("decode toml: %v", err)
	}
	return doc
}

func jsonEqual(t *testing.T, a, b string) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		t.Fatalf("bad json %s: %v", a, err)
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		t.Fatalf("bad json %s: %v", b, err)
	}
	return reflect.DeepEqual(av, bv)
}

func TestMergeConfigDocs(t *testing.T) {
	tests := []struct {
		name         string
		shared       string
		local        string
		want         string // JSON of the expected merged document
		wantWarnings []string
		wantErr      string
	}{
		{
			name:   "scalar local wins",
			shared: `name = "shared"`,
			local:  `name = "mine"`,
			want:   `{"name":"mine"}`,
		},
		{
			name:   "absent key keeps shared",
			shared: "name = \"shared\"\ndescription = \"d\"",
			local:  `name = "mine"`,
			want:   `{"name":"mine","description":"d"}`,
		},
		{
			name:   "empty string and false override",
			shared: "description = \"d\"\ngitignore = true",
			local:  "description = \"\"\ngitignore = false",
			want:   `{"description":"","gitignore":false}`,
		},
		{
			name:    "version mismatch errors",
			shared:  `version = "4.0"`,
			local:   `version = "3.0"`,
			wantErr: "does not match",
		},
		{
			name:   "version equal ok",
			shared: `version = "4.0"`,
			local:  `version = "4.0"`,
			want:   `{"version":"4.0"}`,
		},
		{
			name:   "builtins replaced",
			shared: `builtins = ["go", "rust"]`,
			local:  `builtins = ["python"]`,
			want:   `{"builtins":["python"]}`,
		},
		{
			name:   "builtins bool replaces list",
			shared: `builtins = ["go"]`,
			local:  `builtins = false`,
			want:   `{"builtins":false}`,
		},
		{
			name:   "profiles per key and list replaced",
			shared: "[profiles]\nbackend = [\"a\", \"b\"]\nfront = [\"c\"]",
			local:  "[profiles]\nbackend = [\"x\"]\nqa = [\"q\"]",
			want:   `{"profiles":{"backend":["x"],"front":["c"],"qa":["q"]}}`,
		},
		{
			name:   "defaults effort_by_preset per key",
			shared: "[defaults]\nmodel = \"m\"\n[defaults.effort_by_preset]\nclaude = \"high\"\ncursor = \"low\"",
			local:  "[defaults.effort_by_preset]\ncursor = \"max\"",
			want:   `{"defaults":{"model":"m","effort_by_preset":{"claude":"high","cursor":"max"}}}`,
		},
		{
			name:   "header deep merge",
			shared: "[header]\nstyle = \"minimal\"\nhashes = \"full\"",
			local:  "[header]\nhashes = \"none\"",
			want:   `{"header":{"style":"minimal","hashes":"none"}}`,
		},
		{
			name:   "presets union order",
			shared: `presets = ["claude", "cursor"]`,
			local:  `presets = ["cursor", "devin", "claude", "codex"]`,
			want:   `{"presets":["claude","cursor","devin","codex"]}`,
		},
		{
			name:   "presets drop with bang",
			shared: `presets = ["claude", "cursor"]`,
			local:  `presets = ["!cursor", "codex"]`,
			want:   `{"presets":["claude","codex"]}`,
		},
		{
			name:         "presets drop unknown warns",
			shared:       `presets = ["claude"]`,
			local:        `presets = ["!nope"]`,
			want:         `{"presets":["claude"]}`,
			wantWarnings: []string{`local config drops preset "nope" which is not in the shared config`},
		},
		{
			name:   "custom preset object merged by name",
			shared: "presets = [\"claude\", {name = \"mine\", path = \"A.md\", format = \"md\"}]",
			local:  "presets = [{name = \"mine\", path = \"B.md\"}]",
			want:   `{"presets":["claude",{"name":"mine","path":"B.md","format":"md"}]}`,
		},
		{
			name: "mcp_servers field by field",
			shared: `
[[mcp_servers]]
name = "a"
command = "npx"
args = ["-y", "a"]
[mcp_servers.env]
K1 = "1"
K2 = "2"
`,
			local: `
[[mcp_servers]]
name = "a"
args = ["b"]
[mcp_servers.env]
K2 = "x"
K3 = "3"
`,
			want: `{"mcp_servers":[{"name":"a","command":"npx","args":["b"],"env":{"K1":"1","K2":"x","K3":"3"}}]}`,
		},
		{
			name: "enabled false kept",
			shared: `
[[mcp_servers]]
name = "a"
command = "x"
`,
			local: `
[[mcp_servers]]
name = "a"
enabled = false
`,
			want: `{"mcp_servers":[{"name":"a","command":"x","enabled":false}]}`,
		},
		{
			name: "remove true drops",
			shared: `
[[includes]]
name = "a"
source = "s"
[[includes]]
name = "b"
source = "t"
`,
			local: `
[[includes]]
name = "a"
remove = true
`,
			want: `{"includes":[{"name":"b","source":"t"}]}`,
		},
		{
			name:   "remove unknown warns",
			shared: `name = "x"`,
			local: `
[[plugins]]
name = "ghost"
remove = true
`,
			want:         `{"name":"x"}`,
			wantWarnings: []string{`local config removes plugins entry "ghost" which is not in the shared config`},
		},
		{
			name: "new entries appended in local order with remove stripped",
			shared: `
[[mcp_servers]]
name = "a"
command = "x"
`,
			local: `
[[mcp_servers]]
name = "z"
command = "zz"
remove = false
[[mcp_servers]]
name = "b"
command = "bb"
`,
			want: `{"mcp_servers":[{"name":"a","command":"x"},{"name":"z","command":"zz"},{"name":"b","command":"bb"}]}`,
		},
		{
			name:   "duplicate local names error",
			shared: `name = "x"`,
			local: `
[[mcp_servers]]
name = "a"
[[mcp_servers]]
name = "a"
`,
			wantErr: "duplicate mcp_servers entry",
		},
		{
			name: "ambiguous shared name error",
			shared: `
[[mcp_servers]]
name = "a"
command = "1"
[[mcp_servers]]
name = "a"
command = "2"
`,
			local: `
[[mcp_servers]]
name = "a"
command = "3"
`,
			wantErr: "ambiguous",
		},
		{
			name: "scopes keyed by path when name empty",
			shared: `
[[scopes]]
path = "svc/a"
profile = "p1"
[[scopes]]
path = "svc/b"
`,
			local: `
[[scopes]]
path = "svc/a"
profile = "p2"
presets = ["claude"]
[[scopes]]
path = "svc/b"
remove = true
`,
			want: `{"scopes":[{"path":"svc/a","profile":"p2","presets":["claude"]}]}`,
		},
		{
			name:    "unknown top-level key errors",
			shared:  `name = "x"`,
			local:   "nmea = \"y\"\nfoo = 1",
			wantErr: "foo, nmea",
		},
		{
			name:   "scope matched by path when local has no name",
			shared: "[[scopes]]\nname = \"api\"\npath = \"svc/api\"\nprofile = \"p1\"",
			local:  "[[scopes]]\npath = \"svc/api\"\nprofile = \"p2\"",
			want:   `{"scopes":[{"name":"api","path":"svc/api","profile":"p2"}]}`,
		},
		{
			name:    "named scope with shared path yields duplicate path",
			shared:  "[[scopes]]\nname = \"api\"\npath = \"svc/api\"",
			local:   "[[scopes]]\nname = \"other\"\npath = \"svc/api\"",
			wantErr: "duplicate scopes path",
		},
		{
			name:    "preset table with remove errors",
			shared:  `presets = ["claude"]`,
			local:   `presets = [{name = "mine", remove = true}]`,
			wantErr: "(keys: name, remove) uses remove",
		},
		{
			name:    "bare bang preset errors",
			shared:  `presets = ["claude"]`,
			local:   `presets = ["!"]`,
			wantErr: "empty name",
		},
		{
			name:   "preset object wins over string and keeps first position",
			shared: `presets = ["claude", "mine", "cursor"]`,
			local:  `presets = [{name = "mine", path = "B.md"}]`,
			want:   `{"presets":["claude",{"name":"mine","path":"B.md"},"cursor"]}`,
		},
		{
			name:   "preset object in shared plus string in local keeps object",
			shared: `presets = [{name = "mine", path = "A.md"}]`,
			local:  `presets = ["mine"]`,
			want:   `{"presets":[{"name":"mine","path":"A.md"}]}`,
		},
		{
			name:    "non-boolean remove errors",
			shared:  "[[includes]]\nname = \"a\"",
			local:   "[[includes]]\nname = \"a\"\nremove = \"yes\"",
			wantErr: "expected boolean",
		},
		{
			name:    "non-string version errors",
			shared:  `version = "4"`,
			local:   `version = 4`,
			wantErr: "expected string",
		},
		{
			name:    "local entry without name errors without leaking values",
			shared:  `name = "x"`,
			local:   "[[mcp_servers]]\ncommand = \"c\"\n[mcp_servers.env]\nTOKEN = \"hunter2\"",
			wantErr: "mcp_servers entry #1 (keys: command, env) has none",
		},
		{
			name:    "header scalar vs table errors",
			shared:  "[header]\nstyle = \"minimal\"",
			local:   `header = "oops"`,
			wantErr: "key header has type string, expected table",
		},
		{
			name:    "presets not a list errors",
			shared:  `presets = ["claude"]`,
			local:   `presets = "cursor"`,
			wantErr: "key presets has type string, expected list",
		},
		{
			name:   "dollar schema normalized",
			shared: `schema = "a"`,
			local:  `"$schema" = "b"`,
			want:   `{"schema":"b"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			shared := decodeDoc(t, tt.shared)
			local := decodeDoc(t, tt.local)

			// Act
			got, warnings, err := MergeConfigDocs(shared, local)

			// Assert
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotJSON, _ := json.Marshal(got)
			if !jsonEqual(t, string(gotJSON), tt.want) {
				t.Errorf("merged = %s, want %s", gotJSON, tt.want)
			}
			if !reflect.DeepEqual(warnings, tt.wantWarnings) {
				t.Errorf("warnings = %v, want %v", warnings, tt.wantWarnings)
			}
		})
	}
}

const mergeFixtureShared = `
name = "p"
presets = ["claude", {name = "mine", path = "A.md"}]
[header]
style = "minimal"
[defaults.effort_by_preset]
claude = "high"
[profiles]
a = ["x"]
[[mcp_servers]]
name = "s"
args = ["1"]
[mcp_servers.env]
K = "v"
`

const mergeFixtureLocal = `
presets = ["!claude", "cursor", {name = "mine", path = "B.md"}]
[header]
hashes = "none"
[defaults.effort_by_preset]
claude = "max"
[profiles]
a = ["y"]
[[mcp_servers]]
name = "s"
args = ["2"]
[mcp_servers.env]
K = "w"
[[mcp_servers]]
name = "t"
remove = false
`

func TestMergeConfigDocs_DoesNotMutateInputs(t *testing.T) {
	// Arrange
	shared := decodeDoc(t, mergeFixtureShared)
	local := decodeDoc(t, mergeFixtureLocal)
	sharedBefore := decodeDoc(t, mergeFixtureShared)
	localBefore := decodeDoc(t, mergeFixtureLocal)

	// Act
	merged, _, err := MergeConfigDocs(shared, local)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Mutate the result deeply; inputs must stay untouched.
	merged["profiles"].(map[string]any)["a"].([]any)[0] = "changed"
	merged["mcp_servers"].([]any)[0].(map[string]any)["env"].(map[string]any)["K"] = "changed"
	merged["presets"].([]any)[0].(map[string]any)["path"] = "changed"
	merged["header"].(map[string]any)["style"] = "changed"
	merged["defaults"].(map[string]any)["effort_by_preset"].(map[string]any)["claude"] = "changed"

	// Assert
	if !reflect.DeepEqual(shared, sharedBefore) {
		t.Errorf("shared mutated: %v", shared)
	}
	if !reflect.DeepEqual(local, localBefore) {
		t.Errorf("local mutated: %v", local)
	}
}

func TestMergeConfigDocs_Deterministic(t *testing.T) {
	var first string
	for i := 0; i < 50; i++ {
		// Arrange
		shared := decodeDoc(t, mergeFixtureShared)
		local := decodeDoc(t, mergeFixtureLocal)

		// Act
		merged, warnings, err := MergeConfigDocs(shared, local)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out, err := json.Marshal(map[string]any{"m": merged, "w": warnings})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		// Assert
		if i == 0 {
			first = string(out)
			continue
		}
		if string(out) != first {
			t.Fatalf("run %d differs:\n%s\n%s", i, out, first)
		}
	}
}
