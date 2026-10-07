package versionpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const base = `# project config
version = "5.0"
name = "p"

[lock]
min_release_age = "7d"

[[includes]]
name = "shared"   # the shared rules
source = "https://github.com/example-org/ai-rules"
version = "^1.2"  # stay on 1.x
tag_prefix = "v"

[[installed_skills]]
name = "shared"
source = "https://github.com/example-org/skills"
version = "~2.1.0"

[[skill_sources]]
name = "acme"
url = "https://github.com/example-org/skills"
ref = "^1.4"
`

func TestSetConstraint(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		table      string
		entry      string
		constraint string
		want       string
		wantErr    string
	}{
		{
			name: "version key keeps comments and neighbors", src: base, table: "includes", entry: "shared", constraint: "^2.0",
			want: `version = "^2.0"  # stay on 1.x`,
		},
		{
			name: "same name in another table is untouched", src: base, table: "installed_skills", entry: "shared", constraint: "~3.0.0",
			want: `version = "~3.0.0"`,
		},
		{
			name: "ref shorthand stays the shorthand", src: base, table: "skill_sources", entry: "acme", constraint: "^2.0",
			want: `ref = "^2.0"`,
		},
		{
			name: "literal string keeps its quotes", src: "[[includes]]\nname = 'a'\nsource = 'x'\nversion = '^1'\n", table: "includes", entry: "a", constraint: "^2",
			want: "version = '^2'",
		},
		{
			name: "CRLF line endings survive", src: "[[includes]]\r\nname = \"a\"\r\nsource = \"x\"\r\nversion = \"^1\"\r\n", table: "includes", entry: "a", constraint: "^2",
			want: "version = \"^2\"\r\n",
		},
		{
			name:  "a nested array element line is not a table header",
			src:   "[[includes]]\nname = \"a\"\nsource = \"x\"\npaths = [\n  [\"x\"],\n  [\"y\"]\n]\nversion = \"^1\"\n",
			table: "includes", entry: "a", constraint: "^2", want: `version = "^2"`,
		},
		{name: "missing entry", src: base, table: "includes", entry: "nope", constraint: "^2", wantErr: `no [[includes]] entry named "nope"`},
		{
			name: "duplicate names are ambiguous", src: "[[includes]]\nname = \"a\"\nversion = \"^1\"\n[[includes]]\nname = \"a\"\nversion = \"^1\"\n",
			table: "includes", entry: "a", constraint: "^2", wantErr: "2 [[includes]] entries are named",
		},
		{
			name: "a plain ref is not a constraint line", src: "[[includes]]\nname = \"a\"\nsource = \"x\"\nref = \"main\"\n", table: "includes", entry: "a", constraint: "^2",
			wantErr: "no `version",
		},
		{
			name: "inline table form is refused, not guessed", src: "includes = [{ name = \"a\", version = \"^1\" }]\n", table: "includes", entry: "a", constraint: "^2",
			wantErr: "no [[includes]] entry",
		},
		{name: "unknown table", src: base, table: "rules", entry: "a", constraint: "^2", wantErr: "not a table of sources"},
		{name: "quote in the constraint", src: base, table: "includes", entry: "shared", constraint: `^2"`, wantErr: "refusing to write"},
		{name: "empty constraint", src: base, table: "includes", entry: "shared", constraint: " ", wantErr: "refusing to write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, err := SetConstraint([]byte(tt.src), tt.table, tt.entry, tt.constraint)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, string(got), tt.want)
		})
	}
}

func TestSetConstraintChangesOneLineOnly(t *testing.T) {
	got, err := SetConstraint([]byte(base), "includes", "shared", "^2.0")
	require.NoError(t, err)

	want := `# project config
version = "5.0"
name = "p"

[lock]
min_release_age = "7d"

[[includes]]
name = "shared"   # the shared rules
source = "https://github.com/example-org/ai-rules"
version = "^2.0"  # stay on 1.x
tag_prefix = "v"

[[installed_skills]]
name = "shared"
source = "https://github.com/example-org/skills"
version = "~2.1.0"

[[skill_sources]]
name = "acme"
url = "https://github.com/example-org/skills"
ref = "^1.4"
`
	assert.Equal(t, want, string(got))
}
