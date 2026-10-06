// Package sbom renders the project's AI configuration as a bill of materials in
// CycloneDX 1.6 or SPDX 2.3 JSON: the authored items, the remote sources they
// come from and the MCP servers the harnesses start or call.
//
// The document is deterministic (no timestamp unless asked for, sorted,
// LF-normalised digests) and carries no secret: environment and header values
// are never read into it and URLs lose their userinfo and query. ai-rulez
// digests live only in namespaced properties (SPDX: the package comment), never
// in the standard hash fields, because they are tree digests of ai-rulez's own
// scheme, not plain file hashes.
package sbom

// SpecVersion is the CycloneDX version emitted.
const SpecVersion = "1.6"

// PropertyPrefix namespaces every ai-rulez property.
const PropertyPrefix = "ai-rulez:"

// BOM is a CycloneDX 1.6 document (the subset ai-rulez emits). It also carries
// what ToSPDX needs to render the same content as SPDX 2.3, and the findings of
// the build; none of that is encoded as CycloneDX.
type BOM struct {
	BOMFormat    string       `json:"bomFormat"`
	SpecVersion  string       `json:"specVersion"`
	SerialNumber string       `json:"serialNumber"`
	Version      int          `json:"version"`
	Metadata     Metadata     `json:"metadata"`
	Components   []Component  `json:"components"`
	Services     []Service    `json:"services,omitempty"`
	Dependencies []Dependency `json:"dependencies"`

	// Findings are the AR75x findings of the build (Finding), sorted.
	Findings []Finding `json:"-"`
	// LockPresent reports an ai-rulez.lock that pins content; LockInSync that the
	// pins match the sources (always false without a lock).
	LockPresent bool `json:"-"`
	LockInSync  bool `json:"-"`

	tool    string
	tree    string
	scope   string
	created string
}

type Metadata struct {
	Timestamp string    `json:"timestamp,omitempty"`
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
	Licenses           []LicenseChoice     `json:"licenses,omitempty"`
	Hashes             []Hash              `json:"hashes,omitempty"`
	PURL               string              `json:"purl,omitempty"`
	ExternalReferences []ExternalReference `json:"externalReferences,omitempty"`
	Properties         []Property          `json:"properties,omitempty"`
	Components         []Component         `json:"components,omitempty"`

	info *info
}

// LicenseChoice is a license or an SPDX license expression.
type LicenseChoice struct {
	License    *License `json:"license,omitempty"`
	Expression string   `json:"expression,omitempty"`
}

// License is a license named by its SPDX id, or by a free-form name when it is
// not an SPDX id.
type License struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Hash is a plain digest of a file's bytes.
type Hash struct {
	Alg     string `json:"alg"`
	Content string `json:"content"`
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

// info is what a component carries beyond its CycloneDX fields.
type info struct {
	// declaredLicense is the sanitized license text from the frontmatter.
	declaredLicense string
	// files are the item's files (hashed), sorted by path.
	files []fileEntry
	// commit and vcs describe a remote source for the SPDX downloadLocation.
	commit, vcs string
	// reviews are the approval annotations, signature the lock attestation's.
	reviews   []review
	signature *Signature
}

type fileEntry struct {
	path, sha256, sha1 string
	size               int64
	// hashed is false for a script listed without digests (--files none).
	hashed   bool
	executes bool
}

// review is one approving reviewer of an item, as an SPDX REVIEW annotation.
type review struct {
	reviewer, at, digest string
}
