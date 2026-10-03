// Package opencodev1 detects OpenCode v1-shaped plugins. OpenCode v2 does not run
// them: a v1 plugin is an exported function that returns hooks, while a v2 plugin
// default-exports a definition with an id and a setup function. OpenCode only
// logs the failure to its server log, so ai-rulez surfaces it while generating.
package opencodev1

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

// MigrationHint tells the author what to change and where the official guide lives.
const MigrationHint = "OpenCode v2 does not run v1 plugins (an exported function that returns hooks). " +
	"Default-export Plugin.define({ id, setup(ctx) }) from @opencode/plugin instead and register " +
	"hooks on ctx; see https://opencode.ai/v2/docs/build/plugins/migrate-v1"

// maxSourceBytes bounds how much of a plugin file is inspected.
const maxSourceBytes = 1 << 20

var (
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComment  = regexp.MustCompile(`(?m)^[ \t]*//.*$`)

	v2Define = regexp.MustCompile(`\bPlugin\.define\s*\(`)
	v2ID     = regexp.MustCompile(`\bid\s*:\s*["'` + "`" + `]`)
	v2Setup  = regexp.MustCompile(`\b(?:setup|effect)\s*[(:]`)

	v1DefaultFunction = regexp.MustCompile(
		`(?m)^[ \t]*export\s+default\s+(?:async\s+)?(?:function\b|\([^)]*\)\s*=>|[A-Za-z_$][\w$]*\s*=>)`)
	v1NamedArrow    = regexp.MustCompile(`(?m)^[ \t]*export\s+(?:const|let|var)\s+[A-Za-z_$][\w$]*\s*(?::[^=\n]+)?=\s*async\b`)
	v1NamedFunction = regexp.MustCompile(`(?m)^[ \t]*export\s+async\s+function\s+[A-Za-z_$]`)
	v1Package       = regexp.MustCompile(`from\s+["']@opencode-ai/plugin["']`)
)

// IsV1Plugin reports whether a JavaScript or TypeScript plugin source has the v1
// shape. The heuristics are deliberately conservative: any v2 marker (a
// Plugin.define call, or an id together with setup/effect) means "not v1".
func IsV1Plugin(source string) bool {
	code := lineComment.ReplaceAllString(blockComment.ReplaceAllString(source, ""), "")
	if v2Define.MatchString(code) || (v2ID.MatchString(code) && v2Setup.MatchString(code)) {
		return false
	}
	return v1DefaultFunction.MatchString(code) ||
		v1NamedArrow.MatchString(code) ||
		v1NamedFunction.MatchString(code) ||
		v1Package.MatchString(code)
}

// Finding is a local plugin file that has the v1 shape.
type Finding struct {
	Path string
}

var (
	warnedMu sync.Mutex
	warned   = map[string]bool{}
)

// ResetWarned clears the set of files already warned about; for tests.
func ResetWarned() {
	warnedMu.Lock()
	defer warnedMu.Unlock()
	warned = map[string]bool{}
}

// claim reports whether path has not been warned about yet, and marks it.
func claim(path string) bool {
	warnedMu.Lock()
	defer warnedMu.Unlock()
	if warned[path] {
		return false
	}
	warned[path] = true
	return true
}

// WarnSource warns once when an authored plugin entrypoint has the v1 shape.
func WarnSource(path, source string) bool {
	if !IsV1Plugin(source) || !claim(path) {
		return false
	}
	logger.Warn("OpenCode plugin entrypoint is a v1 plugin", "file", path, "hint", MigrationHint)
	return true
}

// WarnProject scans the project for v1-shaped local plugins and warns once per
// file. It never fails: unreadable files and malformed config are skipped.
func WarnProject(root string) []Finding {
	var fresh []Finding
	for _, finding := range ScanProject(root) {
		if !claim(finding.Path) {
			continue
		}
		logger.Warn("OpenCode plugin is a v1 plugin and will not load in OpenCode v2",
			"file", finding.Path, "hint", MigrationHint)
		fresh = append(fresh, finding)
	}
	return fresh
}

var pluginExtensions = map[string]bool{".js": true, ".ts": true, ".mjs": true}

var entryNames = []string{"index", "server"}

