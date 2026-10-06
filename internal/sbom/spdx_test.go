package sbom_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

const skillWithScript = "---\nname: deploy\ndescription: Ships it\nlicense: Apache-2.0\n---\nbody\n"

func richFiles() map[string]string {
	files := map[string]string{"skills/deploy/SKILL.md": skillWithScript, "skills/deploy/references/notes.md": "notes\n"}
	for k, v := range sampleFiles {
		files[k] = v
	}
	return files
}

func richFixture(t *testing.T, cfg string) *fixture {
	t.Helper()
	f := newFixture(t, cfg, richFiles())
	f.writeExec("skills/deploy/scripts/run.sh", "#!/bin/sh\necho hi\n")
	return f
}

func TestVendoredSchemasAreTheReviewedOnes(t *testing.T) {
	// Arrange: updating a vendored schema is a reviewed change, so its digest is pinned here.
	want := map[string]string{
		"bom-1.6.schema.json":  "18f57f7482593bad9f21b4feed09084640cbeff419d62ad5090c5ceccca5b37d",
		"jsf-0.82.schema.json": "8bae002c25e723db7ee1f26afde680ae1a2b1a8f6b4b4b0fd65dc3becb090aae",
		"spdx.schema.json":     "4b345e2329f209f34e960ae2a8e7cb46a166907e6a45e94978565925dc47b359",
		"spdx-2.3.schema.json": "4126dc29f15e92feec92e9622f4939131ad58b125d6f578aa93e980a6e6f3212",
	}

	for name, digest := range want {
		t.Run(name, func(t *testing.T) {
			// Act
			data, err := os.ReadFile(filepath.Join("testdata", name))

			// Assert
			require.NoError(t, err)
			assert.Equal(t, digest, sha256Hex(string(data)))
		})
	}
}

func TestSPDXOutputValidatesAgainstSPDX23(t *testing.T) {
	// Arrange
	schema := spdxSchema(t)
	files := richFixture(t, baseConfig+mcpConfig+`
[[mcp_servers]]
name = "pinned"
command = "npx"
args = ["-y", "srv@1.2.3"]

[[includes]]
name = "shared"
source = "https://github.com/Org/Rules.git"
ref = "main"
`)
	approved := richFixture(t, baseConfig+"\n[governance]\nrequire_approval = [\"local\"]\n")
	approved.lock(func(l *lockfile.File) {
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "deploy", Digest: approved.digestOf("skill", "deploy"), Reviewer: "Alice@Example.org", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-03-04T05:06:07Z"})
	})
	cases := map[string]struct {
		f    *fixture
		opts sbom.Options
	}{
		"empty":                {newFixture(t, baseConfig, nil), sbom.Options{}},
		"items and mcp":        {files, sbom.Options{}},
		"with files":           {files, sbom.Options{Files: sbom.FilesAll}},
		"skill files":          {files, sbom.Options{Files: sbom.FilesSkills}},
		"timestamp":            {files, sbom.Options{Timestamp: mustTime(t, "2026-05-06T07:08:09Z")}},
		"approvals":            {approved, sbom.Options{Files: sbom.FilesAll}},
		"redacted":             {approved, sbom.Options{RedactReviewers: true}},
		"signature":            {files, sbom.Options{Signature: &sbom.Signature{Status: sbom.SignatureVerified, Signer: "sha256:abc"}}},
		"outputs":              {files, sbom.Options{IncludeOutputs: true}},
		"signature, no signer": {files, sbom.Options{Signature: &sbom.Signature{Status: sbom.SignatureAbsent}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Act
			_, doc := tc.f.spdx(tc.opts)

			// Assert
			requireValid(t, schema, doc)
		})
	}
}

func TestSPDXOutputIsDeterministic(t *testing.T) {
	// Arrange
	lf := richFixture(t, baseConfig+mcpConfig)
	crlfFiles := map[string]string{}
	for k, v := range richFiles() {
		crlfFiles[k] = strings.ReplaceAll(v, "\n", "\r\n")
	}
	crlf := newFixture(t, baseConfig+mcpConfig, crlfFiles)
	crlf.writeExec("skills/deploy/scripts/run.sh", "#!/bin/sh\necho hi\n") // scripts are digested byte for byte

	// Act
	_, first := lf.spdx(sbom.Options{})
	_, second := lf.spdx(sbom.Options{})
	_, third := crlf.spdx(sbom.Options{})

	// Assert
	assert.Equal(t, first, second, "two runs")
	assert.Equal(t, first, third, "CRLF sources: nothing is hashed by default")
}

func TestSPDXIdentifiersAreUniqueAndReferencesResolve(t *testing.T) {
	// Arrange
	f := richFixture(t, baseConfig+mcpConfig+"\n[[roles]]\nname = \"lean\"\n")

	// Act
	doc, raw := f.spdx(sbom.Options{Files: sbom.FilesAll})

	// Assert
	idPattern := regexp.MustCompile(`^SPDXRef-[A-Za-z0-9.-]+$`)
	ids := map[string]bool{doc.SPDXID: true}
	for _, p := range doc.Packages {
		assert.Regexp(t, idPattern, p.SPDXID)
		assert.False(t, ids[p.SPDXID], "duplicate %s", p.SPDXID)
		ids[p.SPDXID] = true
	}
	for _, file := range doc.Files {
		assert.Regexp(t, idPattern, file.SPDXID)
		assert.False(t, ids[file.SPDXID], "duplicate %s", file.SPDXID)
		ids[file.SPDXID] = true
	}
	require.NotEmpty(t, doc.Relationships)
	for _, r := range doc.Relationships {
		assert.True(t, ids[r.Element], "unknown element %s", r.Element)
		assert.True(t, ids[r.Related], "unknown related element %s", r.Related)
	}
	assert.Contains(t, doc.Relationships, sbom.Relationship{Element: "SPDXRef-DOCUMENT", Type: "DESCRIBES", Related: "SPDXRef-Config"})
	assert.Equal(t, "urn:ai-rulez:sbom:spdx:", doc.DocumentNamespace[:len("urn:ai-rulez:sbom:spdx:")])
	assert.Equal(t, "CC0-1.0", doc.DataLicense)
	assert.NotContains(t, raw, "env-secret-value")
	assert.NotContains(t, raw, "header-secret")
	assert.NotContains(t, raw, "url-secret")
}

