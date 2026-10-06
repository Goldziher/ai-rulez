package approval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodeowners_OwnersOf(t *testing.T) {
	file := `# a comment
*                       @default
*.md                    @docs-team
/build/logs/            @logs
docs/*                  docs@example.org
/.ai-rulez/             @acme/platform
/.ai-rulez/skills/      @acme/security @alice
/.ai-rulez/rules/legacy.md
apps/**                 @apps
**/gen/                 @gen # trailing comment
/a/**/z.txt             @deep
`
	co := ParseCodeowners([]byte(file))
	tests := []struct {
		name        string
		path        string
		want        []string
		wantMatched bool
	}{
		{"catch-all", "src/main.go", []string{"@default"}, true},
		{"extension at any depth", "src/deep/readme.md", []string{"@docs-team"}, true},
		{"anchored directory owns its tree", "build/logs/x/y.txt", []string{"@logs"}, true},
		{"anchored directory does not match deeper", "src/build/logs/x.txt", []string{"@default"}, true},
		{"directory wildcard owns direct files", "docs/guide.txt", []string{"docs@example.org"}, true},
		{"directory wildcard does not own nested files", "docs/sub/guide.txt", []string{"@default"}, true},
		{"later line wins", ".ai-rulez/skills/deploy/SKILL.md", []string{"@acme/security", "@alice"}, true},
		{"earlier directory", ".ai-rulez/config.toml", []string{"@acme/platform"}, true},
		{"a line without owners unowns the path", ".ai-rulez/rules/legacy.md", nil, true},
		{"double star suffix", "apps/web/index.js", []string{"@apps"}, true},
		{"double star prefix any depth", "x/y/gen/out.go", []string{"@gen"}, true},
		{"double star in the middle", "a/b/c/z.txt", []string{"@deep"}, true},
		{"double star in the middle, zero dirs", "a/z.txt", []string{"@deep"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, matched := co.OwnersOf(tt.path)

			// Assert
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantMatched, matched)
		})
	}
}

func TestCodeowners_NoCatchAllLeavesPathsUnowned(t *testing.T) {
	co := ParseCodeowners([]byte("/docs/ @docs\n"))

	owners, matched := co.OwnersOf("src/a.go")

	assert.Empty(t, owners)
	assert.False(t, matched)
}

func TestCodeowners_SkipsUnsupportedSyntax(t *testing.T) {
	// Arrange: negation and character classes are not CODEOWNERS syntax
	co := ParseCodeowners([]byte("*.go @go\n!vendor/ @nobody\n[ab].txt @classes\n"))

	// Act
	owners, _ := co.OwnersOf("vendor/x.go")

	// Assert
	assert.Equal(t, []int{2, 3}, co.Skipped)
	assert.Equal(t, []string{"@go"}, owners)
}

func TestCodeowners_EscapesAndCRLF(t *testing.T) {
	co := ParseCodeowners([]byte("\\#hash.txt @hash\r\nmy\\ file.txt @space\r\n"))

	hash, _ := co.OwnersOf("a/#hash.txt")
	space, _ := co.OwnersOf("my file.txt")

	assert.Equal(t, []string{"@hash"}, hash)
	assert.Equal(t, []string{"@space"}, space)
}

func TestIdentity(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Alice", "alice"},
		{"@Alice", "alice"},
		{"github:Alice", "alice"},
		{" github:@alice ", "alice"},
		{"Alice@Example.org", "alice@example.org"},
		{"key:sha256:ab", "key:sha256:ab"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, Identity(tt.in), tt.in)
	}
	assert.True(t, SameReviewer("github:alice", "@alice"))
	assert.False(t, SameReviewer("alice@example.org", "alice"))
}

func TestTeams_Matches(t *testing.T) {
	teams := NewTeams(map[string][]string{"@Acme/Security": {"alice@example.org", "github:bob"}})
	teams.Resolved = map[string][]string{"@acme/infra": {"carol"}}

	tests := []struct {
		name     string
		entries  []string
		reviewer string
		want     bool
	}{
		{"member of an explicit team", []string{"@acme/security"}, "alice@example.org", true},
		{"member by login form", []string{"@acme/security"}, "@bob", true},
		{"resolved team", []string{"@acme/infra"}, "github:carol", true},
		{"non member", []string{"@acme/security"}, "mallory", false},
		{"unknown team matches nobody", []string{"@acme/unknown"}, "alice@example.org", false},
		{"plain user entry", []string{"@dave"}, "github:dave", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, teams.Matches(tt.entries, tt.reviewer))
		})
	}
	assert.Equal(t, []string{"@acme/unknown"}, teams.Unresolved([]string{"@acme/security", "@acme/unknown", "@dave"}))
}

func TestOwnerSet_PathOfAndOwnersOf(t *testing.T) {
	// Arrange: the config directory is .ai-rulez below the CODEOWNERS root
	set := &OwnerSet{
		Codeowners: ParseCodeowners([]byte("/.ai-rulez/skills/ @skills\n/.ai-rulez/ai-rulez.lock @lockers\n")),
		Prefix:     ".ai-rulez", LockPath: ".ai-rulez/ai-rulez.lock",
	}

	// Act
	item, itemCovered := set.OwnersOf(Subject{Kind: "skill", ID: "deploy", Path: "skills/deploy"})
	remote, remoteCovered := set.OwnersOf(Subject{Kind: KindInclude, ID: "shared"})
	none, noneCovered := set.OwnersOf(Subject{Kind: "rule", ID: "style", Path: "rules/style.md"})

	// Assert
	assert.Equal(t, []string{"@skills"}, item)
	assert.True(t, itemCovered)
	assert.Equal(t, []string{"@lockers"}, remote, "remote content is owned through the lock path")
	assert.True(t, remoteCovered)
	assert.Empty(t, none)
	assert.False(t, noneCovered)
}
