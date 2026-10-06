package semver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParse(t *testing.T, s string) Version {
	t.Helper()
	v, err := Parse(s)
	require.NoError(t, err, s)
	return v
}

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		invalid bool
	}{
		{in: "0.0.0", want: "0.0.0"},
		{in: "1.2.3", want: "1.2.3"},
		{in: "10.20.30", want: "10.20.30"},
		{in: "1.2.3-alpha.1", want: "1.2.3-alpha.1"},
		{in: "1.2.3-0.3.7", want: "1.2.3-0.3.7"},
		{in: "1.2.3-x.7.z.92", want: "1.2.3-x.7.z.92"},
		{in: "1.2.3-a-b", want: "1.2.3-a-b"},
		{in: "1.0.0+20130313144700", want: "1.0.0+20130313144700"},
		{in: "1.0.0-beta+exp.sha.5114f85", want: "1.0.0-beta+exp.sha.5114f85"},
		{in: "1.2", invalid: true},
		{in: "1", invalid: true},
		{in: "1.2.3.4", invalid: true},
		{in: "01.2.3", invalid: true},
		{in: "1.02.3", invalid: true},
		{in: "1.2.03", invalid: true},
		{in: "v1.2.3", invalid: true},
		{in: "1.2.3-", invalid: true},
		{in: "1.2.3-01", invalid: true},
		{in: "1.2.3-a..b", invalid: true},
		{in: "1.2.3+", invalid: true},
		{in: "1.2.3+a..b", invalid: true},
		{in: "1.2.-3", invalid: true},
		{in: "1.2.x", invalid: true},
		{in: "", invalid: true},
		{in: "99999999999999999999.0.0", invalid: true},
		{in: "1.2.3-é", invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.invalid {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestCompareFollowsTheSpecificationPrecedence(t *testing.T) {
	// semver.org section 11, in ascending order.
	ordered := []string{
		"0.9.9", "1.0.0-0", "1.0.0-1", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta",
		"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0", "10.0.0",
	}
	for i, a := range ordered {
		for j, b := range ordered {
			want := 0
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			assert.Equal(t, want, mustParse(t, a).Compare(mustParse(t, b)), "%s vs %s", a, b)
		}
	}
}

func TestCompareIgnoresBuildMetadata(t *testing.T) {
	assert.Equal(t, 0, mustParse(t, "1.0.0+a").Compare(mustParse(t, "1.0.0+b")))
	assert.Equal(t, 0, mustParse(t, "1.0.0+a").Compare(mustParse(t, "1.0.0")))
}

func TestParseTag(t *testing.T) {
	tests := []struct {
		name, prefix string
		want         string
		ok           bool
	}{
		{"1.2.3", "", "1.2.3", true},
		{"v1.2.3", "", "1.2.3", true},
		{"v1.2.3-rc.1", "", "1.2.3-rc.1", true},
		{"vv1.2.3", "", "", false},
		{"release-1.2.3", "", "", false},
		{"1.2", "", "", false},
		{"latest", "", "", false},
		{"deploy/v2.1.3", "deploy/v", "2.1.3", true},
		{"deploy/v2.1.3", "", "", false},
		{"v2.1.3", "deploy/v", "", false},
		{"deploy/v2.1", "deploy/v", "", false},
		{"v1.2.3", "v", "1.2.3", true},
		{"vv1.2.3", "v", "", false},
		{"rel-1.2.3", "rel-", "1.2.3", true},
	}
	for _, tt := range tests {
		t.Run(tt.prefix+"|"+tt.name, func(t *testing.T) {
			got, ok := ParseTag(tt.name, tt.prefix)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got.String())
			}
		})
	}
}

func TestConstraintSyntax(t *testing.T) {
	valid := []string{
		"1.2.3", "=1.2.3", "v1.2.3", "^1.2", "~2.1.0", "~>2.1", ">=1.4.0 <2.0.0", ">= 1.4.0 < 2.0.0", "1.x", "1.2.x", "1.X",
		"*", "x", "^1.x || 3", "1.2.3 - 2.3.4", "1 - 2", ">1", "<=1.2", "^0.0.3", "^1.2.3-beta.2", ">=1.0.0-0",
		"  ^1  ", "^*", "~*", ">=*",
	}
	for _, s := range valid {
		t.Run("valid "+s, func(t *testing.T) {
			c, err := ParseConstraint(s)
			require.NoError(t, err)
			assert.NotEmpty(t, c.String())
		})
	}
	invalid := []string{
		"", " ", "||", "1.2.3 ||", "|| 1.2.3", "^", ">=", "1.2.3.4", "1.x.3", "a.b.c", "latest", "^^1", ">*", "<*",
		"1.2.3 - ", "- 1.2.3", "1.2.3-", "01.2.3", "^1.2.3-01", ">=1.2.3 foo", "1.2-beta", "x.1", "~>",
	}
	for _, s := range invalid {
		t.Run("invalid "+s, func(t *testing.T) {
			_, err := ParseConstraint(s)
			assert.Error(t, err)
		})
	}
}

func TestConstraintCheck(t *testing.T) {
	tests := []struct {
		constraint string
		match      []string
		reject     []string
	}{
		{"1.2.3", []string{"1.2.3", "1.2.3+b"}, []string{"1.2.4", "1.2.2", "1.2.3-rc.1"}},
		{"=1.2.3", []string{"1.2.3"}, []string{"1.2.4"}},
		{"^1.2.3", []string{"1.2.3", "1.2.4", "1.9.0", "1.99.99"}, []string{"1.2.2", "2.0.0", "0.9.9", "2.0.0-alpha"}},
		{"^1.2", []string{"1.2.0", "1.2.9", "1.5.0"}, []string{"1.1.9", "2.0.0"}},
		{"^1", []string{"1.0.0", "1.9.9"}, []string{"0.9.9", "2.0.0"}},
		{"^1.x", []string{"1.0.0", "1.9.9"}, []string{"2.0.0"}},
		{"^0.2.3", []string{"0.2.3", "0.2.9"}, []string{"0.2.2", "0.3.0", "1.0.0"}},
		{"^0.2", []string{"0.2.0", "0.2.9"}, []string{"0.3.0", "0.1.9"}},
		{"^0.0.3", []string{"0.0.3"}, []string{"0.0.4", "0.0.2", "0.1.0"}},
		{"^0.0", []string{"0.0.0", "0.0.9"}, []string{"0.1.0"}},
		{"^0", []string{"0.0.0", "0.9.9"}, []string{"1.0.0"}},
		{"~1.2.3", []string{"1.2.3", "1.2.9"}, []string{"1.3.0", "1.2.2"}},
		{"~1.2", []string{"1.2.0", "1.2.9"}, []string{"1.3.0", "1.1.9"}},
		{"~1", []string{"1.0.0", "1.9.9"}, []string{"2.0.0", "0.9.9"}},
		{"~0.2.3", []string{"0.2.3", "0.2.9"}, []string{"0.3.0"}},
		{"~>2.1.0", []string{"2.1.0", "2.1.7"}, []string{"2.2.0"}},
		{">=1.4.0 <2.0.0", []string{"1.4.0", "1.9.9"}, []string{"1.3.9", "2.0.0", "2.0.0-alpha"}},
		{">1.2.3", []string{"1.2.4", "2.0.0"}, []string{"1.2.3", "1.2.2"}},
		{">1.2", []string{"1.3.0", "2.0.0"}, []string{"1.2.9", "1.2.0"}},
		{">1", []string{"2.0.0"}, []string{"1.9.9"}},
		{">=1.2", []string{"1.2.0", "3.0.0"}, []string{"1.1.9"}},
		{"<1.2.3", []string{"1.2.2", "0.1.0"}, []string{"1.2.3", "1.2.4"}},
		{"<1.2", []string{"1.1.9"}, []string{"1.2.0", "1.2.0-rc.1"}},
		{"<=1.2.3", []string{"1.2.3", "1.0.0"}, []string{"1.2.4"}},
		{"<=1.2", []string{"1.2.9"}, []string{"1.3.0"}},
		{"<=1", []string{"1.9.9"}, []string{"2.0.0"}},
		{"1.x", []string{"1.0.0", "1.9.9"}, []string{"0.9.9", "2.0.0"}},
		{"1.2.x", []string{"1.2.0", "1.2.9"}, []string{"1.3.0", "1.1.9"}},
		{"1.2", []string{"1.2.5"}, []string{"1.3.0"}},
		{"1", []string{"1.5.0"}, []string{"2.0.0"}},
		{"*", []string{"0.0.0", "99.0.0"}, []string{"1.0.0-rc.1"}},
		{"x", []string{"1.0.0"}, nil},
		{"1.x || 3.x", []string{"1.5.0", "3.1.0"}, []string{"2.0.0", "4.0.0"}},
		{"^1.2 || >=3.5.0", []string{"1.2.0", "3.5.0", "9.0.0"}, []string{"2.9.0", "3.4.9"}},
		{"1.2.3 - 2.3.4", []string{"1.2.3", "2.0.0", "2.3.4"}, []string{"1.2.2", "2.3.5"}},
		{"1.2 - 2.3", []string{"1.2.0", "2.3.9"}, []string{"1.1.9", "2.4.0"}},
		{"1 - 2", []string{"1.0.0", "2.9.9"}, []string{"3.0.0", "0.9.9"}},
		{">= 1.0.0 < 1.5.0", []string{"1.2.0"}, []string{"1.5.0"}},
	}
	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			require.NoError(t, err)
			for _, s := range tt.match {
				assert.True(t, c.Check(mustParse(t, s), false), "%s should satisfy %s", s, tt.constraint)
			}
			for _, s := range tt.reject {
				assert.False(t, c.Check(mustParse(t, s), false), "%s should not satisfy %s", s, tt.constraint)
			}
		})
	}
}

