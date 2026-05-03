package includes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewGitSourceValidation tests URL validation in NewGitSource
func TestNewGitSourceValidation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid GitHub URL",
			url:     "https://github.com/owner/repo",
			wantErr: false,
		},
		{
			name:    "valid GitLab URL",
			url:     "https://gitlab.com/owner/repo",
			wantErr: false,
		},
		{
			name:    "valid GitHub URL with .git",
			url:     "https://github.com/owner/repo.git",
			wantErr: false,
		},
		{
			name:    "valid SSH URL with git@",
			url:     "git@github.com:owner/repo.git",
			wantErr: false,
		},
		{
			name:    "valid SSH URL with ssh://",
			url:     "ssh://git@github.com/owner/repo.git",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid protocol",
			url:     "git://github.com/owner/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange & Act
			_, err := NewGitSource("test", tt.url, "", "", t.TempDir(), nil, "")

			// Assert
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGitSource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewGitSourceCreation(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	name := "test-source"
	url := "https://github.com/owner/repo"
	path := "/some/path"
	ref := "main"
	include := []string{"rules", "context"}

	// Act
	source, err := NewGitSource(name, url, path, ref, baseDir, include, "")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.name != name {
		t.Errorf("expected name %q, got %q", name, source.name)
	}
	if !strings.Contains(source.repoURL, "github.com") {
		t.Errorf("expected normalized URL to contain github.com, got %q", source.repoURL)
	}
	if source.path != path {
		t.Errorf("expected path %q, got %q", path, source.path)
	}
	if source.ref != ref {
		t.Errorf("expected ref %q, got %q", ref, source.ref)
	}
	if len(source.include) != 2 {
		t.Errorf("expected 2 include items, got %d", len(source.include))
	}
	expectedCacheDir, _ := getIncludeCacheDir(name)
	if source.cacheDir != expectedCacheDir {
		t.Errorf("expected cache dir %q, got %q", expectedCacheDir, source.cacheDir)
	}
}

func TestGitSourceGetType(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	source, err := NewGitSource("test", "https://github.com/owner/repo", "", "", baseDir, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Act
	sourceType := source.GetType()

	// Assert
	if sourceType != SourceTypeGit {
		t.Errorf("expected SourceTypeGit, got %v", sourceType)
	}
}

func TestGitSourceGetName(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	name := "my-source"
	source, err := NewGitSource(name, "https://github.com/owner/repo", "", "", baseDir, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Act
	result := source.GetName()

	// Assert
	if result != name {
		t.Errorf("expected %q, got %q", name, result)
	}
}

// TestFindAIRulezDir tests finding the .ai-rulez directory in cache
func TestFindAIRulezDir(t *testing.T) {
	tests := []struct {
		name         string
		createDir    bool
		createPath   string
		searchPath   string
		wantFound    bool
		wantContains string
	}{
		{
			name:         "found at root of cache",
			createDir:    true,
			createPath:   "",
			searchPath:   "",
			wantFound:    true,
			wantContains: ".ai-rulez",
		},
		{
			name:         "found under specified path",
			createDir:    true,
			createPath:   "some/path",
			searchPath:   "/some/path",
			wantFound:    true,
			wantContains: ".ai-rulez",
		},
		{
			name:         "not found",
			createDir:    false,
			createPath:   "",
			searchPath:   "",
			wantFound:    false,
			wantContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()

			// Use a unique test name to avoid cache collision between tests
			testName := "test-" + strings.ReplaceAll(tt.name, " ", "-")
			source, err := NewGitSource(testName, "https://github.com/owner/repo", tt.searchPath, "", baseDir, nil, "")
			if err != nil {
				t.Fatalf("unexpected error creating source: %v", err)
			}
			cacheDir := source.cacheDir

			// Clean up any existing cache directory from previous test runs
			os.RemoveAll(cacheDir)
			defer os.RemoveAll(cacheDir)

			os.MkdirAll(cacheDir, 0o755)

			if tt.createDir {
				if tt.createPath != "" {
					os.MkdirAll(filepath.Join(cacheDir, tt.createPath, ".ai-rulez"), 0o755)
				} else {
					os.MkdirAll(filepath.Join(cacheDir, ".ai-rulez"), 0o755)
				}
			}

			// Act
			found := source.findAIRulezDir()

			// Assert
			if (found != "") != tt.wantFound {
				t.Errorf("findAIRulezDir() found = %v, want %v", found != "", tt.wantFound)
			}
			if tt.wantFound && !strings.Contains(found, tt.wantContains) {
				t.Errorf("findAIRulezDir() = %q, want to contain %q", found, tt.wantContains)
			}
		})
	}
}

// TestFindAIRulezDirFlatStructureAtSubPath tests finding flat ai-rulez structure at a sub-path
func TestFindAIRulezDirFlatStructureAtSubPath(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	source, err := NewGitSource("test-flat-subpath", "https://github.com/owner/repo", "/domains/polyglot", "", baseDir, nil, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}
	cacheDir := source.cacheDir
	os.RemoveAll(cacheDir)
	defer os.RemoveAll(cacheDir)

	// Create flat structure at sub-path (rules/ directly, no .ai-rulez wrapper)
	os.MkdirAll(filepath.Join(cacheDir, "domains", "polyglot", "rules"), 0o755)
	os.MkdirAll(filepath.Join(cacheDir, "domains", "polyglot", "context"), 0o755)

	// Act
	found := source.findAIRulezDir()

	// Assert
	if found == "" {
		t.Error("findAIRulezDir() should find flat structure at sub-path, got empty")
	}
	if !strings.Contains(found, filepath.Join("domains", "polyglot")) {
		t.Errorf("findAIRulezDir() = %q, want to contain domains/polyglot", found)
	}
}

// TestFindAIRulezDirFlatStructureAtRoot tests flat structure at cache root
func TestFindAIRulezDirFlatStructureAtRoot(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	source, err := NewGitSource("test-flat-root", "https://github.com/owner/repo", "", "", baseDir, nil, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}
	cacheDir := source.cacheDir
	os.RemoveAll(cacheDir)
	defer os.RemoveAll(cacheDir)

	// Create flat structure at root (rules/ directly, no .ai-rulez wrapper)
	os.MkdirAll(filepath.Join(cacheDir, "rules"), 0o755)
	os.MkdirAll(filepath.Join(cacheDir, "context"), 0o755)

	// Act
	found := source.findAIRulezDir()

	// Assert
	if found == "" {
		t.Error("findAIRulezDir() should find flat structure at root, got empty")
	}
	if found != cacheDir {
		t.Errorf("findAIRulezDir() = %q, want %q", found, cacheDir)
	}
}

// TestGitSourceFilterContent tests content filtering
func TestGitSourceFilterContentIncludesRules(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	tree := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.Domain),
	}
	source, err := NewGitSource("test", "https://github.com/owner/repo", "", "", baseDir, []string{"rules"}, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}

	// Act
	filtered := source.filterContent(tree)

	// Assert
	if len(filtered.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(filtered.Rules))
	}
	if len(filtered.Context) != 0 {
		t.Errorf("expected 0 context, got %d", len(filtered.Context))
	}
	if len(filtered.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(filtered.Skills))
	}
}