func TestSPDXRelationshipsFollowTheKindOfThing(t *testing.T) {
	// Arrange
	f := richFixture(t, baseConfig+mcpConfig+`
[[includes]]
name = "shared"
source = "https://github.com/Org/Rules.git"
ref = "main"
`)

	// Act
	doc, _ := f.spdx(sbom.Options{Files: sbom.FilesSkills})

	// Assert
	typeOf := map[string]string{} // related name -> relationship type of Config
	nameOf := map[string]string{}
	for _, p := range doc.Packages {
		nameOf[p.SPDXID] = p.Name
	}
	for _, r := range doc.Relationships {
		if r.Element == "SPDXRef-Config" {
			typeOf[nameOf[r.Related]] = r.Type
		}
	}
	assert.Equal(t, "CONTAINS", typeOf["skill/deploy"])
	assert.Equal(t, "CONTAINS", typeOf["rule/r1"])
	assert.Equal(t, "DEPENDS_ON", typeOf["shared"])
	assert.Equal(t, "DEPENDS_ON", typeOf["fs"])
	assert.Equal(t, "DEPENDS_ON", typeOf["remote"])
	contains := 0
	for _, r := range doc.Relationships {
		if r.Type == "CONTAINS" && strings.Contains(r.Related, "file") {
			contains++
		}
	}
	assert.Equal(t, 4, contains, "deploy: SKILL.md, scripts/run.sh and references/notes.md; s1: SKILL.md")
}

func TestSPDXPackagesCarryPurlAndDownloadLocation(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+`
[[mcp_servers]]
name = "pinned"
command = "npx"
args = ["-y", "@scope/srv@1.2.3"]

[[includes]]
name = "shared"
source = "https://github.com/Org/Rules.git"
ref = "0123456789abcdef0123456789abcdef01234567"
`, nil)

	// Act
	doc, _ := f.spdx(sbom.Options{})

	// Assert
	byName := map[string]sbom.SPDXPackage{}
	for _, p := range doc.Packages {
		byName[p.Name] = p
	}
	require.Contains(t, byName, "pinned")
	assert.Equal(t, []sbom.ExternalRef{{Category: "PACKAGE-MANAGER", Type: "purl", Locator: "pkg:npm/%40scope/srv@1.2.3"}}, byName["pinned"].ExternalRefs)
	assert.Equal(t, "1.2.3", byName["pinned"].VersionInfo)
	require.Contains(t, byName, "shared")
	assert.Equal(t, "git+https://github.com/Org/Rules@0123456789abcdef0123456789abcdef01234567", byName["shared"].DownloadLocation)
}

func TestSPDXCreationTime(t *testing.T) {
	tests := []struct {
		name        string
		opts        sbom.Options
		wantCreated string
		wantComment bool
	}{
		{"fixed placeholder", sbom.Options{}, "1970-01-01T00:00:00Z", true},
		{"timestamp", sbom.Options{Timestamp: mustTime(t, "2026-05-06T09:08:09+02:00")}, "2026-05-06T07:08:09Z", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t, baseConfig, nil)

			// Act
			doc, _ := f.spdx(tt.opts)

			// Assert
			assert.Equal(t, tt.wantCreated, doc.CreationInfo.Created)
			assert.Equal(t, tt.wantComment, doc.CreationInfo.Comment != "")
			assert.Equal(t, []string{"Tool: ai-rulez-9.9.9"}, doc.CreationInfo.Creators)
		})
	}
}

func TestSPDXFilesCarryPlainChecksums(t *testing.T) {
	// Arrange
	f := richFixture(t, baseConfig)

	// Act
	doc, _ := f.spdx(sbom.Options{Files: sbom.FilesAll})

	// Assert
	byName := map[string]sbom.SPDXFile{}
	for _, file := range doc.Files {
		byName[file.FileName] = file
	}
	script := byName["./skills/deploy/scripts/run.sh"]
	require.NotEmpty(t, script.SPDXID)
	assert.Equal(t, []string{"SOURCE"}, script.FileTypes)
	assert.Equal(t, "SHA1", script.Checksums[0].Algorithm)
	assert.Equal(t, "SHA256", script.Checksums[1].Algorithm)
	assert.Equal(t, sha256Hex("#!/bin/sh\necho hi\n"), script.Checksums[1].Value)
	assert.Equal(t, []string{"TEXT"}, byName["./skills/deploy/references/notes.md"].FileTypes)
	var skill sbom.SPDXPackage
	for _, p := range doc.Packages {
		if p.Name == "skill/deploy" {
			skill = p
		}
	}
	assert.True(t, skill.FilesAnalyzed)
	require.NotNil(t, skill.PackageVerificationCode)
	assert.Len(t, skill.PackageVerificationCode.Value, 40)
	assert.Equal(t, "Apache-2.0", skill.LicenseDeclared)
	assert.Equal(t, "Ships it", skill.Description)
}