func TestConstraintPrereleaseRules(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		version    string
		include    bool
		want       bool
	}{
		{"a prerelease is excluded by default", "^1.2.0", "1.3.0-beta.1", false, false},
		{"a prerelease is admitted when asked", "^1.2.0", "1.3.0-beta.1", true, true},
		{"include_prerelease still honors the bounds", "^1.2.0", "2.0.0-beta.1", true, false},
		{"include_prerelease does not admit below the lower bound", "^1.2.0", "1.1.0-beta.1", true, false},
		{"a constraint naming a prerelease admits that tuple", "^1.2.3-beta.2", "1.2.3-beta.4", false, true},
		{"and not an earlier prerelease", "^1.2.3-beta.2", "1.2.3-beta.1", false, false},
		{"and not a prerelease of another tuple", "^1.2.3-beta.2", "1.3.0-beta.1", false, false},
		{"the release of that tuple is admitted", "^1.2.3-beta.2", "1.2.3", false, true},
		{"a later release is admitted", "^1.2.3-beta.2", "1.4.0", false, true},
		{">= naming a prerelease admits only that tuple's prereleases", ">=1.0.0-rc.1", "1.0.0-rc.2", false, true},
		{"and not other tuples", ">=1.0.0-rc.1", "1.1.0-rc.2", false, false},
		{"the wildcard excludes prereleases", "*", "1.0.0-rc.1", false, false},
		{"the wildcard admits them when asked", "*", "1.0.0-rc.1", true, true},
		{"the next major's prerelease is outside a caret range", "^1.2.3", "2.0.0-alpha", true, false},
		{"a prerelease alternative in || is evaluated per set", "1.x || ^2.0.0-rc.1", "2.0.0-rc.2", false, true},
		{"build metadata is ignored", "1.2.3", "1.2.3+build.5", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			require.NoError(t, err)
			assert.Equal(t, tt.want, c.Check(mustParse(t, tt.version), tt.include))
		})
	}
}