func TestGitSourceFilterContentIncludesMultiple(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	tree := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.Domain),
	}
	source, err := NewGitSource("test", "https://github.com/owner/repo", "", "", baseDir, []string{"rules", "context"}, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}

	// Act
	filtered := source.filterContent(tree)

	// Assert
	if len(filtered.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(filtered.Rules))
	}
	if len(filtered.Context) != 1 {
		t.Errorf("expected 1 context, got %d", len(filtered.Context))
	}
	if len(filtered.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(filtered.Skills))
	}
}

func TestGitSourceFilterContentEmptyInclude(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	tree := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.Domain),
	}
	source, err := NewGitSource("test", "https://github.com/owner/repo", "", "", baseDir, []string{}, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}

	// Act
	filtered := source.filterContent(tree)

	// Assert
	if len(filtered.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(filtered.Rules))
	}
	if len(filtered.Context) != 0 {
		t.Errorf("expected 0 context, got %d", len(filtered.Context))
	}
	if len(filtered.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(filtered.Skills))
	}
}

func TestGitSourceFilterContentPreserveDomains(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	domain := &config.Domain{
		Name:  "backend",
		Rules: []config.ContentFile{{Name: "domain-rule"}},
	}
	tree := &config.ContentTree{
		Rules:   []config.ContentFile{{Name: "rule1"}},
		Context: []config.ContentFile{{Name: "context1"}},
		Domains: map[string]*config.Domain{
			"backend": domain,
		},
	}
	source, err := NewGitSource("test", "https://github.com/owner/repo", "", "", baseDir, []string{"rules"}, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}

	// Act
	filtered := source.filterContent(tree)

	// Assert
	if len(filtered.Domains) != 1 {
		t.Errorf("expected 1 domain, got %d", len(filtered.Domains))
	}
	if filtered.Domains["backend"] != domain {
		t.Error("expected domain to be preserved")
	}
}

