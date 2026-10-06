// Package sbom renders the project's AI configuration as a CycloneDX 1.6 JSON
// bill of materials: the authored items, the remote sources they come from and
// the MCP servers the harnesses start or call.
//
// The document is deterministic (no timestamp, sorted, LF-normalised digests)
// and carries no secret: environment and header values are never read into it
// and URLs lose their userinfo and query. ai-rulez digests live only in
// namespaced properties, never in the standard `hashes` field, because they are
// tree digests of ai-rulez's own scheme, not plain file hashes.
package sbom

// SpecVersion is the CycloneDX version emitted.
const SpecVersion = "1.6"

// PropertyPrefix namespaces every ai-rulez property.
const PropertyPrefix = "ai-rulez:"

// BOM is a CycloneDX 1.6 document (the subset ai-rulez emits).
type BOM struct {
	BOMFormat    string       `json:"bomFormat"`
	SpecVersion  string       `json:"specVersion"`
	SerialNumber string       `json:"serialNumber"`
	Version      int          `json:"version"`
	Metadata     Metadata     `json:"metadata"`
	Components   []Component  `json:"components"`
	Services     []Service    `json:"services,omitempty"`
	Dependencies []Dependency `json:"dependencies"`
}

type Metadata struct {
	Tools     Tools     `json:"tools"`
	Component Component `json:"component"`
}

type Tools struct {
	Components []Component `json:"components"`
}

type Component struct {
	Type               string              `json:"type"`
	BOMRef             string              `json:"bom-ref,omitempty"`
	Group              string              `json:"group,omitempty"`
	Name               string              `json:"name"`
	Version            string              `json:"version,omitempty"`
	Description        string              `json:"description,omitempty"`
	PURL               string              `json:"purl,omitempty"`
	ExternalReferences []ExternalReference `json:"externalReferences,omitempty"`
	Properties         []Property          `json:"properties,omitempty"`
}

type ExternalReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Service struct {
	BOMRef        string     `json:"bom-ref,omitempty"`
	Name          string     `json:"name"`
	Endpoints     []string   `json:"endpoints,omitempty"`
	Authenticated bool       `json:"authenticated"`
	TrustBoundary bool       `json:"x-trust-boundary"`
	Properties    []Property `json:"properties,omitempty"`
}

type Dependency struct {
	Ref       string   `json:"ref"`
	DependsOn []string `json:"dependsOn,omitempty"`
}
