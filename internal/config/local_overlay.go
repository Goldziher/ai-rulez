package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

// LocalOverlay records the machine-local config.local.* document merged into a
// loaded Config. A Config carrying one is a merged view and must never be
// written back as the shared configuration.
type LocalOverlay struct {
	// Path is the absolute path of the overlay file.
	Path string
	// Format is "toml", "yaml" or "json".
	Format string
	// Doc is the normalized overlay document as decoded from disk. Treat it as
	// read-only.
	Doc map[string]any
}

// HasLocalInputs reports whether any machine-local input (overlay file or
// local content) contributed to this configuration.
func (c *Config) HasLocalInputs() bool {
	if c == nil {
		return false
	}
	return c.LocalOverlay != nil || (c.LocalContent != nil && !c.LocalContent.IsEmpty())
}

const (
	extTOML = ".toml"
	extYAML = ".yaml"
	extYML  = ".yml"
	extJSON = ".json"
)

const (
	formatYAML = "yaml"
	formatJSON = "json"
)

var localConfigFormatByExt = map[string]string{
	extTOML: "toml",
	extYAML: formatYAML,
	extYML:  formatYAML,
	extJSON: formatJSON,
}

func localConfigNames() []string {
	return []string{
		localVariantName(configTOMLFilename),
		localVariantName(configYAMLFilename),
		localVariantName(configYMLFilename),
		localVariantName(configJSONFilename),
	}
}

// isLocalConfigFilename reports whether base is a config.local.* overlay name.
func isLocalConfigFilename(base string) bool {
	for _, n := range localConfigNames() {
		if base == n {
			return true
		}
	}
	return false
}

// findLocalConfigFile probes configDir for a config.local.* overlay. It
// returns "" when none exists, and an error when several do. A format that
// differs from the main config's only warns.
func findLocalConfigFile(configDir, mainConfigFile string) (path string, err error) {
	var found []string
	for _, name := range localConfigNames() {
		info, statErr := os.Stat(filepath.Join(configDir, name))
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return "", oops.With("path", filepath.Join(configDir, name)).Wrapf(statErr, "stat local config")
		}
		if !info.IsDir() {
			found = append(found, name)
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
	default:
		return "", oops.
			With("config_dir", configDir).
			Hint("Keep a single config.local.* file and delete the others").
			Errorf("multiple local config files found: %s", strings.Join(found, ", "))
	}
	if localConfigFormatByExt[filepath.Ext(found[0])] != localConfigFormatByExt[filepath.Ext(mainConfigFile)] {
		logger.Debug("Local config format differs from the main config", "local", found[0], "main", mainConfigFile)
	}
	return filepath.Join(configDir, found[0]), nil
}