func TestGitSourceFilterContentIncludesAgents(t *testing.T) {
	// Arrange: tree has agents; include list contains "agents"
	baseDir := t.TempDir()
	tree := &config.ContentTree{
		Rules:   []config.ContentFile{{Name: "rule1", Content: "rule content"}},
		Context: []config.ContentFile{{Name: "context1", Content: "context content"}},
		Skills:  []config.ContentFile{{Name: "skill1", Content: "skill content"}},
		Agents:  []config.ContentFile{{Name: "golang-maintainer", Content: "agent content"}},
		Domains: make(map[string]*config.Domain),
	}
	source, err := NewGitSource("test", "https://github.com/owner/repo", "", "", baseDir, []string{"rules", "agents"}, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}

	// Act
	filtered := source.filterContent(tree)

	// Assert: agents are included when "agents" is in include list
	if len(filtered.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(filtered.Agents))
	}
	if len(filtered.Agents) > 0 && filtered.Agents[0].Name != "golang-maintainer" {
		t.Errorf("expected agent name golang-maintainer, got %q", filtered.Agents[0].Name)
	}
	if len(filtered.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(filtered.Rules))
	}
	if len(filtered.Context) != 0 {
		t.Errorf("expected 0 context (not in include), got %d", len(filtered.Context))
	}
	if len(filtered.Skills) != 0 {
		t.Errorf("expected 0 skills (not in include), got %d", len(filtered.Skills))
	}
}

func TestGitSourceFilterContentExcludesAgentsWhenNotInInclude(t *testing.T) {
	// Arrange: tree has agents; include list does not contain "agents"
	baseDir := t.TempDir()
	tree := &config.ContentTree{
		Rules:   []config.ContentFile{{Name: "rule1", Content: "rule content"}},
		Context: []config.ContentFile{{Name: "context1", Content: "context content"}},
		Agents:  []config.ContentFile{{Name: "governance-architect", Content: "agent content"}},
		Domains: make(map[string]*config.Domain),
	}
	source, err := NewGitSource("test", "https://github.com/owner/repo", "", "", baseDir, []string{"rules", "context"}, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}

	// Act
	filtered := source.filterContent(tree)

	// Assert: agents are not copied when "agents" is not in include list
	if len(filtered.Agents) != 0 {
		t.Errorf("expected 0 agents when agents not in include, got %d", len(filtered.Agents))
	}
	if len(filtered.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(filtered.Rules))
	}
	if len(filtered.Context) != 1 {
		t.Errorf("expected 1 context, got %d", len(filtered.Context))
	}
}

// TestValidateGitURL tests URL validation
func TestValidateGitURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid HTTPS URL",
			url:     "https://github.com/owner/repo",
			wantErr: false,
		},
		{
			name:    "valid HTTP URL",
			url:     "http://github.com/owner/repo",
			wantErr: false,
		},
		{
			name:    "valid SSH URL with git@ prefix",
			url:     "git@github.com:owner/repo.git",
			wantErr: false,
		},
		{
			name:    "valid SSH URL with ssh:// protocol",
			url:     "ssh://git@github.com/owner/repo.git",
			wantErr: false,
		},
		{
			name:    "valid SSH URL for GitLab",
			url:     "git@gitlab.com:owner/repo.git",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid protocol",
			url:     "git://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "invalid URL format",
			url:     "https://not a url at all",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := validateGitURL(tt.url)

			// Assert
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGitURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNormalizeGitURL tests URL normalization
func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "removes trailing slash",
			input: "https://github.com/owner/repo/",
			want:  "https://github.com/owner/repo",
		},
		{
			name:  "keeps URL as-is without trailing slash",
			input: "https://github.com/owner/repo",
			want:  "https://github.com/owner/repo",
		},
		{
			name:  "removes multiple trailing slashes",
			input: "https://github.com/owner/repo///",
			want:  "https://github.com/owner/repo",
		},
		{
			name:  "converts SSH URL with git@ to HTTPS",
			input: "git@github.com:owner/repo.git",
			want:  "https://github.com/owner/repo.git",
		},
		{
			name:  "converts SSH URL without .git suffix",
			input: "git@github.com:owner/repo",
			want:  "https://github.com/owner/repo",
		},
		{
			name:  "converts ssh:// URL to HTTPS",
			input: "ssh://git@github.com/owner/repo.git",
			want:  "https://github.com/owner/repo.git",
		},
		{
			name:  "converts GitLab SSH URL",
			input: "git@gitlab.com:owner/repo.git",
			want:  "https://gitlab.com/owner/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result := normalizeGitURL(tt.input)

			// Assert
			if result != tt.want {
				t.Errorf("normalizeGitURL() = %q, want %q", result, tt.want)
			}
		})
	}
}

