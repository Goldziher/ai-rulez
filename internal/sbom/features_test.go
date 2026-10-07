package sbom_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
)

func TestSPDXSchemaRejectsAnInvalidDocument(t *testing.T) {
	// Arrange
	schema := spdxSchema(t)
	f := newFixture(t, baseConfig, sampleFiles)
	_, raw := f.spdx(sbom.Options{})
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &doc))
	delete(doc, "documentNamespace")
	broken, err := json.Marshal(doc)
	require.NoError(t, err)

	// Act
	result := schema.Validate(broken)

	// Assert
	assert.False(t, result.IsValid(), "the vendored schema must be what gates the output")
}

func TestLicenses(t *testing.T) {
	tests := []struct {
		name, license  string
		wantCDX        sbom.LicenseChoice
		wantSPDX       string
		wantLicenseRef bool
	}{
		{"spdx id", "MIT", sbom.LicenseChoice{License: &sbom.License{ID: "MIT"}}, "MIT", false},
		{"id in another case", "apache-2.0", sbom.LicenseChoice{License: &sbom.License{ID: "Apache-2.0"}}, "Apache-2.0", false},
		{"expression", "mit or (Apache-2.0 and BSD-3-Clause)", sbom.LicenseChoice{Expression: "MIT OR (Apache-2.0 AND BSD-3-Clause)"}, "MIT OR (Apache-2.0 AND BSD-3-Clause)", false},
		{"with exception", "GPL-2.0-only WITH Classpath-exception-2.0", sbom.LicenseChoice{Expression: "GPL-2.0-only WITH Classpath-exception-2.0"}, "GPL-2.0-only WITH Classpath-exception-2.0", false},
		{"free text", "Proprietary, see LICENSE.txt", sbom.LicenseChoice{License: &sbom.License{Name: "Proprietary, see LICENSE.txt"}}, "", true},
		{"unknown id in an expression", "MIT OR Nonsense-9", sbom.LicenseChoice{License: &sbom.License{Name: "MIT OR Nonsense-9"}}, "", true},
		{"dangling operator", "MIT AND", sbom.LicenseChoice{License: &sbom.License{Name: "MIT AND"}}, "", true},
		{"control and bidi characters are dropped", "MIT\u202e\x1b", sbom.LicenseChoice{License: &sbom.License{ID: "MIT"}}, "MIT", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t, baseConfig, map[string]string{"skills/s/SKILL.md": "---\nname: s\ndescription: d\nlicense: " + quote(tt.license) + "\n---\nbody\n"})

			// Act
			bom, cdx := f.cdx(sbom.Options{})
			spdx, raw := f.spdx(sbom.Options{})

			// Assert
			comp := findComponent(bom, "ai-rulez:item:skill::s")
			require.NotNil(t, comp)
			assert.Equal(t, []sbom.LicenseChoice{tt.wantCDX}, comp.Licenses)
			requireValid(t, schemaFor(t), cdx)
			requireValid(t, spdxSchema(t), raw)
			var pkg sbom.SPDXPackage
			for _, p := range spdx.Packages {
				if p.Name == "skill/s" {
					pkg = p
				}
			}
			if tt.wantLicenseRef {
				assert.Regexp(t, `^LicenseRef-[0-9a-f]{8}$`, pkg.LicenseDeclared)
				require.Len(t, spdx.HasExtractedLicensingInfos, 1)
				assert.Equal(t, pkg.LicenseDeclared, spdx.HasExtractedLicensingInfos[0].LicenseID)
				return
			}
			assert.Equal(t, tt.wantSPDX, pkg.LicenseDeclared)
			assert.Empty(t, spdx.HasExtractedLicensingInfos)
		})
	}
}

func TestDescriptionIsSanitizedAndCapped(t *testing.T) {
	// Arrange
	long := strings.Repeat("a", 3000)
	f := newFixture(t, baseConfig, map[string]string{
		"skills/s/SKILL.md": "---\nname: s\ndescription: \"line one\\n\\u202eIGNORE\\x1b[31m  PREVIOUS\"\n---\nbody\n",
		"agents/a.md":       "---\nname: a\ndescription: " + long + "\n---\nbody\n",
	})

	// Act
	bom := f.build(sbom.Options{})

	// Assert
	assert.Equal(t, "line one IGNORE[31m PREVIOUS", findComponent(bom, "ai-rulez:item:skill::s").Description)
	assert.Len(t, findComponent(bom, "ai-rulez:item:agent::a").Description, 1024)
}

