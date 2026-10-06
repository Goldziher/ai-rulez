package config

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// VerifiersDirName is the directory under a configuration directory that holds
// verifier declaration files (`*.toml`, each with [[verifiers]] tables).
const VerifiersDirName = "verifiers"

// maxVerifierFileBytes bounds one declaration file.
const maxVerifierFileBytes = 1 << 20

// ImportedVerifierFile is a verifier declaration file that arrived through an
// include. Its verifiers are data, not trusted code: they may not use the
// command predicate unless the include is named in [verifiers_settings]
// trust_exec_from.
type ImportedVerifierFile struct {
	// Include is the name of the include (or installed skill) the file came from.
	Include string
	// Skill is true when it came from an installed skill's verifiers/ directory.
	// A skill's verifiers never use the command predicate, trusted or not.
	Skill bool
	// Name is the file name under the include's verifiers/ directory.
	Name string
	// Data is the file content.
	Data string
}

// ScanVerifierFiles reads the `*.toml` files of <configDir>/verifiers in name
// order. Symlinks, non-regular files and files over 1 MiB are skipped, never
// followed. A missing directory is not an error.
func ScanVerifierFiles(configDir string) ([]ImportedVerifierFile, error) {
	return ScanVerifierFilesIn(osView(configDir), configDir)
}

// ScanVerifierFilesIn is ScanVerifierFiles reading through v.
func ScanVerifierFilesIn(v workspace.View, configDir string) ([]ImportedVerifierFile, error) {
	dir := filepath.Join(configDir, VerifiersDirName)
	entries, err := v.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, oops.With("dir", dir).Wrapf(err, "read verifiers directory")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []ImportedVerifierFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := v.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue // a symlink or special file is never followed
		}
		f, err := v.Open(path) //nolint:gosec // a regular file inside the include
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(f, maxVerifierFileBytes+1))
		_ = f.Close()
		if readErr != nil || len(data) > maxVerifierFileBytes {
			continue
		}
		out = append(out, ImportedVerifierFile{Name: e.Name(), Data: string(data)})
	}
	return out, nil
}