// decodeConfigDoc decodes config bytes into a generic document using the
// native library for the file's format.
func decodeConfigDoc(path string, data []byte) (map[string]any, error) {
	doc := map[string]any{}
	var err error
	switch localConfigFormatByExt[strings.ToLower(filepath.Ext(path))] {
	case "toml":
		err = toml.Unmarshal(data, &doc)
	case formatYAML:
		doc, err = decodeYAMLDoc(data)
	case formatJSON:
		err = json.Unmarshal(data, &doc)
	default:
		return nil, oops.With("path", path).Errorf("unsupported config format: %s", filepath.Base(path))
	}
	if err != nil {
		return nil, oops.With("path", path).Hint("Check the file syntax").Wrapf(err, "parse %s", filepath.Base(path))
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func readConfigDoc(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // config path chosen by the user
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read %s", filepath.Base(path))
	}
	return decodeConfigDoc(path, data)
}

// withLocalOverlay applies a config.local.* overlay found beside the main
// config, unless disabled. Without an overlay cfg is returned untouched.
func withLocalOverlay(cfg *Config, mainPath, configDir string, lo loadOptions) (*Config, error) {
	if lo.withoutLocal {
		return cfg, nil
	}
	localPath, err := findLocalConfigFile(configDir, filepath.Base(mainPath))
	if err != nil || localPath == "" {
		return cfg, err
	}

	mainDoc, err := readConfigDoc(mainPath)
	if err != nil {
		return nil, err
	}
	localDoc, err := readConfigDoc(localPath)
	if err != nil {
		return nil, err
	}
	merged, warnings, err := MergeConfigDocs(mainDoc, localDoc)
	if err != nil {
		return nil, oops.With("path", localPath).Wrapf(err, "apply local overlay %s", filepath.Base(localPath))
	}
	for _, w := range warnings {
		logger.Warn(w, "path", localPath)
	}

	out, err := decodeMergedDoc(mainPath, localPath, merged)
	if err != nil {
		return nil, err
	}
	out.ConfigFile = cfg.ConfigFile
	out.LocalOverlay = &LocalOverlay{
		Path:   localPath,
		Format: localConfigFormatByExt[filepath.Ext(localPath)],
		Doc:    normalizeConfigDocKeys(localDoc),
	}
	return out, nil
}

// decodeMergedDoc decodes a merged document into a Config. YAML mains go
// through yaml.Marshal so scalars decode exactly as in a native YAML load;
// TOML and JSON mains go through JSON.
func decodeMergedDoc(mainPath, localPath string, merged map[string]any) (*Config, error) {
	var (
		cfg *Config
		err error
	)
	doc := DocForJSON(merged) // "$schema" is the yaml and json tag
	if localConfigFormatByExt[strings.ToLower(filepath.Ext(mainPath))] == formatYAML {
		var data []byte
		if data, err = yaml.Marshal(doc); err == nil {
			cfg, err = decodeConfigYAML(data, mainPath)
		}
	} else {
		cfg, err = decodeJSONDoc(doc, mainPath)
	}
	if err != nil {
		return nil, oops.
			With("path", localPath).
			Hint("Check the value types in the local overlay").
			Wrapf(SanitizeDecodeError(err), "decode %s merged with %s", filepath.Base(mainPath), filepath.Base(localPath))
	}
	return cfg, nil
}

// quotedValueRe matches the backtick-quoted source value that yaml decode errors
// carry ("cannot unmarshal !!str `value` into []string").
var quotedValueRe = regexp.MustCompile("\\s*`[^`]*`")

// SanitizeDecodeError returns an error with the offending source values removed
// from a decode failure, keeping line numbers and types. The merged document
// includes machine-local values that may be credentials, so they must not reach
// terminals, logs or MCP responses.
func SanitizeDecodeError(err error) error {
	return errors.New(quotedValueRe.ReplaceAllString(err.Error(), ""))
}

// decodeJSONDoc decodes a document through JSON. YAML numbers are first emitted
// as numbers (for int fields); if that fails on a string field such as an env
// value, they are retried as strings.
func decodeJSONDoc(doc map[string]any, path string) (*Config, error) {
	var firstErr error
	for _, d := range []map[string]any{doc, stringifyRawScalars(doc).(map[string]any)} { //nolint:errcheck // map in, map out
		data, err := json.Marshal(d)
		if err == nil {
			var cfg *Config
			if cfg, err = decodeConfigJSON(data, path); err == nil {
				return cfg, nil
			}
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

// stringifyRawScalars returns a deep copy with YAML numbers kept as text turned
// into plain strings.
func stringifyRawScalars(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = stringifyRawScalars(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = stringifyRawScalars(e)
		}
		return out
	case rawScalar:
		return string(t)
	}
	return v
}

// rawScalar keeps the source text of a YAML number so a value such as
// `version: 4.0` or an integer env value decodes into a string field exactly
// as it does in a native YAML load.
type rawScalar string

// MarshalYAML emits the original plain scalar.
func (r rawScalar) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: string(r)}, nil
}

// MarshalJSON emits the scalar as a JSON number when it is one.
func (r rawScalar) MarshalJSON() ([]byte, error) {
	if json.Valid([]byte(r)) && !strings.HasPrefix(string(r), "\"") {
		return []byte(r), nil
	}
	return json.Marshal(string(r))
}

// decodeYAMLDoc decodes YAML into a generic document, preserving the text of
// numeric scalars (see rawScalar).
func decodeYAMLDoc(data []byte) (map[string]any, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err //nolint:wrapcheck // wrapped by decodeConfigDoc
	}
	if root.Kind == 0 {
		return map[string]any{}, nil
	}
	v, err := yamlNodeValue(&root)
	if err != nil {
		return nil, err
	}
	doc, ok := v.(map[string]any)
	if !ok {
		if v == nil {
			return map[string]any{}, nil
		}
		return nil, oops.Errorf("top level of the document must be a mapping")
	}
	return doc, nil
}

func yamlNodeValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return yamlNodeValue(n.Content[0])
	case yaml.AliasNode:
		return yamlNodeValue(n.Alias)
	case yaml.SequenceNode:
		out := make([]any, len(n.Content))
		for i, c := range n.Content {
			v, err := yamlNodeValue(c)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case yaml.MappingNode:
		return yamlMappingValue(n)
	case yaml.ScalarNode:
		if n.Tag == "!!int" || n.Tag == "!!float" {
			return rawScalar(n.Value), nil
		}
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err //nolint:wrapcheck // wrapped by decodeConfigDoc
	}
	return v, nil
}

func yamlMappingValue(n *yaml.Node) (any, error) {
	out := make(map[string]any, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		if k.Tag == "!!merge" { // merge keys: let yaml resolve the whole mapping
			var m map[string]any
			if err := n.Decode(&m); err != nil {
				return nil, err //nolint:wrapcheck // wrapped by decodeConfigDoc
			}
			return m, nil
		}
		v, err := yamlNodeValue(n.Content[i+1])
		if err != nil {
			return nil, err
		}
		out[k.Value] = v
	}
	return out, nil
}

// ReadLocalOverlay reads the config.local.* overlay beside the main config in
// configDir without loading the rest of the configuration. It returns nil when
// there is none.
func ReadLocalOverlay(configDir, mainConfigFile string) (*LocalOverlay, error) {
	localPath, err := findLocalConfigFile(configDir, mainConfigFile)
	if err != nil || localPath == "" {
		return nil, err
	}
	doc, err := readConfigDoc(localPath)
	if err != nil {
		return nil, err
	}
	return &LocalOverlay{
		Path:   localPath,
		Format: localConfigFormatByExt[filepath.Ext(localPath)],
		Doc:    normalizeConfigDocKeys(doc),
	}, nil
}

// KeyPaths lists the dotted key paths the overlay sets, sorted, without any
// values (overlay files may hold secrets). Named list entries are addressed by
// name, e.g. "mcp_servers.github.env.TOKEN".
func (o *LocalOverlay) KeyPaths() []string {
	if o == nil {
		return nil
	}
	var out []string
	walkLeaves(nil, o.Doc, func(segs []string, _ any) {
		out = append(out, strings.Join(segs, "."))
	})
	sort.Strings(out)
	return out
}

// errMergedConfigWrite is returned when a merged configuration is saved.
func errMergedConfigWrite() error {
	return oops.
		Hint("Load with config.WithoutLocal() to modify the shared config").
		Errorf("refusing to write a configuration merged with a local overlay")
}