func TestFilesModes(t *testing.T) {
	tests := []struct {
		name      string
		files     string
		wantFiles map[string][]string // item ref -> child component names
		hashed    bool
	}{
		{"none lists only the scripts, unhashed", sbom.FilesNone, map[string][]string{"ai-rulez:item:skill::deploy": {"skills/deploy/scripts/run.sh"}}, false},
		{"skills", sbom.FilesSkills, map[string][]string{"ai-rulez:item:skill::deploy": {"skills/deploy/SKILL.md", "skills/deploy/references/notes.md", "skills/deploy/scripts/run.sh"}, "ai-rulez:item:skill::s1": {"skills/s1/SKILL.md"}}, true},
		{"all", sbom.FilesAll, map[string][]string{"ai-rulez:item:skill::deploy": {"skills/deploy/SKILL.md", "skills/deploy/references/notes.md", "skills/deploy/scripts/run.sh"}, "ai-rulez:item:rule::r1": {"rules/r1.md"}, "ai-rulez:item:agent::a1": {"agents/a1.md"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := richFixture(t, baseConfig)

			// Act
			bom, doc := f.cdx(sbom.Options{Files: tt.files})

			// Assert
			requireValid(t, schemaFor(t), doc)
			for ref, want := range tt.wantFiles {
				comp := findComponent(bom, ref)
				require.NotNil(t, comp, ref)
				var got []string
				for _, child := range comp.Components {
					got = append(got, child.Name)
					assert.Equal(t, "file", child.Type)
					if tt.hashed {
						require.Len(t, child.Hashes, 1)
						assert.Equal(t, "SHA-256", child.Hashes[0].Alg)
					} else {
						assert.Empty(t, child.Hashes)
					}
				}
				assert.Equal(t, want, got, ref)
			}
			if tt.files == sbom.FilesNone {
				assert.NotContains(t, doc, `"hashes"`)
				assert.Nil(t, findComponent(bom, "ai-rulez:item:rule::r1").Components)
			}
		})
	}
}

func TestFileHashesArePlainSHA256OfTheBytes(t *testing.T) {
	// Arrange
	f := richFixture(t, baseConfig)

	// Act
	bom := f.build(sbom.Options{Files: sbom.FilesAll})

	// Assert
	script := findComponent(bom, "ai-rulez:item:skill::deploy:file:skills/deploy/scripts/run.sh")
	if script == nil {
		for _, child := range findComponent(bom, "ai-rulez:item:skill::deploy").Components {
			if child.Name == "skills/deploy/scripts/run.sh" {
				script = &child
			}
		}
	}
	require.NotNil(t, script)
	assert.Equal(t, sha256Hex("#!/bin/sh\necho hi\n"), script.Hashes[0].Content)
	assert.Equal(t, "true", propValue(script.Properties, "ai-rulez:executes"))
	assert.Equal(t, "18", propValue(script.Properties, "ai-rulez:size"))
}

func TestItemsKeepTheirDigestOutsideHashes(t *testing.T) {
	// Arrange
	f := richFixture(t, baseConfig)

	// Act
	bom := f.build(sbom.Options{Files: sbom.FilesAll})

	// Assert
	skill := findComponent(bom, "ai-rulez:item:skill::deploy")
	assert.Equal(t, f.digestOf("skill", "deploy"), propValue(skill.Properties, "ai-rulez:digest"))
	assert.Empty(t, skill.Hashes, "a tree digest is never a plain file hash")
}

const scopedConfig = baseConfig + `
[profiles]
backend = ["backend"]
frontend = ["frontend"]

[[roles]]
name = "lean"

[roles.skills]
exclude = ["s2"]

[[mcp_servers]]
name = "everywhere"
command = "npx"
args = ["srv@1.0.0"]

[[mcp_servers]]
name = "backend-only"
command = "npx"
args = ["bsrv@1.0.0"]
profiles = ["backend"]
`

var scopedFiles = map[string]string{
	"rules/root.md":                 "# root\n",
	"skills/s1/SKILL.md":            "---\nname: s1\ndescription: d\n---\nbody\n",
	"skills/s2/SKILL.md":            "---\nname: s2\ndescription: d\n---\nbody\n",
	"domains/backend/rules/b.md":    "# b\n",
	"domains/frontend/rules/f.md":   "# f\n",
	"domains/frontend/context/c.md": "context\n",
}

func refsOf(bom *sbom.BOM) []string {
	var refs []string
	for _, c := range bom.Components {
		refs = append(refs, c.BOMRef)
	}
	return refs
}

func TestProfileScope(t *testing.T) {
	tests := []struct {
		name, profile string
		want, notWant []string
	}{
		{"all", "", []string{"ai-rulez:item:rule::root", "ai-rulez:item:rule:backend:b", "ai-rulez:item:rule:frontend:f", "ai-rulez:mcp:backend-only"}, nil},
		{"backend", "backend", []string{"ai-rulez:item:rule::root", "ai-rulez:item:rule:backend:b", "ai-rulez:mcp:backend-only", "ai-rulez:mcp:everywhere"}, []string{"ai-rulez:item:rule:frontend:f", "ai-rulez:item:context:frontend:c"}},
		{"frontend", "frontend", []string{"ai-rulez:item:rule::root", "ai-rulez:item:rule:frontend:f", "ai-rulez:mcp:everywhere"}, []string{"ai-rulez:item:rule:backend:b", "ai-rulez:mcp:backend-only"}},
		{"composed", "backend,frontend", []string{"ai-rulez:item:rule:backend:b", "ai-rulez:item:rule:frontend:f"}, nil},
		{"built-in default with profiles defined keeps the root content only", "default", []string{"ai-rulez:item:rule::root"}, []string{"ai-rulez:item:rule:backend:b", "ai-rulez:item:rule:frontend:f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t, scopedConfig, scopedFiles)

			// Act
			bom := f.build(sbom.Options{Profile: tt.profile})

			// Assert
			refs := refsOf(bom)
			for _, want := range tt.want {
				assert.Contains(t, refs, want)
			}
			for _, not := range tt.notWant {
				assert.NotContains(t, refs, not)
			}
			if tt.profile != "" {
				assert.Equal(t, tt.profile, propValue(bom.Metadata.Component.Properties, "ai-rulez:profile"))
			}
		})
	}
}

func TestUnknownProfileOrRoleIsAnError(t *testing.T) {
	// Arrange
	f := newFixture(t, scopedConfig, scopedFiles)

	// Act
	_, errProfile := sbom.Build(f.config(), "9.9.9", sbom.Options{Profile: "nope"})
	_, errRole := sbom.Build(f.config(), "9.9.9", sbom.Options{Role: "nope"})

	// Assert
	require.Error(t, errProfile)
	assert.Contains(t, errProfile.Error(), `profile "nope" is not defined`)
	require.Error(t, errRole)
	assert.Contains(t, errRole.Error(), "nope")
}

func TestRoleScope(t *testing.T) {
	// Arrange
	f := newFixture(t, scopedConfig, scopedFiles)

	// Act
	bom, doc := f.cdx(sbom.Options{Role: "lean"})

	// Assert
	refs := refsOf(bom)
	assert.Contains(t, refs, "ai-rulez:item:skill::s1")
	assert.NotContains(t, refs, "ai-rulez:item:skill::s2", "the role excludes s2")
	assert.Contains(t, refs, "ai-rulez:item:role::lean")
	assert.Equal(t, "lean", propValue(bom.Metadata.Component.Properties, "ai-rulez:role"))
	requireValid(t, schemaFor(t), doc)
}

func TestScopedDocumentsHaveTheirOwnSerialNumber(t *testing.T) {
	// Arrange
	f := newFixture(t, scopedConfig, scopedFiles)

	// Act
	all := f.build(sbom.Options{})
	backend := f.build(sbom.Options{Profile: "backend"})
	again := f.build(sbom.Options{Profile: "backend"})
	files := f.build(sbom.Options{Files: sbom.FilesAll})

	// Assert
	assert.NotEqual(t, all.SerialNumber, backend.SerialNumber)
	assert.Equal(t, backend.SerialNumber, again.SerialNumber)
	assert.NotEqual(t, all.SerialNumber, files.SerialNumber)
}

func TestLockStateIsReported(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig, sampleFiles)
	absent := f.build(sbom.Options{})
	f.lock(nil)
	inSync := f.build(sbom.Options{})
	f.write("rules/r1.md", "# edited\n")
	stale := f.build(sbom.Options{})

	// Assert
	assert.False(t, absent.LockPresent)
	assert.False(t, absent.LockInSync)
	assert.True(t, inSync.LockPresent)
	assert.True(t, inSync.LockInSync)
	assert.True(t, stale.LockPresent)
	assert.False(t, stale.LockInSync)
}

