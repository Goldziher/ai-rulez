package mcp

import (
	"encoding/json"

	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/oops"
)

// projectlessItem is one entry of the catalog of a directory with no
// configuration: a skill that discovery found.
type projectlessItem struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
	URI         string `json:"uri"`
}

// projectlessCatalog is the catalog resource without a configuration. Its items
// are the served skills; the instruction files, rules and agents come from the
// scopes section and the discovery report is added to it like to any catalog
// (decorateCatalog).
type projectlessCatalog struct {
	SchemaVersion int               `json:"schema_version"`
	Project       bool              `json:"project"`
	Items         []projectlessItem `json:"items"`
}

// readProjectlessCatalog answers a catalog read of a server with no
// configuration, from the discovered content alone. Any other URI cannot be
// read without a configuration, and is not listed either.
func (s *Server) readProjectlessCatalog(uri string, target authoringTarget) (*sdkmcp.ReadResourceResult, error) {
	if !target.catalog {
		return nil, resourceNotFound(uri)
	}
	doc := projectlessCatalog{SchemaVersion: govview.CatalogSchemaVersion, Items: []projectlessItem{}}
	for _, sk := range s.cat().Skills() {
		doc.Items = append(doc.Items, projectlessItem{Kind: "skill", ID: sk.Name, Path: sk.Source, Description: sk.Description, URI: sk.URI})
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, oops.Wrapf(err, "encode the catalog")
	}
	text, err := s.decorateCatalog(string(raw), target.query)
	if err != nil {
		return nil, err
	}
	return &sdkmcp.ReadResourceResult{Contents: []*sdkmcp.ResourceContents{{URI: uri, MIMEType: mimeJSON, Text: text}}}, nil
}
