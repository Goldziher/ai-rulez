package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// vendored pins every schema under schemas/. mirror is the in-package copy the
// product embeds, which must stay byte-identical; empty when there is none.
// Updating a schema is a reviewed change: see README.md.
var vendored = []struct{ path, sha256, mirror string }{
	{"agent-plugins/1.0.0/plugin.schema.json", "0a4aad95ce337878ad38802ebf0daa3fde76abe3f65400c86bcbb1ec0b3ab883", "internal/agentplugins/schemas/1.0.0/plugin.schema.json"},
	{"agent-plugins/1.0.0/mcp.schema.json", "6539175bfcdf43085855183e86da40ea94b166547a72b47ae9a0a390516d3acb", "internal/agentplugins/schemas/1.0.0/mcp.schema.json"},
	{"agent-plugins/1.1.0/plugin.schema.json", "fdc7bb3962c48c9d2d561641d2bc96225c94ca69c4087010241b9423a290370f", "internal/agentplugins/schemas/1.1.0/plugin.schema.json"},
	{"agent-plugins/1.1.0/mcp.schema.json", "f227ec2c0e40cd23051bd7a6ba1f64789eff7773d4e481d80002ab9fd3c45137", "internal/agentplugins/schemas/1.1.0/mcp.schema.json"},
	{"ard/ard-entry.schema.json", "011b86d55fd5d2883dffae3f0577d26f5efb56ca866eb079edbc78a628f95499", "internal/ard/schema/ard-entry.schema.json"},
	{"cyclonedx/1.6/bom-1.6.schema.json", "18f57f7482593bad9f21b4feed09084640cbeff419d62ad5090c5ceccca5b37d", "internal/sbom/testdata/bom-1.6.schema.json"},
	{"cyclonedx/1.6/jsf-0.82.schema.json", "8bae002c25e723db7ee1f26afde680ae1a2b1a8f6b4b4b0fd65dc3becb090aae", "internal/sbom/testdata/jsf-0.82.schema.json"},
	{"cyclonedx/1.6/spdx.schema.json", "4b345e2329f209f34e960ae2a8e7cb46a166907e6a45e94978565925dc47b359", "internal/sbom/testdata/spdx.schema.json"},
	{"spdx/2.3/spdx-2.3.schema.json", "4126dc29f15e92feec92e9622f4939131ad58b125d6f578aa93e980a6e6f3212", "internal/sbom/testdata/spdx-2.3.schema.json"},
	{"in-toto/statement-v1.schema.json", "", ""},
	{"dsse/envelope.schema.json", "", ""},
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestVendoredSchemasArePinned(t *testing.T) {
	for _, v := range vendored {
		t.Run(v.path, func(t *testing.T) {
			// Act
			data := schemaFile(t, v.path)

			// Assert
			if v.sha256 != "" {
				assert.Equal(t, v.sha256, digest(data))
			}
			if v.mirror != "" {
				assert.Equal(t, data, repoFile(t, v.mirror), "the product's embedded copy drifted from the conformance copy")
			}
		})
	}
}