func TestPinning(t *testing.T) {
	tests := []struct {
		name, server string
		wantPinned   string
		wantCode     string
		wantPURL     string
	}{
		{"npm exact", "command = \"npx\"\nargs = [\"srv@1.2.3\"]\n", "true", "", "pkg:npm/srv@1.2.3"},
		{"npm prerelease", "command = \"npx\"\nargs = [\"srv@1.2.3-beta.1\"]\n", "true", "", "pkg:npm/srv@1.2.3-beta.1"},
		{"npm latest", "command = \"npx\"\nargs = [\"srv@latest\"]\n", "false", "AR750", "pkg:npm/srv"},
		{"npm range", "command = \"npx\"\nargs = [\"srv@^1.2.3\"]\n", "false", "AR750", "pkg:npm/srv"},
		{"npm major only", "command = \"npx\"\nargs = [\"srv@1\"]\n", "false", "AR750", "pkg:npm/srv"},
		{"npm no version", "command = \"npx\"\nargs = [\"srv\"]\n", "false", "AR750", "pkg:npm/srv"},
		{"pypi exact", "command = \"uvx\"\nargs = [\"srv==0.4.0\"]\n", "true", "", "pkg:pypi/srv@0.4.0"},
		{"pypi floating", "command = \"uvx\"\nargs = [\"srv\"]\n", "false", "AR750", "pkg:pypi/srv"},
		{"go exact", "command = \"go\"\nargs = [\"run\", \"example.com/x/srv@v1.2.3\"]\n", "true", "", "pkg:golang/example.com/x/srv@v1.2.3"},
		{"go latest", "command = \"go\"\nargs = [\"run\", \"example.com/x/srv@latest\"]\n", "false", "AR750", "pkg:golang/example.com/x/srv"},
		{"oci digest", "command = \"docker\"\nargs = [\"run\", \"mcp/fetch@sha256:abc123\"]\n", "true", "", "pkg:oci/fetch@sha256:abc123?repository_url=docker.io/mcp/fetch"},
		{"oci tag", "command = \"docker\"\nargs = [\"run\", \"mcp/fetch:1.0\"]\n", "false", "AR750", "pkg:oci/fetch?repository_url=docker.io/mcp/fetch&tag=1.0"},
		{"no coordinates", "command = \"/opt/srv\"\n", "", "AR751", ""},
		{"declared exact", "command = \"/opt/srv\"\npackage = \"pkg:npm/%40scope/srv@2.0.0\"\n", "true", "", "pkg:npm/%40scope/srv@2.0.0"},
		{"declared floating", "command = \"/opt/srv\"\npackage = \"pkg:pypi/srv\"\n", "false", "AR750", "pkg:pypi/srv"},
		{"declared overrides the launcher", "command = \"npx\"\nargs = [\"other@9.9.9\"]\npackage = \"pkg:npm/srv@1.0.0\"\n", "true", "", "pkg:npm/srv@1.0.0"},
		{"declared oci needs a digest", "command = \"/x\"\npackage = \"pkg:oci/img?repository_url=ghcr.io/o/img\"\n", "false", "AR750", "pkg:oci/img?repository_url=ghcr.io/o/img"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t, baseConfig+"\n[[mcp_servers]]\nname = \"srv\"\n"+tt.server, nil)

			// Act
			bom := f.build(sbom.Options{})

			// Assert
			comp := findComponent(bom, "ai-rulez:mcp:srv")
			require.NotNil(t, comp)
			assert.Equal(t, tt.wantPURL, comp.PURL)
			assert.Equal(t, tt.wantPinned, propValue(comp.Properties, "ai-rulez:pinned"))
			var codes []string
			for _, finding := range bom.Findings {
				codes = append(codes, finding.Code)
				assert.Equal(t, "mcp-server srv", finding.Subject)
			}
			if tt.wantCode == "" {
				assert.Empty(t, codes)
				return
			}
			assert.Equal(t, []string{tt.wantCode}, codes)
		})
	}
}

