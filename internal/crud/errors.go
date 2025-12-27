package crud

import (
	"errors"
	"fmt"
)

// Error types for CRUD operations
var (
	ErrDomainExists      = errors.New("domain already exists")
	ErrDomainNotFound    = errors.New("domain not found")
	ErrInvalidDomainName = errors.New("invalid domain name")
	ErrInvalidFileName   = errors.New("invalid file name")
	ErrFileExists        = errors.New("file already exists")
	ErrFileNotFound      = errors.New("file not found")
	ErrInvalidInclude    = errors.New("invalid include source")
	ErrInvalidPriority   = errors.New("invalid priority level")
	ErrConfigNotFound    = errors.New("config.yaml not found")
)

// ValidationError wraps validation errors with additional context
type ValidationError struct {
	Field   string
	Value   string
	Message string
	Hint    string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s = %q: %s", e.Field, e.Value, e.Message)
}

func (e *ValidationError) String() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s\nHint: %s", e.Error(), e.Hint)
	}
	return e.Error()
}

// FileExistsError indicates a file already exists
type FileExistsError struct {
	Path string
	Type string
}

func (e *FileExistsError) Error() string {
	return fmt.Sprintf("%s already exists at %s", e.Type, e.Path)
}

// DomainExistsError indicates a domain already exists
type DomainExistsError struct {
	Name string
	Path string
}

func (e *DomainExistsError) Error() string {
	return fmt.Sprintf("domain %q already exists at %s", e.Name, e.Path)
}

// DomainNotFoundError indicates a domain does not exist
type DomainNotFoundError struct {
	Name string
	Path string
}

func (e *DomainNotFoundError) Error() string {
	return fmt.Sprintf("domain %q not found (expected at %s)", e.Name, e.Path)
}

// InvalidNameError indicates an invalid domain or file name
type InvalidNameError struct {
	Field   string
	Value   string
	Reason  string
	Example string
}

func (e *InvalidNameError) Error() string {
	return fmt.Sprintf("invalid %s %q: %s", e.Field, e.Value, e.Reason)
}

func (e *InvalidNameError) String() string {
	hint := fmt.Sprintf("Use format like %q", e.Example)
	if e.Reason != "" {
		hint = fmt.Sprintf("%s (reason: %s)", hint, e.Reason)
	}
	return fmt.Sprintf("%s\nHint: %s", e.Error(), hint)
}
