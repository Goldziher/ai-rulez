package crud

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/samber/oops"
)

// AddDomain creates a new domain with subdirectories (rules, context, skills)
// Returns DomainResult with the created domain information
func (op *OperatorImpl) AddDomain(ctx context.Context, req *AddDomainRequest) (*DomainResult, error) {
	if req == nil {
		return nil, oops.
			Hint("AddDomainRequest cannot be nil").
			Errorf("invalid request")
	}

	// Validate domain name
	if err := ValidateDomainName(req.Name); err != nil {
		return nil, err
	}

	// Validate description if provided
	if req.Description != "" {
		if err := ValidateDescription(req.Description, "description"); err != nil {
			return nil, err
		}
	}

	// Check if domain already exists
	if op.filesMgr.DomainExists(req.Name) {
		return nil, &DomainExistsError{
			Name: req.Name,
			Path: op.filesMgr.GetDomainPath(req.Name),
		}
	}

	// Create domain directory structure
	if err := op.filesMgr.CreateDomainStructure(req.Name); err != nil {
		return nil, err
	}

	// Write description file if provided
	if req.Description != "" {
		descPath := filepath.Join(op.filesMgr.GetDomainPath(req.Name), ".description")
		//nolint:gosec,errcheck // description file is optional, best effort
		_ = os.WriteFile(descPath, []byte(req.Description), 0o644)
	}

	return &DomainResult{
		Name:        req.Name,
		Path:        op.filesMgr.GetDomainPath(req.Name),
		Description: req.Description,
		Created:     true,
	}, nil
}

// RemoveDomain deletes a domain directory and all its contents
func (op *OperatorImpl) RemoveDomain(ctx context.Context, name string) error {
	// Validate domain name
	if err := ValidateDomainName(name); err != nil {
		return err
	}

	// Check if domain exists
	if !op.filesMgr.DomainExists(name) {
		return &DomainNotFoundError{
			Name: name,
			Path: op.filesMgr.GetDomainPath(name),
		}
	}

	domainPath := op.filesMgr.GetDomainPath(name)

	// Delete domain directory
	return op.filesMgr.DeleteDirectory(domainPath)
}

// ListDomains scans the domains directory and returns information about all domains
func (op *OperatorImpl) ListDomains(ctx context.Context) ([]DomainInfo, error) {
	domainsDir := filepath.Join(op.aiRulezDir, "domains")

	// If domains directory doesn't exist, return empty list
	if !op.filesMgr.PathExists(domainsDir) {
		return []DomainInfo{}, nil
	}

	// List subdirectories in domains/
	subdirs, err := op.filesMgr.ListSubdirectories(domainsDir)
	if err != nil {
		return nil, err
	}

	// Sort subdirectories alphabetically
	sort.Strings(subdirs)

	// Build DomainInfo for each subdirectory
	var domains []DomainInfo
	for _, dirName := range subdirs {
		domainPath := filepath.Join(domainsDir, dirName)

		// Verify it's a directory with the expected structure
		if !op.filesMgr.IsDirectory(domainPath) {
			continue
		}

		// Try to read description if it exists
		description := ""
		descPath := filepath.Join(domainPath, ".description")
		if data, err := os.ReadFile(descPath); err == nil {
			description = string(data)
		}

		domains = append(domains, DomainInfo{
			Name:        dirName,
			Path:        domainPath,
			Description: description,
		})
	}

	return domains, nil
}