func TestUnpinnedPackageKeepsTheRequestedVersionOutOfThePurl(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+"\n[[mcp_servers]]\nname = \"srv\"\ncommand = \"npx\"\nargs = [\"srv@latest\"]\n", nil)

	// Act
	bom := f.build(sbom.Options{})

	// Assert
	comp := findComponent(bom, "ai-rulez:mcp:srv")
	assert.Empty(t, comp.Version)
	assert.NotContains(t, comp.PURL, "latest")
	assert.Equal(t, "latest", propValue(comp.Properties, "ai-rulez:requested-version"))
}

func TestInvalidDeclaredPackageIsIgnored(t *testing.T) {
	tests := []struct {
		name, pkg string
	}{
		{"not a purl", "left-pad"},
		{"credentials in a qualifier", "pkg:oci/img@sha256:abc?repository_url=https://user:tok3n@registry.example/img"},
		{"no name", "pkg:npm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t, baseConfig+"\n[[mcp_servers]]\nname = \"srv\"\ncommand = \"npx\"\nargs = [\"real@1.0.0\"]\npackage = "+quote(tt.pkg)+"\n", nil)

			// Act
			bom, doc := f.cdx(sbom.Options{})

			// Assert
			comp := findComponent(bom, "ai-rulez:mcp:srv")
			assert.Equal(t, "pkg:npm/real@1.0.0", comp.PURL, "falls back to the launcher")
			assert.NotContains(t, doc, "tok3n")
			require.NotEmpty(t, bom.Findings)
			assert.Equal(t, "AR751", bom.Findings[0].Code)
		})
	}
}

