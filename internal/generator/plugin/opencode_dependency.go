package plugin

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

const npmDependenciesKey = "dependencies"

// reconcileOpenCodeDependency prunes a former automatic SDK only when the old
// sidecar proves that the generated metadata has not been edited. Disabling the
// automatic dependency also relinquishes its ownership for subsequent runs.
func (m *Manifest) reconcileOpenCodeDependency(baseDir string, data []byte, pkg map[string]any) error {
	dependencies, ok := pkg[npmDependenciesKey].(map[string]any)
	if m.addsOpenCodeDependency() || !slices.Contains(m.Runtimes, config.PluginRuntimeOpenCode) || !ok {
		return nil
	}
	if _, exists := dependencies["@opencode/plugin"]; !exists {
		return nil
	}
	previousData, err := os.ReadFile(filepath.Join(baseDir, provenanceFileName))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return oops.Wrapf(err, "read prior OpenCode dependency ownership")
	}
	var previous provenanceDocument
	if err := unmarshalProvenance(previousData, &previous); err != nil {
		return err
	}
	if previous.SchemaVersion != provenanceSchema {
		return oops.Errorf("unsupported plugin provenance schema %q", previous.SchemaVersion)
	}
	keys := previous.Managed["package.json"]
	if slices.Contains(keys, npmDependenciesKey) &&
		hashBytes(managedSubdocument(data, keys)) == previous.Outputs["package.json"].ContentHash {
		delete(dependencies, "@opencode/plugin")
	}
	return nil
}
