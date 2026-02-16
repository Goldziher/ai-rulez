package includes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
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

// TestBuildRawURLGitHub tests GitHub raw URL construction
func TestBuildRawURLGitHub(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		path    string
		ref     string
		wantURL string
		wantErr bool
	}{
		{
			name:    "basic GitHub URL with default ref",
			repo:    "https://github.com/owner/repo",
			path:    "/",
			ref:     "",
			wantURL: "https://raw.githubusercontent.com/owner/repo/main/",
			wantErr: false,
		},
		{
			name:    "GitHub URL with custom ref",
			repo:    "https://github.com/owner/repo",
			path:    "/",
			ref:     "develop",
			wantURL: "https://raw.githubusercontent.com/owner/repo/develop/",
			wantErr: false,
		},
		{
			name:    "GitHub URL with custom path",
			repo:    "https://github.com/owner/repo",
			path:    "/some/path",
			ref:     "main",
			wantURL: "https://raw.githubusercontent.com/owner/repo/main/some/path",
			wantErr: false,
		},
		{
			name:    "GitHub URL with .git suffix",
			repo:    "https://github.com/owner/repo.git",
			path:    "/",
			ref:     "main",
			wantURL: "https://raw.githubusercontent.com/owner/repo/main/",
			wantErr: false,
		},
		{
			name:    "GitHub URL with trailing slash",
			repo:    "https://github.com/owner/repo/",
			path:    "/",
			ref:     "main",
			wantURL: "https://raw.githubusercontent.com/owner/repo/main/",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()
			source, err := NewGitSource("test", tt.repo, tt.path, tt.ref, baseDir, nil, "")
			if err != nil {
				t.Fatalf("unexpected error creating source: %v", err)
			}

			// Act
			rawURL, err := source.buildRawURL()

			// Assert
			if (err != nil) != tt.wantErr {
				t.Errorf("buildRawURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if rawURL != tt.wantURL {
				t.Errorf("buildRawURL() = %q, want %q", rawURL, tt.wantURL)
			}
		})
	}
}

// TestBuildRawURLGitLab tests GitLab raw URL construction
func TestBuildRawURLGitLab(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		path    string
		ref     string
		wantURL string
		wantErr bool
	}{
		{
			name:    "basic GitLab URL with default ref",
			repo:    "https://gitlab.com/owner/repo",
			path:    "/",
			ref:     "",
			wantURL: "https://gitlab.com/owner/repo/-/raw/main/",
			wantErr: false,
		},
		{
			name:    "GitLab URL with custom ref",
			repo:    "https://gitlab.com/owner/repo",
			path:    "/",
			ref:     "develop",
			wantURL: "https://gitlab.com/owner/repo/-/raw/develop/",
			wantErr: false,
		},
		{
			name:    "GitLab URL with custom path",
			repo:    "https://gitlab.com/owner/repo",
			path:    "/some/path",
			ref:     "main",
			wantURL: "https://gitlab.com/owner/repo/-/raw/main/some/path",
			wantErr: false,
		},
		{
			name:    "GitLab URL with .git suffix",
			repo:    "https://gitlab.com/owner/repo.git",
			path:    "/",
			ref:     "main",
			wantURL: "https://gitlab.com/owner/repo/-/raw/main/",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()
			source, err := NewGitSource("test", tt.repo, tt.path, tt.ref, baseDir, nil, "")
			if err != nil {
				t.Fatalf("unexpected error creating source: %v", err)
			}

			// Act
			rawURL, err := source.buildRawURL()

			// Assert
			if (err != nil) != tt.wantErr {
				t.Errorf("buildRawURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if rawURL != tt.wantURL {
				t.Errorf("buildRawURL() = %q, want %q", rawURL, tt.wantURL)
			}
		})
	}
}