func TestSourcePinning(t *testing.T) {
	tests := []struct {
		name, ref  string
		pin        *lockfile.Entry
		wantPinned string
		wantFound  bool
	}{
		{"branch, no lock", "main", nil, "false", true},
		{"commit ref", "0123456789abcdef0123456789abcdef01234567", nil, "true", false},
		{"locked commit", "main", &lockfile.Entry{Name: "shared", Commit: "0123456789abcdef0123456789abcdef01234567", Digest: "sha256:abc"}, "true", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t, baseConfig+"\n[[includes]]\nname = \"shared\"\nsource = \"https://github.com/Org/Rules.git\"\nref = \""+tt.ref+"\"\n", nil)
			if tt.pin != nil {
				f.lock(func(l *lockfile.File) { l.Include = append(l.Include, *tt.pin) })
			}

			// Act
			bom := f.build(sbom.Options{})

			// Assert
			comp := findComponent(bom, "ai-rulez:source:include:shared")
			require.NotNil(t, comp)
			assert.Equal(t, tt.wantPinned, propValue(comp.Properties, "ai-rulez:pinned"))
			assert.Equal(t, tt.wantFound, len(bom.Findings) == 1 && bom.Findings[0].Code == "AR750", "%v", bom.Findings)
		})
	}
}

func TestDeclaredPackageNeverReachesHarnessOutputs(t *testing.T) {
	// Arrange: the field is for the SBOM only.
	f := newFixture(t, baseConfig+"\n[[mcp_servers]]\nname = \"srv\"\ncommand = \"npx\"\nargs = [\"srv\"]\npackage = \"pkg:npm/srv@1.0.0\"\n", nil)
	cfg := f.config()

	// Act
	_, err := generator.NewGenerator(cfg).GenerateFiles("")

	// Assert
	require.NoError(t, err)
	written, err := os.ReadFile(filepath.Join(f.dir, ".mcp.json"))
	require.NoError(t, err)
	assert.Contains(t, string(written), `"srv"`, "the server itself is generated")
	assert.NotContains(t, string(written), "pkg:npm")
	assert.NotContains(t, string(written), `"package"`)
}

func TestIncludeOutputs(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig, sampleFiles)

	// Act
	without := f.build(sbom.Options{})
	bom, doc := f.cdx(sbom.Options{IncludeOutputs: true})
	spdx, raw := f.spdx(sbom.Options{IncludeOutputs: true})

	// Assert
	for _, c := range without.Components {
		assert.NotEqual(t, "output", propValue(c.Properties, "ai-rulez:kind"))
	}
	var outputs []sbom.Component
	for _, c := range bom.Components {
		if propValue(c.Properties, "ai-rulez:kind") == "output" {
			outputs = append(outputs, c)
		}
	}
	require.NotEmpty(t, outputs)
	for _, c := range outputs {
		assert.Equal(t, "file", c.Type)
		assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, propValue(c.Properties, "ai-rulez:output-digest"))
		assert.Empty(t, c.Hashes, "an output pin is a header-free rendering, not a plain hash")
	}
	assert.Equal(t, "included", propValue(bom.Metadata.Component.Properties, "ai-rulez:outputs"))
	requireValid(t, schemaFor(t), doc)
	requireValid(t, spdxSchema(t), raw)
	assert.Greater(t, len(spdx.Packages), len(without.ToSPDX().Packages))
}

