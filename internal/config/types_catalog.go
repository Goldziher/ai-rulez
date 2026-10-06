package config

import "github.com/samber/oops"

// DefaultCatalogPageSize is how many items of the catalog overview one page of
// the static site shows when [catalog] max_items_per_page is not set.
const DefaultCatalogPageSize = 200

// CatalogConfig is the [catalog] table: defaults for `ai-rulez catalog` (see
// docs/catalog.md). Every key is optional; a command-line flag overrides it.
type CatalogConfig struct {
	// Title is the title of the static site.
	Title string `yaml:"title,omitempty" json:"title,omitempty" toml:"title,omitempty"`
	// IncludeExcerpt sets whether items carry a body excerpt. Unset: on, except
	// for an indexable site.
	IncludeExcerpt *bool `yaml:"include_excerpt,omitempty" json:"include_excerpt,omitempty" toml:"include_excerpt,omitempty"` //nolint:tagliatelle
	// ExcludeOwners leaves owner names out of the catalog and the site.
	ExcludeOwners bool `yaml:"exclude_owners,omitempty" json:"exclude_owners,omitempty" toml:"exclude_owners,omitempty"` //nolint:tagliatelle
	// Indexable lets search engines index the site (no robots.txt, no noindex).
	Indexable bool `yaml:"indexable,omitempty" json:"indexable,omitempty" toml:"indexable,omitempty"`
	// MaxItemsPerPage is how many overview rows one page of the site shows
	// before the rest moves to the next page. 0 means DefaultCatalogPageSize.
	MaxItemsPerPage int `yaml:"max_items_per_page,omitempty" json:"max_items_per_page,omitempty" toml:"max_items_per_page,omitempty"` //nolint:tagliatelle
}

func (c *Config) validateCatalog() error {
	cat := c.Catalog
	if cat == nil {
		return nil
	}
	if cat.MaxItemsPerPage < 0 {
		return oops.With("field", "catalog.max_items_per_page").Errorf("max_items_per_page must not be negative")
	}
	return nil
}