// TestCacheDirCreation tests that cache directory is created properly
func TestCacheDirCreation(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	sourceName := "my-source"

	// Act
	source, err := NewGitSource(sourceName, "https://github.com/owner/repo", "", "", baseDir, nil, "")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCacheDir, _ := getIncludeCacheDir(sourceName)
	if source.cacheDir != expectedCacheDir {
		t.Errorf("expected cache dir %q, got %q", expectedCacheDir, source.cacheDir)
	}
}

// initLocalRepo initializes a bare git repo and returns its path.
func initLocalRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}
	dir := t.TempDir()
	cmds := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	return dir
}

// commitFile writes relPath into repoDir and creates a commit.
func commitFile(t *testing.T, repoDir, relPath, content string) {
	t.Helper()
	path := filepath.Join(repoDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	for _, args := range [][]string{{"add", relPath}, {"commit", "-m", "update"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
}

func TestGitSourceFetch_CacheHit(t *testing.T) {
	repoDir := initLocalRepo(t)
	commitFile(t, repoDir, ".ai-rulez/rules/rule.md", "---\nname: rule1\npriority: high\n---\n# Rule")

	cacheDir := t.TempDir()
	source := &GitSource{
		name:        "test-include",
		repoURL:     normalizeGitURL("file://" + repoDir),
		originalURL: "file://" + repoDir,
		cacheDir:    cacheDir,
		accessToken: "",
	}

	ctx := context.Background()

	// First fetch populates the cache.
	tree, err := source.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, tree)
	assert.NotEmpty(t, tree.Rules, "expected at least one rule after first fetch")

	// Second fetch should hit the SHA cache and return same content.
	tree2, err := source.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, tree2)
	assert.Len(t, tree2.Rules, len(tree.Rules))
}

func TestGitSourceFetch_CacheMiss_AfterCommit(t *testing.T) {
	repoDir := initLocalRepo(t)
	commitFile(t, repoDir, ".ai-rulez/rules/rule.md", "---\nname: rule1\npriority: high\n---\n# Rule Old")

	cacheDir := t.TempDir()
	source := &GitSource{
		name:        "test-include-miss",
		repoURL:     normalizeGitURL("file://" + repoDir),
		originalURL: "file://" + repoDir,
		cacheDir:    cacheDir,
		accessToken: "",
	}

	ctx := context.Background()

	_, err := source.Fetch(ctx)
	require.NoError(t, err)

	// Push a new commit so the remote SHA changes.
	commitFile(t, repoDir, ".ai-rulez/rules/rule.md", "---\nname: rule1\npriority: high\n---\n# Rule New")

	// Invalidate in-process scan cache so the test sees the new on-disk files.
	if dir := source.findAIRulezDir(); dir != "" {
		invalidateScan(dir)
	}

	tree2, err := source.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, tree2)
	require.NotEmpty(t, tree2.Rules)
	assert.Contains(t, tree2.Rules[0].Content, "Rule New")
}

func TestGitSourceFetch_SkipFetch_MissingCache(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	cacheDir := t.TempDir()
	source := &GitSource{
		name:        "test-skip-missing",
		repoURL:     "https://github.com/owner/repo",
		originalURL: "https://github.com/owner/repo",
		cacheDir:    cacheDir,
		accessToken: "",
	}

	t.Cleanup(func() { SkipFetch = false })
	SkipFetch = true

	_, err := source.Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no cached")
}

func TestGitSourceFetch_SkipFetch_WithCache(t *testing.T) {
	repoDir := initLocalRepo(t)
	commitFile(t, repoDir, ".ai-rulez/rules/rule.md", "---\nname: rule1\npriority: high\n---\n# Rule")

	cacheDir := t.TempDir()
	source := &GitSource{
		name:        "test-skip-cached",
		repoURL:     normalizeGitURL("file://" + repoDir),
		originalURL: "file://" + repoDir,
		cacheDir:    cacheDir,
		accessToken: "",
	}

	ctx := context.Background()

	// Populate cache with a live fetch.
	_, err := source.Fetch(ctx)
	require.NoError(t, err)

	t.Cleanup(func() { SkipFetch = false })
	SkipFetch = true

	tree, err := source.Fetch(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, tree.Rules)
}