func TestApprovalProperties(t *testing.T) {
	// Arrange
	cfg := baseConfig + "\n[governance]\nrequire_approval = [\"kind:skill\"]\nmin_approvers = 1\n"
	files := map[string]string{
		"skills/ok/SKILL.md":      "---\nname: ok\ndescription: d\n---\nbody\n",
		"skills/stale/SKILL.md":   "---\nname: stale\ndescription: d\n---\nbody\n",
		"skills/missing/SKILL.md": "---\nname: missing\ndescription: d\n---\nbody\n",
		"rules/r.md":              "# r\n",
	}
	f := newFixture(t, cfg, files)
	f.lock(func(l *lockfile.File) {
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "ok", Digest: f.digestOf("skill", "ok"), Reviewer: "Alice@Example.org", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-03-04T05:06:07Z", Expires: "2999-01-01"})
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "stale", Digest: "sha256:" + strings.Repeat("0", 64), Reviewer: "bob@example.org", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-03-04T05:06:07Z"})
	})

	// Act
	bom := f.build(sbom.Options{Now: mustTime(t, "2026-06-01T00:00:00Z")})

	// Assert
	get := func(ref, name string) string { return propValue(findComponent(bom, ref).Properties, "ai-rulez:"+name) }
	assert.Equal(t, "approved", get("ai-rulez:item:skill::ok", "approval"))
	assert.Equal(t, "alice@example.org", get("ai-rulez:item:skill::ok", "approvers"))
	assert.Equal(t, "asserted", get("ai-rulez:item:skill::ok", "approval-assurance"))
	assert.Equal(t, "2999-01-01", get("ai-rulez:item:skill::ok", "approval-expires"))
	assert.Equal(t, "stale", get("ai-rulez:item:skill::stale", "approval"))
	assert.Empty(t, get("ai-rulez:item:skill::stale", "approvers"), "a stale approval names nobody")
	assert.Equal(t, "missing", get("ai-rulez:item:skill::missing", "approval"))
	assert.Equal(t, "not-required", get("ai-rulez:item:rule::r", "approval"))
}

func TestApprovalPropertiesAreAbsentWithoutGovernanceOrRecords(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig, sampleFiles)
	f.lock(nil)

	// Act
	bom := f.build(sbom.Options{})

	// Assert
	for _, c := range bom.Components {
		assert.Empty(t, propValue(c.Properties, "ai-rulez:approval"), c.BOMRef)
	}
}

func TestExpiredApprovalIsReported(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+"\n[governance]\nrequire_approval = [\"kind:skill\"]\n", map[string]string{"skills/s/SKILL.md": "---\nname: s\ndescription: d\n---\nbody\n"})
	f.lock(func(l *lockfile.File) {
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "s", Digest: f.digestOf("skill", "s"), Reviewer: "a@x", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-01-01T00:00:00Z", Expires: "2026-02-01"})
	})

	// Act
	bom := f.build(sbom.Options{Now: mustTime(t, "2026-06-01T00:00:00Z")})

	// Assert
	assert.Equal(t, "expired", propValue(findComponent(bom, "ai-rulez:item:skill::s").Properties, "ai-rulez:approval"))
}

