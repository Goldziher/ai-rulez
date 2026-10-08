package conformance

import (
	"testing"
)

const sbomProject = `version = "5.0"
name = "conf"
presets = ["claude"]

[[mcp_servers]]
name = "tools"
command = "npx"
args = ["-y", "@acme/mcp"]

[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://mcp.acme.example/mcp"
`

func sbomFiles() map[string]string {
	return map[string]string{
		".ai-rulez/config.toml":            sbomProject,
		".ai-rulez/rules/r.md":             "# Rule\n\nBe nice.\n",
		".ai-rulez/context/c.md":           "context\n",
		".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Ship it.\nlicense: Apache-2.0\n---\nDeploy.\n",
	}
}

func TestCycloneDXOutputConformsToTheOfficial16Schema(t *testing.T) {
	// Arrange
	schema := compile(t, "http://cyclonedx.org/schema/bom-1.6.schema.json", map[string]string{
		"http://cyclonedx.org/schema/spdx.schema.json":     "cyclonedx/1.6/spdx.schema.json",
		"http://cyclonedx.org/schema/jsf-0.82.schema.json": "cyclonedx/1.6/jsf-0.82.schema.json",
		"http://cyclonedx.org/schema/bom-1.6.schema.json":  "cyclonedx/1.6/bom-1.6.schema.json",
	})
	dir := project(t, sbomFiles())

	// Act
	doc := run(t, dir, "sbom", "--files", "all")

	// Assert
	requireValid(t, schema, doc)
	requireInvalid(t, schema, `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[{"type":"nonsense","name":"x"}]}`)
}

func TestSPDXOutputConformsToTheOfficial23Schema(t *testing.T) {
	// Arrange
	const id = "http://spdx.org/rdf/terms/2.3"
	schema := compile(t, id, map[string]string{id: "spdx/2.3/spdx-2.3.schema.json"})
	dir := project(t, sbomFiles())

	// Act
	doc := run(t, dir, "sbom", "--format", "spdx-json", "--files", "all")

	// Assert
	requireValid(t, schema, doc)
	requireInvalid(t, schema, `{"spdxVersion":"SPDX-2.3","packages":[{"name":1}]}`)
}
