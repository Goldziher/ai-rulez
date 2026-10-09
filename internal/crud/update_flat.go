package crud

import (
	"context"
	"errors"
	"strings"
)

// ErrEmptyContent is returned when an update would replace an item with nothing.
var ErrEmptyContent = errors.New("content is empty")

// UpdateFlatItem atomically replaces the content of an existing agent or command
// with content as given. Unlike UpdateFile it adds no priority or targets
// frontmatter: those two kinds carry none of their own.
func (op *OperatorImpl) UpdateFlatItem(ctx context.Context, domain, ftype, name, content string) (*FileResult, error) {
	if err := ValidateFileName(name); err != nil {
		return nil, err
	}
	if domain != "" {
		if err := ValidateDomainName(domain); err != nil {
			return nil, err
		}
		if !op.filesMgr.DomainExists(domain) {
			return nil, &DomainNotFoundError{Name: domain, Path: op.filesMgr.GetDomainPath(domain)}
		}
	}
	if !op.filesMgr.FileOrSkillExists(domain, ftype, name) {
		return nil, ErrFileNotFound
	}
	if strings.TrimSpace(content) == "" {
		return nil, ErrEmptyContent
	}
	filePath := op.filesMgr.GetFilePath(domain, ftype, name)
	previous := ""
	if prev, err := op.filesMgr.ReadFile(filePath); err == nil {
		previous = prev
	}
	if err := op.overwriteConcept(ctx, filePath, ftype, domain, name, EnsureTrailingNewline(content), previous); err != nil {
		return nil, err
	}
	return &FileResult{Name: name, FullPath: filePath, Type: ftype, Domain: domain}, nil
}