func TestReviewerRedaction(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+"\n[governance]\nrequire_approval = [\"kind:skill\"]\n", map[string]string{"skills/s/SKILL.md": "---\nname: s\ndescription: d\n---\nbody\n"})
	f.lock(func(l *lockfile.File) {
		for _, who := range []string{"alice@example.org", "bob@example.org"} {
			l.SetApproval(lockfile.Approval{Kind: "skill", ID: "s", Digest: f.digestOf("skill", "s"), Reviewer: who, Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-03-04T05:06:07Z"})
		}
	})

	// Act
	plain, plainCDX := f.cdx(sbom.Options{})
	redacted, redactedCDX := f.cdx(sbom.Options{RedactReviewers: true})
	_, redactedSPDX := f.spdx(sbom.Options{RedactReviewers: true})
	_, plainSPDX := f.spdx(sbom.Options{})
	none, noneCDX := f.cdx(sbom.Options{NoApprovals: true})

	// Assert
	assert.Equal(t, "alice@example.org,bob@example.org", propValue(findComponent(plain, "ai-rulez:item:skill::s").Properties, "ai-rulez:approvers"))
	assert.Contains(t, plainCDX, "alice@example.org")
	assert.Contains(t, plainSPDX, "Person: alice@example.org")
	for _, doc := range []string{redactedCDX, redactedSPDX} {
		assert.NotContains(t, doc, "alice")
		assert.NotContains(t, doc, "bob")
		assert.NotContains(t, doc, "example.org")
	}
	approvers := propValue(findComponent(redacted, "ai-rulez:item:skill::s").Properties, "ai-rulez:approvers")
	assert.Regexp(t, `^reviewer-[0-9a-f]{8},reviewer-[0-9a-f]{8}$`, approvers)
	assert.Equal(t, "approved", propValue(findComponent(redacted, "ai-rulez:item:skill::s").Properties, "ai-rulez:approval"), "the status stays")
	again, _ := f.cdx(sbom.Options{RedactReviewers: true})
	assert.Equal(t, approvers, propValue(findComponent(again, "ai-rulez:item:skill::s").Properties, "ai-rulez:approvers"), "the salted hash is stable")
	assert.Empty(t, propValue(findComponent(none, "ai-rulez:item:skill::s").Properties, "ai-rulez:approval"))
	assert.NotContains(t, noneCDX, "approval")
}

func TestSPDXReviewAnnotations(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+"\n[governance]\nrequire_approval = [\"kind:skill\"]\n", map[string]string{"skills/s/SKILL.md": "---\nname: s\ndescription: d\n---\nbody\n"})
	digest := f.digestOf("skill", "s")
	f.lock(func(l *lockfile.File) {
		l.SetApproval(lockfile.Approval{Kind: "skill", ID: "s", Digest: digest, Reviewer: "alice@example.org", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-03-04T05:06:07+02:00"})
	})

	// Act
	doc, raw := f.spdx(sbom.Options{})

	// Assert
	requireValid(t, spdxSchema(t), raw)
	for _, p := range doc.Packages {
		if p.Name != "skill/s" {
			continue
		}
		require.Len(t, p.Annotations, 1)
		assert.Equal(t, sbom.Annotation{
			Annotator: "Person: alice@example.org", Date: "2026-03-04T03:06:07Z", Type: "REVIEW",
			Comment: "approved " + digest + " (assurance asserted)",
		}, p.Annotations[0])
		return
	}
	t.Fatal("skill/s package not found")
}

func TestSignatureStatus(t *testing.T) {
	tests := []struct {
		name string
		sig  *sbom.Signature
		want map[string]string
	}{
		{"verified", &sbom.Signature{Status: sbom.SignatureVerified, Signer: "me@example.org", Issuer: "https://issuer.example", SignedAt: mustTime(t, "2026-05-06T07:08:09Z")},
			map[string]string{"ai-rulez:signature": "verified", "ai-rulez:signer": "me@example.org", "ai-rulez:signer-issuer": "https://issuer.example", "ai-rulez:signed-at": "2026-05-06T07:08:09Z"}},
		{"absent", &sbom.Signature{Status: sbom.SignatureAbsent}, map[string]string{"ai-rulez:signature": "absent"}},
		{"invalid", &sbom.Signature{Status: sbom.SignatureInvalid, Code: "AR721"}, map[string]string{"ai-rulez:signature": "invalid", "ai-rulez:signature-code": "AR721"}},
		{"not asked", nil, map[string]string{"ai-rulez:signature": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFixture(t, baseConfig, nil)

			// Act
			bom, doc := f.cdx(sbom.Options{Signature: tt.sig})
			_, raw := f.spdx(sbom.Options{Signature: tt.sig})

			// Assert
			for name, want := range tt.want {
				assert.Equal(t, want, propValue(bom.Metadata.Component.Properties, name), name)
			}
			requireValid(t, schemaFor(t), doc)
			requireValid(t, spdxSchema(t), raw)
			if tt.sig != nil {
				assert.Contains(t, raw, "lock attestation "+tt.sig.Status)
			}
		})
	}
}

func TestTimestamp(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig, nil)

	// Act
	without := f.build(sbom.Options{})
	with := f.build(sbom.Options{Timestamp: mustTime(t, "2026-05-06T09:08:09+02:00")})

	// Assert
	assert.Empty(t, without.Metadata.Timestamp)
	assert.Equal(t, "2026-05-06T07:08:09Z", with.Metadata.Timestamp)
	assert.Equal(t, without.SerialNumber, with.SerialNumber, "the time never changes the identity")
}

func TestDependencyGraph(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+"\n[[roles]]\nname = \"all\"\n", map[string]string{
		"skills/base/SKILL.md":  "---\nname: base\ndescription: d\n---\nbody\n",
		"skills/addon/SKILL.md": "---\nname: addon\ndescription: d\nskills: [base, missing]\n---\nbody\n",
		"agents/helper.md":      "---\nname: helper\ndescription: d\nskills: [addon]\n---\nbody\n",
		"rules/r.md":            "# r\n",
	})

	// Act
	bom, doc := f.cdx(sbom.Options{})

	// Assert
	deps := map[string][]string{}
	for _, d := range bom.Dependencies {
		deps[d.Ref] = d.DependsOn
	}
	assert.Contains(t, deps["ai-rulez:project"], "ai-rulez:item:skill::base")
	assert.Equal(t, []string{"ai-rulez:item:skill::base"}, deps["ai-rulez:item:skill::addon"])
	assert.Equal(t, []string{"ai-rulez:item:skill::addon"}, deps["ai-rulez:item:agent::helper"])
	assert.Equal(t, []string{"ai-rulez:item:agent::helper", "ai-rulez:item:rule::r", "ai-rulez:item:skill::addon", "ai-rulez:item:skill::base"}, deps["ai-rulez:item:role::all"])
	known := map[string]bool{"ai-rulez:project": true}
	for _, ref := range refsOf(bom) {
		known[ref] = true
	}
	for ref, on := range deps {
		assert.True(t, known[ref], ref)
		for _, target := range on {
			assert.True(t, known[target], "%s depends on unknown %s", ref, target)
		}
	}
	requireValid(t, schemaFor(t), doc)
	spdx, _ := f.spdx(sbom.Options{})
	var dependsOn int
	for _, r := range spdx.Relationships {
		if r.Type == "DEPENDS_ON" {
			dependsOn++
		}
	}
	assert.Equal(t, 6, dependsOn, "addon->base, helper->addon and the role's four items")
}

