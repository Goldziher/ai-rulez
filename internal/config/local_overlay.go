package config

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// LocalOverlay records the machine-local config.local.* document merged into a
// loaded Config. A Config carrying one is a merged view and must never be
// written back as the shared configuration.
type LocalOverlay struct {
	// Path is the absolute path of the overlay file.
	Path string
	// Doc is the normalized overlay document as decoded from disk. Treat it as
	// read-only.
	Doc map[string]any
	// Tracked is true when git tracks the overlay file. A committed overlay came
	// from the repository, not from this machine, so it gets no machine-local trust.
	Tracked bool
}

// HasLocalInputs reports whether any machine-local input (overlay file or
// local content) contributed to this configuration.
func (c *Config) HasLocalInputs() bool {
	if c == nil {
		return false
	}
	return c.LocalOverlay != nil || (c.LocalContent != nil && !c.LocalContent.IsEmpty())
}

func localConfigNames() []string {
	return []string{localVariantName(configTOMLFilename)}
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

// findLocalConfigFile probes configDir for config.local.toml. It returns "" when
// there is none, and an error for a V3 config.local.yaml, .yml or .json, which
// v5 no longer reads (ignoring it would silently drop the overrides).
func findLocalConfigFile(v workspace.View, configDir string) (path string, err error) {
	if legacy := firstExistingFile(v, configDir, legacyLocalNames); legacy != "" {
		return "", newLegacyConfigError(legacy)
	}
	name := localVariantName(configTOMLFilename)
	info, statErr := v.Stat(filepath.Join(configDir, name))
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return "", nil
		}
		return "", oops.With("path", filepath.Join(configDir, name)).Wrapf(statErr, "stat local config")
	}
	if info.IsDir() {
		return "", nil
	}
	return filepath.Join(configDir, name), nil
}

// decodeConfigDoc decodes TOML config bytes into a generic document.
func decodeConfigDoc(path string, data []byte) (map[string]any, error) {
	doc := map[string]any{}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, oops.With("path", path).Hint("Check the file syntax").Wrapf(err, "parse %s", filepath.Base(path))
	}
	if swapped := swappedLintTables(path, doc); swapped != nil {
		return nil, swapped
	}
	return doc, nil
}

func readConfigDoc(v workspace.View, path string) (map[string]any, error) {
	data, err := v.ReadFile(path)
	if err != nil {
		return nil, oops.With("path", path).Wrapf(err, "read %s", filepath.Base(path))
	}
	return decodeConfigDoc(path, data)
}

// withLocalOverlay applies a config.local.* overlay found beside the main
// config, unless disabled. Without an overlay cfg is returned untouched.
func withLocalOverlay(v workspace.View, cfg *Config, mainPath, configDir string, lo loadOptions) (*Config, error) {
	if lo.withoutLocal {
		return cfg, nil
	}
	localPath, err := findLocalConfigFile(v, configDir)
	if err != nil || localPath == "" {
		return cfg, err
	}

	if err := checkConfigFileInside(v, localPath); err != nil {
		return nil, err
	}
	mainDoc, err := readConfigDoc(v, mainPath)
	if err != nil {
		return nil, err
	}
	localDoc, err := readConfigDoc(v, localPath)
	if err != nil {
		return nil, err
	}
	merged, warnings, err := MergeConfigDocs(mainDoc, localDoc)
	if err != nil {
		return nil, oops.With("path", localPath).Wrapf(err, "apply local overlay %s", filepath.Base(localPath))
	}
	for _, w := range warnings {
		cfg.Warn(w, "path", localPath)
	}

	out, err := decodeMergedDoc(mainPath, localPath, merged)
	if err != nil {
		return nil, err
	}
	out.ConfigFile = cfg.ConfigFile
	out.LocalOverlay = &LocalOverlay{
		Path: localPath,
		Doc:  normalizeConfigDocKeys(localDoc),
	}
	if isGitTracked(filepath.Dir(localPath), localPath) {
		out.LocalOverlay.Tracked = true
		cfg.Warn("config.local overlay is tracked by git, so it is repository content: its local includes get the same project-containment check as config.toml", "path", localPath)
	}
	return out, nil
}

// decodeMergedDoc decodes a merged document into a Config through JSON.
func decodeMergedDoc(mainPath, localPath string, merged map[string]any) (*Config, error) {
	cfg, err := decodeJSONDoc(DocForJSON(merged), mainPath) // "$schema" is the json tag
	if err != nil {
		return nil, oops.
			With("path", localPath).
			Hint("Check the value types in the local overlay").
			Wrapf(SanitizeDecodeError(err), "decode %s merged with %s", filepath.Base(mainPath), filepath.Base(localPath))
	}
	return cfg, nil
}

// quotedValueRe matches the backtick-quoted source value that decode errors
// carry ("cannot unmarshal string `value` into []string").
var quotedValueRe = regexp.MustCompile("\\s*`[^`]*`")

// SanitizeDecodeError returns an error with the offending source values removed
// from a decode failure, keeping line numbers and types. The merged document
// includes machine-local values that may be credentials, so they must not reach
// terminals, logs or MCP responses.
func SanitizeDecodeError(err error) error {
	return errors.New(quotedValueRe.ReplaceAllString(err.Error(), ""))
}

// decodeJSONDoc decodes a document through JSON into a Config.
func decodeJSONDoc(doc map[string]any, path string) (*Config, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, err //nolint:wrapcheck // wrapped by decodeMergedDoc
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, oops.
			With("path", path).
			Hint("Check the value types in the local overlay").
			Wrapf(err, "decode merged config")
	}
	// agents_md defaults to true in v5; JSON cannot tell "absent" from false.
	if _, stated := doc["agents_md"]; !stated {
		cfg.AgentsMD = true
	}
	return &cfg, nil
}

// ReadLocalOverlay reads config.local.toml beside the main config in
// configDir without loading the rest of the configuration. It returns nil when
// there is none.
func ReadLocalOverlay(configDir string) (*LocalOverlay, error) {
	v := osView(configDir)
	localPath, err := findLocalConfigFile(v, configDir)
	if err != nil || localPath == "" {
		return nil, err
	}
	if err := checkConfigFileInside(v, localPath); err != nil {
		return nil, err
	}
	doc, err := readConfigDoc(v, localPath)
	if err != nil {
		return nil, err
	}
	return &LocalOverlay{
		Path: localPath,
		Doc:  normalizeConfigDocKeys(doc),
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

// isGitTracked reports whether git tracks file, asking git from dir. A missing
// git binary, a directory outside a repository or a virtual path all read as
// untracked.
func isGitTracked(dir, file string) bool {
	if !filepath.IsAbs(file) {
		return false
	}
	cmd := exec.Command("git", "-C", dir, "ls-files", "--error-unmatch", "--", file) //nolint:gosec // fixed program, the path is an argument
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	return cmd.Run() == nil
}
