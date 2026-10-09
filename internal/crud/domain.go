package crud

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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
		//nolint:errcheck // description file is optional, best effort
		_ = op.filesMgr.WriteFileOverwrite(descPath, req.Description)
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
	if err := op.CheckDomainRemovable(ctx, name); err != nil {
		return err
	}
	// Delete domain directory
	if err := op.filesMgr.DeleteDirectory(op.filesMgr.GetDomainPath(name)); err != nil {
		return err
	}
	return op.refreshIndexes(ctx)
}

// CheckDomainRemovable reports why RemoveDomain would fail (a bad name, no such
// domain, a profile that still lists it), without removing anything.
func (op *OperatorImpl) CheckDomainRemovable(ctx context.Context, name string) error {
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

	if users := op.profilesUsingDomain(ctx, name); len(users) > 0 {
		return oops.
			With("domain", name).
			With("profiles", users).
			Hint("Remove the profile or drop the domain from it first: 'ai-rulez profile remove "+users[0]+"'.").
			Errorf("domain %q is still used by profile(s): %s", name, strings.Join(users, ", "))
	}

	return nil
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

// profilesUsingDomain names the profiles of the shared config that list the
// domain, sorted. A configuration that does not load names none: the removal is
// not blocked by an unrelated problem, and `validate` reports it.
func (op *OperatorImpl) profilesUsingDomain(ctx context.Context, domain string) []string {
	cfg, err := op.load(config.WithUnresolvedIncludesTolerated(ctx), config.WithoutLocal())
	if err != nil {
		return nil
	}
	var users []string
	for profile, domains := range cfg.Profiles {
		if slices.Contains(domains, domain) {
			users = append(users, profile)
		}
	}
	sort.Strings(users)
	return users
}

// DomainPath is the directory of the named domain, whether or not it exists.
func (op *OperatorImpl) DomainPath(name string) string { return op.filesMgr.GetDomainPath(name) }
