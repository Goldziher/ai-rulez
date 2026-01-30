package includes

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// SourceType represents the type of include source
type SourceType string

const (
	SourceTypeGit   SourceType = "git"
	SourceTypeLocal SourceType = "local"
)

// Source represents a content source that can be fetched
type Source interface {
	Fetch(ctx context.Context) (*config.ContentTreeV3, error)
	GetType() SourceType
	GetName() string
}

// DetectSourceType determines if source is a git URL or local path
func DetectSourceType(source string) SourceType {
	// Git URLs start with http:// or https://
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return SourceTypeGit
	}
	// SSH Git URLs (e.g., git@github.com:user/repo.git or user@host:path/repo.git)
	// Pattern: contains @ followed by : (but not :// which would be http/https)
	if strings.Contains(source, "@") && strings.Contains(source, ":") {
		atIndex := strings.Index(source, "@")
		colonIndex := strings.Index(source, ":")
		// Check that @ comes before : and : is not part of ://
		if atIndex < colonIndex && !strings.HasPrefix(source[colonIndex:], "://") {
			return SourceTypeGit
		}
	}
	// Everything else is treated as a local path
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

// discoverDomains scans the domains/ directory within an .ai-rulez directory
// and returns a list of domain names. This is used when scanning includes
// to ensure all domains from the included source are discovered and scanned.
func discoverDomains(aiRulezDir string) []string {
	domainsDir := filepath.Join(aiRulezDir, "domains")

	// Check if domains directory exists
	info, err := os.Stat(domainsDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	// Read domain subdirectories
	entries, err := os.ReadDir(domainsDir)
	if err != nil {
		return nil
	}

	var domains []string
	for _, entry := range entries {
		if entry.IsDir() {
			domains = append(domains, entry.Name())
		}
	}

	return domains
}
