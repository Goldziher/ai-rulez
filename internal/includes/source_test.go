package includes

import (
	"testing"
)

func TestDetectSourceType(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected SourceType
	}{
		{
			name:     "https git URL",
			source:   "https://github.com/org/repo",
			expected: SourceTypeGit,
		},
		{
			name:     "http git URL",
			source:   "http://example.com/repo",
			expected: SourceTypeGit,
		},
		{
			name:     "absolute path",
			source:   "/absolute/path",
			expected: SourceTypeLocal,
		},
		{
			name:     "relative path with ..",
			source:   "../relative/path",
			expected: SourceTypeLocal,
		},
		{
			name:     "relative path with .",
			source:   "./relative/path",
			expected: SourceTypeLocal,
		},
		{
			name:     "simple relative path",
			source:   "relative/path",
			expected: SourceTypeLocal,
		},
		{
			name:     "file name",
			source:   "file.txt",
			expected: SourceTypeLocal,
		},
		{
			name:     "empty string",
			source:   "",
			expected: SourceTypeLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectSourceType(tt.source)
			if result != tt.expected {
				t.Errorf("DetectSourceType(%q) = %v, want %v", tt.source, result, tt.expected)
			}
		})
	}
}

func TestDetectSourceType_Forms(t *testing.T) {
	tests := []struct {
		source string
		want   SourceType
	}{
		{"ssh://git@github.com/org/repo.git", SourceTypeGit},
		{"ssh://git@host:2222/org/repo.git", SourceTypeGit},
		{"git://example.com/repo.git", SourceTypeGit},
		{"git+ssh://git@host/repo.git", SourceTypeGit},
		{"git@github.com:org/repo.git", SourceTypeGit},
		{"user@host:path/repo.git", SourceTypeGit},
		{"https://github.com/org/repo", SourceTypeGit},
		{"HTTPS://github.com/org/repo", SourceTypeGit},
		{"file:///tmp/repo", SourceTypeGit},
		{`C:\Users\me\repo`, SourceTypeLocal},
		{"C:/Users/me/repo", SourceTypeLocal},
		{`C:\Users\me@corp\repo`, SourceTypeLocal},
		{"D:rel", SourceTypeLocal},
		{"/home/a@b:c/repo", SourceTypeLocal},
		{"./dir@x:y", SourceTypeLocal},
		{"../shared", SourceTypeLocal},
		{"shared/rules", SourceTypeLocal},
		{"host:path", SourceTypeLocal},
		{"@host:path", SourceTypeLocal},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			if got := DetectSourceType(tt.source); got != tt.want {
				t.Errorf("DetectSourceType(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected bool
	}{
		{
			name:     "https git URL",
			source:   "https://github.com/org/repo",
			expected: true,
		},
		{
			name:     "http git URL",
			source:   "http://example.com/repo",
			expected: true,
		},
		{
			name:     "absolute path",
			source:   "/absolute/path",
			expected: false,
		},
		{
			name:     "relative path",
			source:   "../relative/path",
			expected: false,
		},
		{
			name:     "simple path",
			source:   "./path",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGitURL(tt.source)
			if result != tt.expected {
				t.Errorf("IsGitURL(%q) = %v, want %v", tt.source, result, tt.expected)
			}
		})
	}
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected bool
	}{
		{
			name:     "https git URL",
			source:   "https://github.com/org/repo",
			expected: false,
		},
		{
			name:     "http git URL",
			source:   "http://example.com/repo",
			expected: false,
		},
		{
			name:     "absolute path",
			source:   "/absolute/path",
			expected: true,
		},
		{
			name:     "relative path with ..",
			source:   "../relative/path",
			expected: true,
		},
		{
			name:     "relative path with .",
			source:   "./path",
			expected: true,
		},
		{
			name:     "simple relative path",
			source:   "path/to/file",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLocalPath(tt.source)
			if result != tt.expected {
				t.Errorf("IsLocalPath(%q) = %v, want %v", tt.source, result, tt.expected)
			}
		})
	}
}
