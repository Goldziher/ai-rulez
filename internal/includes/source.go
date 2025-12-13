package includes

import (
	"context"
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
