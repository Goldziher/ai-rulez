package includes

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// SourceType represents the type of include source
type SourceType string

const (
	SourceTypeGit   SourceType = "git"
	SourceTypeLocal SourceType = "local"
)

// Source represents a content source that can be fetched
type Source interface {
	Fetch(ctx context.Context) (*config.ContentTree, error)
	GetType() SourceType
	GetName() string
}

// DetectSourceType determines if source is a git URL or local path. The rule
// lives in lockfile.IsGitSource, which the content lock shares.
func DetectSourceType(source string) SourceType {
	if lockfile.IsGitSource(source) {
		return SourceTypeGit
	}
	return SourceTypeLocal
}

// IsGitURL checks if a source string is a git repository URL
func IsGitURL(source string) bool {
	return DetectSourceType(source) == SourceTypeGit
}

// IsLocalPath checks if a source string is a local file path
func IsLocalPath(source string) bool {
	return DetectSourceType(source) == SourceTypeLocal
}

// discoverDomainDirs reads the domains/ subdirectory of an .ai-rulez path
// and returns the names of all subdirectories (domain names).
func discoverDomainDirs(aiRulezDir string) []string {
	domainsPath := filepath.Join(aiRulezDir, "domains")
	entries, err := workspace.OSView(aiRulezDir).ReadDir(domainsPath)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// stripGitPlus drops a leading, case-insensitive "git+" scheme prefix, the form
// skill sources accept ("git+https://host/repo"). git itself cannot clone it.
func stripGitPlus(u string) string {
	if len(u) >= 4 && strings.EqualFold(u[:4], "git+") {
		return u[4:]
	}
	return u
}