func TestLockScopeSkillsIsStated(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig+"\n[lock]\nscope = \"skills\"\n", richFiles())

	// Act
	bom := f.build(sbom.Options{})

	// Assert
	assert.Equal(t, "skills", propValue(bom.Metadata.Component.Properties, "ai-rulez:lock-scope"))
	assert.NotNil(t, findComponent(bom, "ai-rulez:item:skill::s1"))
	assert.Nil(t, findComponent(bom, "ai-rulez:item:rule::r1"), "only the pinned kinds are listed")
}

func TestDrift(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig, sampleFiles)
	_, base := f.cdx(sbom.Options{})
	_, baseSPDX := f.spdx(sbom.Options{})
	f.write("rules/r2.md", "# new\n")
	f.write("rules/r1.md", "# edited\n")
	_, edited := f.cdx(sbom.Options{})
	_, editedSPDX := f.spdx(sbom.Options{})
	oldTool := strings.ReplaceAll(base, `"version": "9.9.9"`, `"version": "1.0.0"`)
	oldToolSPDX := strings.ReplaceAll(baseSPDX, "ai-rulez-9.9.9", "ai-rulez-1.0.0")

	tests := []struct {
		name           string
		committed, now string
		want           []string
		wantNone       bool
	}{
		{"identical", base, base, nil, true},
		{"another tool version only", oldTool, base, nil, true},
		{"another tool version only, spdx", oldToolSPDX, baseSPDX, nil, true},
		{"cyclonedx: added and changed", base, edited, []string{"components: added ai-rulez:item:rule::r2", "components: changed ai-rulez:item:rule::r1", "serialNumber changed"}, false},
		{"spdx: added and changed", baseSPDX, editedSPDX, []string{"documentNamespace changed", "packages: added SPDXRef-ai-rulez-item-rule--r2-"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			diffs, err := sbom.Drift([]byte(tt.committed), []byte(tt.now))

			// Assert
			require.NoError(t, err)
			if tt.wantNone {
				assert.Empty(t, diffs)
				return
			}
			joined := strings.Join(diffs, "\n")
			for _, want := range tt.want {
				assert.Contains(t, joined, want)
			}
		})
	}
}

func TestDriftRejectsInvalidJSON(t *testing.T) {
	// Act
	_, err := sbom.Drift([]byte("not json"), []byte("{}"))

	// Assert
	require.Error(t, err)
}

func TestNormalizeFormat(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"", sbom.FormatCycloneDX, false},
		{"cyclonedx", sbom.FormatCycloneDX, false},
		{"CycloneDX", sbom.FormatCycloneDX, false},
		{"spdx-json", sbom.FormatSPDXJSON, false},
		{"spdx", sbom.FormatSPDXJSON, false},
		{"xml", "", true},
	}
	for _, tt := range tests {
		got, err := sbom.NormalizeFormat(tt.in)
		if tt.wantErr {
			require.Error(t, err, tt.in)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, tt.want, got)
	}
}

func TestOptionsAreValidated(t *testing.T) {
	// Arrange
	f := newFixture(t, baseConfig, nil)

	// Act
	_, err := sbom.Build(f.config(), "9.9.9", sbom.Options{Files: "some"})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown --files "some"`)
}

func TestNoSecretReachesEitherFormat(t *testing.T) {
	// Arrange
	f := richFixture(t, baseConfig+mcpConfig+"\n[[mcp_servers]]\nname = \"decl\"\ncommand = \"/x\"\npackage = \"pkg:oci/img@sha256:abc?repository_url=https://user:s3cret@r.example/img\"\n")

	// Act
	_, cdx := f.cdx(sbom.Options{Files: sbom.FilesAll, IncludeOutputs: true})
	_, spdx := f.spdx(sbom.Options{Files: sbom.FilesAll, IncludeOutputs: true})

	// Assert
	for _, doc := range []string{cdx, spdx} {
		for _, secret := range []string{"env-secret-value", "url-secret", "query-secret", "header-secret", "s3cret"} {
			assert.NotContains(t, doc, secret)
		}
	}
}
