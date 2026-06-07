package providers

import (
	"embed"
	"fmt"
)

//go:embed builtin/*.toml
var builtinFS embed.FS

// BuiltinNames returns the list of provider names embedded in the binary.
// One entry per *.toml file under builtin/.
func BuiltinNames() ([]string, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, fmt.Errorf("read embedded builtin dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		const ext = ".toml"
		if len(name) <= len(ext) || name[len(name)-len(ext):] != ext {
			continue
		}
		names = append(names, name[:len(name)-len(ext)])
	}
	return names, nil
}

// LoadBuiltin parses and validates the embedded provider spec for the given
// name. Returns a ready-to-use Generator.
func LoadBuiltin(name string) (*Generator, error) {
	path := "builtin/" + name + ".toml"
	raw, err := builtinFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read embedded provider %q: %w", name, err)
	}
	spec, err := LoadProviderSpec(raw, path, FormatTOML)
	if err != nil {
		return nil, fmt.Errorf("load embedded provider %q: %w", name, err)
	}
	return New(spec), nil
}