// TestBuildArchiveURLGitHub tests GitHub archive URL construction
func TestBuildArchiveURLGitHub(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		ref     string
		wantURL string
		wantErr bool
	}{
		{
			name:    "basic GitHub archive URL",
			repo:    "https://github.com/owner/repo",
			ref:     "main",
			wantURL: "https://github.com/owner/repo/archive/refs/heads/main.tar.gz",
			wantErr: false,
		},
		{
			name:    "GitHub archive URL with custom ref",
			repo:    "https://github.com/owner/repo",
			ref:     "v1.0.0",
			wantURL: "https://github.com/owner/repo/archive/refs/heads/v1.0.0.tar.gz",
			wantErr: false,
		},
		{
			name:    "GitHub archive URL with default ref",
			repo:    "https://github.com/owner/repo",
			ref:     "",
			wantURL: "https://github.com/owner/repo/archive/refs/heads/main.tar.gz",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()
			source, err := NewGitSource("test", tt.repo, "", tt.ref, baseDir, nil, "")
			if err != nil {
				t.Fatalf("unexpected error creating source: %v", err)
			}

			// Act
			archiveURL, err := source.buildArchiveURL()

			// Assert
			if (err != nil) != tt.wantErr {
				t.Errorf("buildArchiveURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if archiveURL != tt.wantURL {
				t.Errorf("buildArchiveURL() = %q, want %q", archiveURL, tt.wantURL)
			}
		})
	}
}

// TestBuildArchiveURLGitLab tests GitLab archive URL construction
func TestBuildArchiveURLGitLab(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		ref     string
		wantURL string
		wantErr bool
	}{
		{
			name:    "basic GitLab archive URL",
			repo:    "https://gitlab.com/owner/repo",
			ref:     "main",
			wantURL: "https://gitlab.com/owner/repo/-/archive/main/archive.tar.gz",
			wantErr: false,
		},
		{
			name:    "GitLab archive URL with custom ref",
			repo:    "https://gitlab.com/owner/repo",
			ref:     "v1.0.0",
			wantURL: "https://gitlab.com/owner/repo/-/archive/v1.0.0/archive.tar.gz",
			wantErr: false,
		},
		{
			name:    "GitLab archive URL with default ref",
			repo:    "https://gitlab.com/owner/repo",
			ref:     "",
			wantURL: "https://gitlab.com/owner/repo/-/archive/main/archive.tar.gz",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			baseDir := t.TempDir()
			source, err := NewGitSource("test", tt.repo, "", tt.ref, baseDir, nil, "")
			if err != nil {
				t.Fatalf("unexpected error creating source: %v", err)
			}

			// Act
			archiveURL, err := source.buildArchiveURL()

			// Assert
			if (err != nil) != tt.wantErr {
				t.Errorf("buildArchiveURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if archiveURL != tt.wantURL {
				t.Errorf("buildArchiveURL() = %q, want %q", archiveURL, tt.wantURL)
			}
		})
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

// TestGitSourceFilterContent tests content filtering
func TestGitSourceFilterContentIncludesRules(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	tree := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.DomainV3),
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
	tree := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.DomainV3),
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
	tree := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "rule content"},
		},
		Context: []config.ContentFile{
			{Name: "context1", Content: "context content"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "skill content"},
		},
		Domains: make(map[string]*config.DomainV3),
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
	domain := &config.DomainV3{
		Name:  "backend",
		Rules: []config.ContentFile{{Name: "domain-rule"}},
	}
	tree := &config.ContentTreeV3{
		Rules:   []config.ContentFile{{Name: "rule1"}},
		Context: []config.ContentFile{{Name: "context1"}},
		Domains: map[string]*config.DomainV3{
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
	tree := &config.ContentTreeV3{
		Rules:   []config.ContentFile{{Name: "rule1", Content: "rule content"}},
		Context: []config.ContentFile{{Name: "context1", Content: "context content"}},
		Skills:  []config.ContentFile{{Name: "skill1", Content: "skill content"}},
		Agents:  []config.ContentFile{{Name: "golang-maintainer", Content: "agent content"}},
		Domains: make(map[string]*config.DomainV3),
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
	tree := &config.ContentTreeV3{
		Rules:   []config.ContentFile{{Name: "rule1", Content: "rule content"}},
		Context: []config.ContentFile{{Name: "context1", Content: "context content"}},
		Agents:  []config.ContentFile{{Name: "governance-architect", Content: "agent content"}},
		Domains: make(map[string]*config.DomainV3),
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

// TestFetchErrorHandlingInvalidURL tests error handling for invalid URLs
func TestFetchErrorHandlingInvalidURL(t *testing.T) {
	// Arrange
	baseDir := t.TempDir()
	source, err := NewGitSource("test", "https://invalid-host-xyz.example.com/owner/repo", "", "", baseDir, nil, "")
	if err != nil {
		t.Fatalf("unexpected error creating source: %v", err)
	}
	ctx := context.Background()

	// Act
	_, err = source.Fetch(ctx)

	// Assert
	// This should fail because we're trying to fetch from a non-existent server
	// The actual error depends on the HTTP client, but it should not be nil
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}