// ScanProject returns the v1-shaped local plugins found in the project's
// .opencode/plugin(s) directories and in the plugin arrays of opencode.json(c).
// Only local files are inspected; npm package names are never resolved.
func ScanProject(root string) []Finding {
	candidates := map[string]bool{}
	for _, dir := range []string{"plugin", "plugins"} {
		collectDirectory(filepath.Join(root, ".opencode", dir), candidates)
	}
	for _, rel := range []string{"opencode.json", "opencode.jsonc", filepath.Join(".opencode", "opencode.json"), filepath.Join(".opencode", "opencode.jsonc")} {
		collectConfigured(filepath.Join(root, rel), candidates)
	}

	paths := make([]string, 0, len(candidates))
	for path := range candidates {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var findings []Finding
	for _, path := range paths {
		if source, ok := readSource(path); ok && IsV1Plugin(source) {
			findings = append(findings, Finding{Path: path})
		}
	}
	return findings
}

func collectDirectory(dir string, out map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			collectEntry(path, out)
		} else if pluginExtensions[filepath.Ext(path)] {
			out[path] = true
		}
	}
}

// collectEntry adds a plugin directory's index/server entry files.
func collectEntry(dir string, out map[string]bool) {
	for _, name := range entryNames {
		for ext := range pluginExtensions {
			path := filepath.Join(dir, name+ext)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				out[path] = true
			}
		}
	}
}

func collectConfigured(configPath string, out map[string]bool) {
	data, ok := readBytes(configPath)
	if !ok {
		return
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC(data), &doc); err != nil {
		return
	}
	base := filepath.Dir(configPath)
	for _, key := range []string{"plugin", "plugins"} {
		var items []any
		if raw, found := doc[key]; !found || json.Unmarshal(raw, &items) != nil {
			continue
		}
		for _, item := range items {
			spec := pluginSpec(item)
			target, local := localTarget(base, spec)
			if !local {
				continue
			}
			info, err := os.Stat(target)
			switch {
			case err != nil:
			case info.IsDir():
				collectEntry(target, out)
			default:
				out[target] = true
			}
		}
	}
}

// pluginSpec extracts the package string from "x", ["x", {...}] or {"package": "x"}.
func pluginSpec(item any) string {
	switch value := item.(type) {
	case string:
		return value
	case []any:
		if len(value) > 0 {
			if spec, ok := value[0].(string); ok {
				return spec
			}
		}
	case map[string]any:
		if spec, ok := value["package"].(string); ok {
			return spec
		}
	}
	return ""
}

// localTarget resolves a plugin spec to a filesystem path; npm names are not local.
func localTarget(base, spec string) (string, bool) {
	switch {
	case strings.HasPrefix(spec, "file://"):
		return filepath.FromSlash(strings.TrimPrefix(spec, "file://")), true
	case strings.HasPrefix(spec, "./"), strings.HasPrefix(spec, "../"):
		return filepath.Join(base, filepath.FromSlash(spec)), true
	case filepath.IsAbs(spec):
		return spec, true
	}
	return "", false
}

func readBytes(path string) ([]byte, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxSourceBytes))
	return data, err == nil
}

func readSource(path string) (string, bool) {
	data, ok := readBytes(path)
	return string(data), ok
}

// stripJSONC removes comments and trailing commas so encoding/json can parse it.
func stripJSONC(in []byte) []byte {
	var out bytes.Buffer
	for i := 0; i < len(in); i++ {
		switch {
		case in[i] == '"':
			end := endOfString(in, i)
			out.Write(in[i : end+1])
			i = end
		case hasPrefixAt(in, i, "//"):
			for i < len(in) && in[i] != '\n' {
				i++
			}
			out.WriteByte('\n')
		case hasPrefixAt(in, i, "/*"):
			i += 2
			for i+1 < len(in) && !hasPrefixAt(in, i, "*/") {
				i++
			}
			i++
		default:
			out.WriteByte(in[i])
		}
	}
	return trailingComma.ReplaceAll(out.Bytes(), []byte("$1"))
}

// endOfString returns the index of the quote closing the string that opens at start.
func endOfString(in []byte, start int) int {
	for i := start + 1; i < len(in); i++ {
		switch in[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return len(in) - 1
}

func hasPrefixAt(in []byte, i int, prefix string) bool {
	return bytes.HasPrefix(in[i:], []byte(prefix))
}

var trailingComma = regexp.MustCompile(`,(\s*[\]}])`)
