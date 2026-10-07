package config

// Harnesses whose hooks are code, not a settings file: each loads a plugin or
// extension module, so [[hooks]] are rendered into a generated JavaScript or
// TypeScript file (internal/generator/hookplugins) instead of a document merged
// into the tool's settings. They are valid in HookGroup.Targets and
// HookGroup.Matchers like the harnesses of HookHarnesses.
const (
	HarnessOpencode = "opencode"
	HarnessKilo     = "kilo"
	HarnessMimocode = "mimocode"
	HarnessPi       = "pi"
	HarnessAmp      = "amp"
)

// HookPluginHarnesses lists the plugin-based harnesses, in a stable order.
var HookPluginHarnesses = []string{HarnessOpencode, HarnessKilo, HarnessMimocode, HarnessPi, HarnessAmp}

// withPluginHarnesses appends the plugin-based harnesses the list lacks, so the
// list stays independent of the declaration of HookHarnesses, which other hook
// renderers extend.
func withPluginHarnesses(list []string) []string {
	for _, harness := range HookPluginHarnesses {
		list = appendMissing(list, harness)
	}
	return list
}

func appendMissing(list []string, name string) []string {
	for _, item := range list {
		if item == name {
			return list
		}
	}
	return append(list, name)
}
