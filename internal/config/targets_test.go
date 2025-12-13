package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestMatchesTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputPath string
		targets    []string
		expected   bool
	}{
		{
			name:       "empty targets returns true",
			outputPath: "CLAUDE.md",
			targets:    []string{},
			expected:   true,
		},
		{
			name:       "nil targets returns true",
			outputPath: "CLAUDE.md",
			targets:    nil,
			expected:   true,
		},
		{
			name:       "exact match",
			outputPath: "CLAUDE.md",
			targets:    []string{"CLAUDE.md"},
			expected:   true,
		},
		{
			name:       "exact match with path",
			outputPath: "docs/README.md",
			targets:    []string{"docs/README.md"},
			expected:   true,
		},
		{
			name:       "glob pattern *.md matches",
			outputPath: "CLAUDE.md",
			targets:    []string{"*.md"},
			expected:   true,
		},
		{
			name:       "glob pattern *.md does not match with path",
			outputPath: "docs/README.md",
			targets:    []string{"*.md"},
			expected:   false,
		},
		{
			name:       "glob pattern docs/* matches",
			outputPath: "docs/README.md",
			targets:    []string{"docs/*"},
			expected:   true,
		},
		{
			name:       "glob pattern .claude/* matches",
			outputPath: ".claude/agent.md",
			targets:    []string{".claude/*"},
			expected:   true,
		},
		{
			name:       "multiple targets, first matches",
			outputPath: "CLAUDE.md",
			targets:    []string{"CLAUDE.md", "README.md"},
			expected:   true,
		},
		{
			name:       "multiple targets, second matches",
			outputPath: "README.md",
			targets:    []string{"CLAUDE.md", "README.md"},
			expected:   true,
		},
		{
			name:       "no match",
			outputPath: "CLAUDE.md",
			targets:    []string{"README.md"},
			expected:   false,
		},
		{
			name:       "glob pattern *.txt doesn't match .md",
			outputPath: "CLAUDE.md",
			targets:    []string{"*.txt"},
			expected:   false,
		},
		{
			name:       "directory pattern doesn't match file in different dir",
			outputPath: "src/main.go",
			targets:    []string{"docs/*"},
			expected:   false,
		},
		{
			name:       "directory path matches ** pattern",
			outputPath: ".claude/agents",
			targets:    []string{".claude/**/*.md"},
			expected:   true,
		},
		{
			name:       "directory path with trailing slash matches ** pattern",
			outputPath: ".claude/agents/",
			targets:    []string{".claude/**/*.md"},
			expected:   true,
		},
		{
			name:       "nested directory path matches ** pattern",
			outputPath: ".claude/foo/bar",
			targets:    []string{".claude/**/*.md"},
			expected:   true,
		},
		{
			name:       "subdirectory path does not match simple wildcard pattern",
			outputPath: ".claude",
			targets:    []string{".claude/*.md"},
			expected:   false,
		},
		{
			name:       "nested subdirectory path matches simple wildcard pattern",
			outputPath: ".claude/agents",
			targets:    []string{".claude/*.md"},
			expected:   true,
		},
		{
			name:       "empty string in targets is ignored",
			outputPath: "CLAUDE.md",
			targets:    []string{"", "CLAUDE.md"},
			expected:   true,
		},
		{
			name:       "whitespace-only target is ignored",
			outputPath: "CLAUDE.md",
			targets:    []string{"  ", "CLAUDE.md"},
			expected:   true,
		},
		{
			name:       "complex path with glob",
			outputPath: "ai/agents/code-reviewer.md",
			targets:    []string{"ai/agents/*.md"},
			expected:   true,
		},
		{
			name:       "nested directory pattern",
			outputPath: "docs/api/endpoints.md",
			targets:    []string{"docs/**"},
			expected:   false,
		},
		{
			name:       "single star in middle of path",
			outputPath: "docs/v1/api.md",
			targets:    []string{"docs/*/api.md"},
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := config.MatchesTarget(tt.outputPath, tt.targets)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterRules(t *testing.T) {
	t.Parallel()

	rules := []config.Rule{
		{
			Name:    "Global Rule",
			Content: "Applies to all outputs",
			Targets: []string{},
		},
		{
			Name:    "Claude Only",
			Content: "Only for CLAUDE.md",
			Targets: []string{"CLAUDE.md"},
		},
		{
			Name:    "Markdown Files",
			Content: "All markdown files",
			Targets: []string{"*.md"},
		},
		{
			Name:    "Docs Directory",
			Content: "Files in docs directory",
			Targets: []string{"docs/*"},
		},
		{
			Name:    "Specific Path",
			Content: "Specific file only",
			Targets: []string{"ai/agents/test.md"},
		},
	}

	tests := []struct {
		name       string
		outputPath string
		expected   []string
	}{
		{
			name:       "CLAUDE.md gets global, claude-specific, and markdown rules",
			outputPath: "CLAUDE.md",
			expected:   []string{"Global Rule", "Claude Only", "Markdown Files"},
		},
		{
			name:       "README.md gets global and markdown rules",
			outputPath: "README.md",
			expected:   []string{"Global Rule", "Markdown Files"},
		},
		{
			name:       "docs/api.md gets global and docs rules",
			outputPath: "docs/api.md",
			expected:   []string{"Global Rule", "Docs Directory"},
		},
		{
			name:       "ai/agents/test.md gets global and specific rules",
			outputPath: "ai/agents/test.md",
			expected:   []string{"Global Rule", "Specific Path"},
		},
		{
			name:       "src/main.go gets only global rule",
			outputPath: "src/main.go",
			expected:   []string{"Global Rule"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			filtered, err := config.FilterRules(rules, tt.outputPath, nil)
			assert.NoError(t, err)

			var ruleNames []string
			for _, rule := range filtered {
				ruleNames = append(ruleNames, rule.Name)
			}

			assert.ElementsMatch(t, tt.expected, ruleNames)
		})
	}
}

func TestFilterSections(t *testing.T) {
	t.Parallel()

	sections := []config.Section{
		{
			Name:    "Global Section",
			Content: "Appears everywhere",
			Targets: []string{},
		},
		{
			Name:    "Claude Intro",
			Content: "Only in CLAUDE.md",
			Targets: []string{"CLAUDE.md"},
		},
		{
			Name:    "Documentation Header",
			Content: "For all docs",
			Targets: []string{"docs/*"},
		},
	}

	tests := []struct {
		name       string
		outputPath string
		expected   []string
	}{
		{
			name:       "CLAUDE.md gets global and claude sections",
			outputPath: "CLAUDE.md",
			expected:   []string{"Global Section", "Claude Intro"},
		},
		{
			name:       "docs/README.md gets global and docs sections",
			outputPath: "docs/README.md",
			expected:   []string{"Global Section", "Documentation Header"},
		},
		{
			name:       "other.md gets only global section",
			outputPath: "other.md",
			expected:   []string{"Global Section"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			filtered, err := config.FilterSections(sections, tt.outputPath, nil)
			assert.NoError(t, err)

			var sectionTitles []string
			for _, section := range filtered {
				sectionTitles = append(sectionTitles, section.Name)
			}

			assert.ElementsMatch(t, tt.expected, sectionTitles)
		})
	}
}

func TestFilterAgents(t *testing.T) {
	t.Parallel()

	agents := []config.Agent{
		{
			Name:        "Universal Agent",
			Description: "Works everywhere",
			Targets:     []string{},
		},
		{
			Name:        "Claude Assistant",
			Description: "Only for Claude",
			Targets:     []string{"CLAUDE.md"},
		},
		{
			Name:        "Code Reviewer",
			Description: "For agent files",
			Targets:     []string{"ai/agents/*"},
		},
	}

	tests := []struct {
		name       string
		outputPath string
		expected   []string
	}{
		{
			name:       "CLAUDE.md gets universal and claude agents",
			outputPath: "CLAUDE.md",
			expected:   []string{"Universal Agent", "Claude Assistant"},
		},
		{
			name:       "ai/agents/reviewer.md gets universal and code reviewer agents",
			outputPath: "ai/agents/reviewer.md",
			expected:   []string{"Universal Agent", "Code Reviewer"},
		},
		{
			name:       "other.md gets only universal agent",
			outputPath: "other.md",
			expected:   []string{"Universal Agent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			filtered, err := config.FilterAgents(agents, tt.outputPath, nil)
			assert.NoError(t, err)

			var agentNames []string
			for _, agent := range filtered {
				agentNames = append(agentNames, agent.Name)
			}

			assert.ElementsMatch(t, tt.expected, agentNames)
		})
	}
}

func TestFilterEmptySlices(t *testing.T) {
	t.Parallel()

	rules, err := config.FilterRules([]config.Rule{}, "CLAUDE.md", nil)
	assert.NoError(t, err)
	assert.Empty(t, rules)

	sections, err := config.FilterSections([]config.Section{}, "CLAUDE.md", nil)
	assert.NoError(t, err)
	assert.Empty(t, sections)

	agents, err := config.FilterAgents([]config.Agent{}, "CLAUDE.md", nil)
	assert.NoError(t, err)
	assert.Empty(t, agents)

	rules, err = config.FilterRules(nil, "CLAUDE.md", nil)
	assert.NoError(t, err)
	assert.Nil(t, rules)

	sections, err = config.FilterSections(nil, "CLAUDE.md", nil)
	assert.NoError(t, err)
	assert.Nil(t, sections)

	agents, err = config.FilterAgents(nil, "CLAUDE.md", nil)
	assert.NoError(t, err)
	assert.Nil(t, agents)
}

func TestMatchesTargetEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		outputPath string
		targets    []string
		expected   bool
	}{
		{
			name:       "empty output path matches empty targets",
			outputPath: "",
			targets:    []string{},
			expected:   true,
		},
		{
			name:       "empty output path with specific targets fails",
			outputPath: "",
			targets:    []string{"*.md"},
			expected:   false,
		},
		{
			name:       "path with spaces exact match",
			outputPath: "my docs/readme.md",
			targets:    []string{"my docs/readme.md"},
			expected:   true,
		},
		{
			name:       "path with spaces glob match",
			outputPath: "my docs/readme.md",
			targets:    []string{"my docs/*.md"},
			expected:   true,
		},
		{
			name:       "case sensitive matching",
			outputPath: "CLAUDE.MD",
			targets:    []string{"claude.md"},
			expected:   false,
		},
		{
			name:       "case sensitive glob",
			outputPath: "README.MD",
			targets:    []string{"*.md"},
			expected:   false,
		},
		{
			name:       "case sensitive glob match MD",
			outputPath: "README.MD",
			targets:    []string{"*.MD"},
			expected:   true,
		},
		{
			name:       "path traversal patterns",
			outputPath: "../docs/readme.md",
			targets:    []string{"../docs/*"},
			expected:   true,
		},
		{
			name:       "normalized path vs unnormalized target",
			outputPath: "./docs/readme.md",
			targets:    []string{"docs/readme.md"},
			expected:   true,
		},
		{
			name:       "root directory patterns",
			outputPath: "/docs/readme.md",
			targets:    []string{"/docs/*"},
			expected:   true,
		},
		{
			name:       "question mark wildcard",
			outputPath: "file1.md",
			targets:    []string{"file?.md"},
			expected:   true,
		},
		{
			name:       "question mark wildcard no match",
			outputPath: "file10.md",
			targets:    []string{"file?.md"},
			expected:   false,
		},
		{
			name:       "character class patterns",
			outputPath: "file1.md",
			targets:    []string{"file[0-9].md"},
			expected:   true,
		},
		{
			name:       "character class no match",
			outputPath: "filea.md",
			targets:    []string{"file[0-9].md"},
			expected:   false,
		},
		{
			name:       "wildcard pattern match",
			outputPath: "file1.md",
			targets:    []string{"file*.md"},
			expected:   true,
		},
		{
			name:       "very long path",
			outputPath: "a/very/deeply/nested/directory/structure/with/many/levels/file.md",
			targets:    []string{"a/very/deeply/nested/directory/structure/with/many/levels/*.md"},
			expected:   true,
		},
		{
			name:       "unicode in paths",
			outputPath: "docs/测试.md",
			targets:    []string{"docs/*.md"},
			expected:   true,
		},
		{
			name:       "exact unicode match",
			outputPath: "docs/测试.md",
			targets:    []string{"docs/测试.md"},
			expected:   true,
		},
		{
			name:       "multiple directory levels with glob",
			outputPath: "src/main/java/com/example/App.java",
			targets:    []string{"src/main/java/*/*.java"},
			expected:   false,
		},
		{
			name:       "file with no extension exact match",
			outputPath: "Dockerfile",
			targets:    []string{"Dockerfile"},
			expected:   true,
		},
		{
			name:       "file with no extension glob",
			outputPath: "Dockerfile",
			targets:    []string{"Docker*"},
			expected:   true,
		},
		{
			name:       "hidden files exact match",
			outputPath: ".gitignore",
			targets:    []string{".gitignore"},
			expected:   true,
		},
		{
			name:       "hidden files glob match",
			outputPath: ".gitignore",
			targets:    []string{".*"},
			expected:   true,
		},
		{
			name:       "multiple mixed targets",
			outputPath: "docs/api.md",
			targets:    []string{"CLAUDE.md", "*.txt", "docs/*", "src/*.go"},
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := config.MatchesTarget(tt.outputPath, tt.targets)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterRulesWithNamedTargets(t *testing.T) {
	t.Parallel()

	namedTargets := map[string][]string{
		"claude-files": {"CLAUDE.md", ".claude/*.md"},
		"docs-files":   {"docs/*.md", "README.md"},
	}

	rules := []config.Rule{
		{
			Name:    "Global Rule",
			Content: "Applies to all outputs",
			Targets: []string{},
		},
		{
			Name:    "Claude Named Target",
			Content: "Uses named target",
			Targets: []string{"@claude-files"},
		},
		{
			Name:    "Mixed Targets",
			Content: "Mixed named and inline",
			Targets: []string{"@docs-files", "*.go"},
		},
		{
			Name:    "Inline Only",
			Content: "Regular inline target",
			Targets: []string{"src/*.java"},
		},
	}

	tests := []struct {
		name        string
		outputPath  string
		expected    []string
		expectError bool
	}{
		{
			name:       "CLAUDE.md matches global and claude named target",
			outputPath: "CLAUDE.md",
			expected:   []string{"Global Rule", "Claude Named Target"},
		},
		{
			name:       "docs/api.md matches global and mixed targets",
			outputPath: "docs/api.md",
			expected:   []string{"Global Rule", "Mixed Targets"},
		},
		{
			name:       "README.md matches global and mixed targets (from named target)",
			outputPath: "README.md",
			expected:   []string{"Global Rule", "Mixed Targets"},
		},
		{
			name:       "src/Main.java matches global and inline only",
			outputPath: "src/Main.java",
			expected:   []string{"Global Rule", "Inline Only"},
		},
		{
			name:       ".claude/agent.md matches global and claude named target",
			outputPath: ".claude/agent.md",
			expected:   []string{"Global Rule", "Claude Named Target"},
		},
		{
			name:       "unmatched file gets only global rule",
			outputPath: "other/file.txt",
			expected:   []string{"Global Rule"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filtered, err := config.FilterRules(rules, tt.outputPath, namedTargets)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				var ruleNames []string
				for _, rule := range filtered {
					ruleNames = append(ruleNames, rule.Name)
				}

				assert.ElementsMatch(t, tt.expected, ruleNames)
			}
		})
	}
}

// TODO: Implement named target validation in FilterRules

func TestFilterSectionsWithNamedTargets(t *testing.T) {
	t.Parallel()

	namedTargets := map[string][]string{
		"doc-files": {"docs/*.md", "README.md"},
	}

	sections := []config.Section{
		{
			Name:    "Global Section",
			Content: "Appears everywhere",
			Targets: []string{},
		},
		{
			Name:    "Doc Section",
			Content: "For documentation",
			Targets: []string{"@doc-files"},
		},
	}

	tests := []struct {
		name       string
		outputPath string
		expected   []string
	}{
		{
			name:       "README.md gets both sections",
			outputPath: "README.md",
			expected:   []string{"Global Section", "Doc Section"},
		},
		{
			name:       "docs/api.md gets both sections",
			outputPath: "docs/api.md",
			expected:   []string{"Global Section", "Doc Section"},
		},
		{
			name:       "CLAUDE.md gets only global section",
			outputPath: "CLAUDE.md",
			expected:   []string{"Global Section"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filtered, err := config.FilterSections(sections, tt.outputPath, namedTargets)
			assert.NoError(t, err)

			var sectionTitles []string
			for _, section := range filtered {
				sectionTitles = append(sectionTitles, section.Name)
			}

			assert.ElementsMatch(t, tt.expected, sectionTitles)
		})
	}
}

func TestFilterAgentsWithNamedTargets(t *testing.T) {
	t.Parallel()

	namedTargets := map[string][]string{
		"agent-files":  {"ai/agents/*.md"},
		"claude-files": {"CLAUDE.md"},
	}

	agents := []config.Agent{
		{
			Name:        "Universal Agent",
			Description: "Works everywhere",
			Targets:     []string{},
		},
		{
			Name:        "Agent Specialist",
			Description: "Only for agent files",
			Targets:     []string{"@agent-files"},
		},
		{
			Name:        "Claude Assistant",
			Description: "Mixed targets",
			Targets:     []string{"@claude-files", "*.md"},
		},
	}

	tests := []struct {
		name       string
		outputPath string
		expected   []string
	}{
		{
			name:       "ai/agents/test.md gets universal and agent specialist",
			outputPath: "ai/agents/test.md",
			expected:   []string{"Universal Agent", "Agent Specialist"},
		},
		{
			name:       "CLAUDE.md gets universal and claude assistant",
			outputPath: "CLAUDE.md",
			expected:   []string{"Universal Agent", "Claude Assistant"},
		},
		{
			name:       "docs/readme.md gets universal only (*.md no longer matches subdirs)",
			outputPath: "docs/readme.md",
			expected:   []string{"Universal Agent"},
		},
		{
			name:       "other.txt gets only universal",
			outputPath: "other.txt",
			expected:   []string{"Universal Agent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filtered, err := config.FilterAgents(agents, tt.outputPath, namedTargets)
			assert.NoError(t, err)

			var agentNames []string
			for _, agent := range filtered {
				agentNames = append(agentNames, agent.Name)
			}

			assert.ElementsMatch(t, tt.expected, agentNames)
		})
	}
}
