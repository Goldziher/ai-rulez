package includes

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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

// gitURLSchemes are the URL schemes git can fetch from. Anything else with a
// scheme-like prefix is a local path.
var gitURLSchemes = []string{"http://", "https://", "file://", "ssh://", "git://", "git+ssh://", "git+https://", "git+http://"}

// DetectSourceType determines if source is a git URL or local path.
//
// Git sources are URLs with a git-capable scheme (http, https, file, ssh, git
// and the git+ forms) and scp-like addresses, "[user@]host:path" with an "@"
// before the first ":" and no "/" before it ("git@github.com:org/repo.git").
// Everything else, including Windows drive paths (C:\x, C:/x), is local.
func DetectSourceType(source string) SourceType {
	lower := strings.ToLower(source)
	for _, scheme := range gitURLSchemes {
		if strings.HasPrefix(lower, scheme) {
			return SourceTypeGit
		}
	}
	colon := strings.Index(source, ":")
	at := strings.Index(source, "@")
	if colon > 0 && at > 0 && at < colon && !strings.ContainsAny(source[:colon], `/\`) {
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
	entries, err := os.ReadDir(domainsPath)
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
