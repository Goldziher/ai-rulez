package sbom

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // the SPDX package verification code is defined over SHA-1
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/samber/oops"
)

// SPDXVersion is the SPDX specification version emitted.
const SPDXVersion = "SPDX-2.3"

const (
	noAssertion = "NOASSERTION"
	// createdSentinel is SPDX's required creation time when none is given: a
	// fixed value keeps the document reproducible.
	createdSentinel = "1970-01-01T00:00:00Z"
	configID        = "SPDXRef-Config"
)

// SPDX is an SPDX 2.3 JSON document (the subset ai-rulez emits).
type SPDX struct {
	SPDXVersion                string             `json:"spdxVersion"`
	DataLicense                string             `json:"dataLicense"`
	SPDXID                     string             `json:"SPDXID"`
	Name                       string             `json:"name"`
	DocumentNamespace          string             `json:"documentNamespace"`
	CreationInfo               CreationInfo       `json:"creationInfo"`
	Packages                   []SPDXPackage      `json:"packages"`
	Files                      []SPDXFile         `json:"files,omitempty"`
	HasExtractedLicensingInfos []ExtractedLicense `json:"hasExtractedLicensingInfos,omitempty"`
	Relationships              []Relationship     `json:"relationships"`
}

type CreationInfo struct {
	Comment  string   `json:"comment,omitempty"`
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type SPDXPackage struct {
	SPDXID                  string            `json:"SPDXID"`
	Name                    string            `json:"name"`
	VersionInfo             string            `json:"versionInfo,omitempty"`
	DownloadLocation        string            `json:"downloadLocation"`
	FilesAnalyzed           bool              `json:"filesAnalyzed"`
	PackageVerificationCode *VerificationCode `json:"packageVerificationCode,omitempty"`
	LicenseConcluded        string            `json:"licenseConcluded"`
	LicenseDeclared         string            `json:"licenseDeclared"`
	LicenseInfoFromFiles    []string          `json:"licenseInfoFromFiles,omitempty"`
	CopyrightText           string            `json:"copyrightText"`
	Description             string            `json:"description,omitempty"`
	Comment                 string            `json:"comment,omitempty"`
	ExternalRefs            []ExternalRef     `json:"externalRefs,omitempty"`
	Annotations             []Annotation      `json:"annotations,omitempty"`
}

type VerificationCode struct {
	Value string `json:"packageVerificationCodeValue"`
}

type ExternalRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

type Annotation struct {
	Annotator string `json:"annotator"`
	Date      string `json:"annotationDate"`
	Type      string `json:"annotationType"`
	Comment   string `json:"comment"`
}

type SPDXFile struct {
	SPDXID             string     `json:"SPDXID"`
	FileName           string     `json:"fileName"`
	Checksums          []Checksum `json:"checksums"`
	FileTypes          []string   `json:"fileTypes,omitempty"`
	LicenseConcluded   string     `json:"licenseConcluded"`
	LicenseInfoInFiles []string   `json:"licenseInfoInFiles"`
	CopyrightText      string     `json:"copyrightText"`
}

type Checksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"checksumValue"`
}

type ExtractedLicense struct {
	LicenseID     string `json:"licenseId"`
	ExtractedText string `json:"extractedText"`
	Name          string `json:"name,omitempty"`
}

type Relationship struct {
	Element string `json:"spdxElementId"`
	Type    string `json:"relationshipType"`
	Related string `json:"relatedSpdxElement"`
}

// spdxID makes an SPDX identifier of a bom-ref: the characters SPDX does not
// allow are replaced, and a hash of the original is appended so two refs never
// share an identifier.
func spdxID(ref string) string {
	var b strings.Builder
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	sum := sha256.Sum256([]byte(ref))
	return "SPDXRef-" + b.String() + "-" + hex.EncodeToString(sum[:4])
}

// ToSPDX renders the document as SPDX 2.3: every component and service is a
// package, the files of an item are files related to it by CONTAINS, and the
// project describes them (CONTAINS for authored items, DEPENDS_ON for the
// remote sources, MCP servers and outputs it relies on).
func (b *BOM) ToSPDX() *SPDX {
	created := b.created
	comment := ""
	if created == "" {
		created = createdSentinel
		comment = "created is a fixed placeholder so the document is reproducible; pass --timestamp (or set SOURCE_DATE_EPOCH) for the time of generation"
	}
	doc := &SPDX{
		SPDXVersion:       SPDXVersion,
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              b.Metadata.Component.Name,
		DocumentNamespace: "urn:ai-rulez:sbom:spdx:" + strings.TrimPrefix(b.SerialNumber, "urn:uuid:"),
		CreationInfo:      CreationInfo{Comment: comment, Created: created, Creators: []string{"Tool: ai-rulez-" + b.tool}},
	}
	lics := &licenseTable{}
	ids := map[string]string{projectRef: configID}
	kinds := map[string]string{}

	root := b.spdxPackage(&b.Metadata.Component, configID, lics)
	if sig := b.Metadata.Component.info; sig != nil && sig.signature != nil {
		root.Annotations = append(root.Annotations, signatureAnnotation(sig.signature, b.tool, created))
	}
	doc.Packages = append(doc.Packages, root)
	rels := []Relationship{{Element: doc.SPDXID, Type: "DESCRIBES", Related: configID}}

	for i := range b.Components {
		c := &b.Components[i]
		id := spdxID(c.BOMRef)
		ids[c.BOMRef] = id
		kinds[c.BOMRef] = propOf(c, "kind")
		pkg := b.spdxPackage(c, id, lics)
		if files := c.info.hashedFiles(); len(files) > 0 {
			pkg.FilesAnalyzed = true
			pkg.PackageVerificationCode = &VerificationCode{Value: verificationCode(files)}
			pkg.LicenseInfoFromFiles = []string{noAssertion}
			for _, f := range files {
				fid := spdxID(c.BOMRef + ":file:" + f.path)
				doc.Files = append(doc.Files, spdxFile(fid, f))
				rels = append(rels, Relationship{Element: id, Type: "CONTAINS", Related: fid})
			}
		}
		doc.Packages = append(doc.Packages, pkg)
	}
	for i := range b.Services {
		s := &b.Services[i]
		id := spdxID(s.BOMRef)
		ids[s.BOMRef] = id
		kinds[s.BOMRef] = "service"
		doc.Packages = append(doc.Packages, servicePackage(s, id))
	}
	rels = append(rels, b.spdxDependencies(ids, kinds)...)
	rest := doc.Packages[1:]
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].SPDXID < rest[j].SPDXID })
	sort.SliceStable(doc.Files, func(i, j int) bool { return doc.Files[i].SPDXID < doc.Files[j].SPDXID })
	sort.SliceStable(rels, func(i, j int) bool {
		a, c := rels[i], rels[j]
		switch {
		case a.Element != c.Element:
			return a.Element < c.Element
		case a.Type != c.Type:
			return a.Type < c.Type
		}
		return a.Related < c.Related
	})
	doc.Relationships = rels
	doc.HasExtractedLicensingInfos = lics.extracted()
	return doc
}

// hashedFiles are the files that have checksums: the only ones SPDX can list.
func (i *info) hashedFiles() []fileEntry {
	if i == nil {
		return nil
	}
	var out []fileEntry
	for _, f := range i.files {
		if f.hashed {
			out = append(out, f)
		}
	}
	return out
}

// servicePackage is a remote MCP server as a package: SPDX 2.3 has no service type.
func servicePackage(s *Service, id string) SPDXPackage {
	comment := "remote service"
	if len(s.Endpoints) > 0 {
		comment += " " + s.Endpoints[0]
	}
	return SPDXPackage{
		SPDXID: id, Name: s.Name, DownloadLocation: noAssertion, LicenseConcluded: noAssertion,
		LicenseDeclared: noAssertion, CopyrightText: noAssertion,
		Comment: comment + "\n" + propertyLines(s.Properties),
	}
}

// spdxDependencies turns the dependency graph into relationships: the project
// contains the authored items and depends on sources, MCP servers, services and
// outputs; everything else depends on what it names.
func (b *BOM) spdxDependencies(ids, kinds map[string]string) []Relationship {
	var rels []Relationship
	for _, d := range b.Dependencies {
		from, ok := ids[d.Ref]
		if !ok {
			continue
		}
		for _, target := range d.DependsOn {
			to, ok := ids[target]
			if !ok {
				continue
			}
			typ := "DEPENDS_ON"
			if d.Ref == projectRef {
				switch kinds[target] {
				case "source", "mcp-server", "output", "service":
				default:
					typ = "CONTAINS"
				}
			}
			rels = append(rels, Relationship{Element: from, Type: typ, Related: to})
		}
	}
	return rels
}

func (b *BOM) spdxPackage(c *Component, id string, lics *licenseTable) SPDXPackage {
	name := c.Name
	if c.Group != "" {
		name = c.Group + "/" + c.Name
	}
	pkg := SPDXPackage{
		SPDXID: id, Name: name, VersionInfo: c.Version, DownloadLocation: noAssertion,
		LicenseConcluded: noAssertion, LicenseDeclared: noAssertion, CopyrightText: noAssertion,
		Description: c.Description, Comment: propertyLines(c.Properties),
	}
	if c.PURL != "" {
		pkg.ExternalRefs = []ExternalRef{{Category: "PACKAGE-MANAGER", Type: "purl", Locator: c.PURL}}
	}
	if c.info == nil {
		return pkg
	}
	if c.info.vcs != "" {
		pkg.DownloadLocation = "git+" + c.info.vcs
		if c.info.commit != "" {
			pkg.DownloadLocation += "@" + c.info.commit
		}
	}
	if c.info.declaredLicense != "" {
		pkg.LicenseDeclared = lics.declare(c.info.declaredLicense)
	}
	for _, r := range c.info.reviews {
		pkg.Annotations = append(pkg.Annotations, Annotation{
			Annotator: "Person: " + r.reviewer, Date: spdxTime(r.at), Type: "REVIEW",
			Comment: "approved " + r.digest + " (assurance " + r.assurance + ")",
		})
	}
	sort.SliceStable(pkg.Annotations, func(i, j int) bool {
		a, c := pkg.Annotations[i], pkg.Annotations[j]
		if a.Annotator != c.Annotator {
			return a.Annotator < c.Annotator
		}
		return a.Date < c.Date
	})
	return pkg
}

func signatureAnnotation(s *Signature, tool, date string) Annotation {
	text := "lock attestation " + s.Status
	if s.Signer != "" {
		text += "; signer " + s.Signer
	}
	if s.Issuer != "" {
		text += "; issuer " + s.Issuer
	}
	if !s.SignedAt.IsZero() {
		text += "; signed at " + s.SignedAt.UTC().Format(time.RFC3339)
	}
	if s.Code != "" {
		text += "; " + s.Code
	}
	return Annotation{Annotator: "Tool: ai-rulez-" + tool, Date: date, Type: "OTHER", Comment: text}
}

// spdxTime formats an RFC 3339 time as SPDX wants it (UTC, whole seconds); a
// value that does not parse becomes the sentinel.
func spdxTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return createdSentinel
	}
	return t.UTC().Format(time.RFC3339)
}

func propertyLines(props []Property) string {
	lines := make([]string, 0, len(props))
	for _, p := range props {
		lines = append(lines, p.Name+"="+p.Value)
	}
	return strings.Join(lines, "\n")
}

func spdxFile(id string, f fileEntry) SPDXFile {
	return SPDXFile{
		SPDXID: id, FileName: "./" + f.path,
		Checksums:        []Checksum{{Algorithm: "SHA1", Value: f.sha1}, {Algorithm: "SHA256", Value: f.sha256}},
		FileTypes:        []string{fileType(f)},
		LicenseConcluded: noAssertion, LicenseInfoInFiles: []string{noAssertion}, CopyrightText: noAssertion,
	}
}

func fileType(f fileEntry) string {
	if f.executes {
		return "SOURCE"
	}
	switch strings.ToLower(path.Ext(f.path)) {
	case ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".csv", ".xml", ".html":
		return "TEXT"
	}
	return "OTHER"
}

// verificationCode is the SPDX package verification code: the SHA-1 of the
// sorted SHA-1 checksums of the package's files, concatenated.
func verificationCode(files []fileEntry) string {
	sums := make([]string, len(files))
	for i, f := range files {
		sums[i] = f.sha1
	}
	sort.Strings(sums)
	sum := sha1.Sum([]byte(strings.Join(sums, ""))) //nolint:gosec // see import
	return hex.EncodeToString(sum[:])
}

// licenseTable maps declared licenses to SPDX expressions, with a LicenseRef for
// the ones that are not SPDX ids.
type licenseTable struct {
	refs map[string]ExtractedLicense
}

func (t *licenseTable) declare(raw string) string {
	kind, text := classifyLicense(raw)
	switch kind {
	case licenseID, licenseExpression:
		return text
	case licenseName:
		sum := sha256.Sum256([]byte(text))
		id := "LicenseRef-" + hex.EncodeToString(sum[:4])
		if t.refs == nil {
			t.refs = map[string]ExtractedLicense{}
		}
		t.refs[id] = ExtractedLicense{
			LicenseID: id, Name: text,
			ExtractedText: "The license text is not available; the item's frontmatter declares: " + text,
		}
		return id
	}
	return noAssertion
}

func (t *licenseTable) extracted() []ExtractedLicense {
	out := make([]ExtractedLicense, 0, len(t.refs))
	for _, l := range t.refs {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LicenseID < out[j].LicenseID })
	return out
}

// WriteSPDX encodes doc as indented JSON with a trailing newline.
func WriteSPDX(w io.Writer, doc *SPDX) error { return encode(w, doc) }

func encode(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return oops.Wrapf(err, "encode sbom")
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return oops.Wrapf(err, "write sbom")
	}
	return nil
}
