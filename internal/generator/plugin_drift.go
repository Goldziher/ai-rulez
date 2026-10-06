package generator

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// driftRef is the baseline plugin output is compared against.
const driftRef = "HEAD"

// manifestCandidates are the manifests that carry a plugin's version, in the
// order they are tried.
var manifestCandidates = []string{
	".claude-plugin/plugin.json",
	".cursor-plugin/plugin.json",
	".codex-plugin/plugin.json",
	"plugin.json",
}

// maxDriftFiles bounds how many changed files a finding names.
const maxDriftFiles = 3

// PluginVersionDrift compares the plugin bundles a run would generate against
// the ones committed at HEAD. A bundle whose recorded content changed while the
// version in its manifest stayed the same is drift: a client that cached the
// plugin at that version keeps the old copy. A bundle without a version is
// tracked by commit, so it never drifts, and a bundle that is new since HEAD has
// no baseline. Outside a git repository, or with no plugin configured, the
// result is empty.
func (g *Generator) PluginVersionDrift(profile string) ([]lint.PluginDrift, error) {
	if g.config.Plugin == nil && g.config.Marketplace == nil {
		return nil, nil
	}
	top := g.git().TopLevel(g.config.BaseDir)
	if top == "" {
		return nil, nil
	}
	// The committed side is read from a snapshot of HEAD, not from the work tree.
	// A repository without a commit has no baseline, so nothing drifts.
	snap, err := workspace.GitSnapshot(g.context(), top, driftRef, g.host().Runner)
	if err != nil {
		return nil, nil //nolint:nilerr // no baseline is not an error
	}
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]config.OutputFile, len(outputs))
	for _, o := range outputs {
		if !o.IsDir {
			byPath[filepath.Clean(o.Path)] = o
		}
	}

	var drift []lint.PluginDrift
	for path, sidecar := range byPath {
		if filepath.Base(path) != plugin.ProvenanceFileName {
			continue
		}
		bundle := filepath.Dir(path)
		found, ok := driftFor(snap, top, bundle, sidecar, byPath)
		if ok {
			drift = append(drift, found)
		}
	}
	sort.Slice(drift, func(i, j int) bool { return drift[i].File < drift[j].File })
	return drift, nil
}

func driftFor(snap workspace.Workspace, top, bundle string, sidecar config.OutputFile, byPath map[string]config.OutputFile) (lint.PluginDrift, bool) {
	baseline, err := snap.ReadFile(gitutil.RepoRelative(top, filepath.Join(bundle, plugin.ProvenanceFileName)))
	if err != nil {
		return lint.PluginDrift{}, false
	}
	nowHashes, nowSource, err := plugin.ProvenanceOutputs(outputBytes(sidecar))
	if err != nil {
		return lint.PluginDrift{}, false
	}
	thenHashes, thenSource, err := plugin.ProvenanceOutputs(baseline)
	if err != nil || nowSource == thenSource {
		return lint.PluginDrift{}, false
	}

	for _, rel := range manifestCandidates {
		manifest, present := byPath[filepath.Join(bundle, filepath.FromSlash(rel))]
		if !present {
			continue
		}
		now := manifestFields(outputBytes(manifest))
		if now.version == "" {
			return lint.PluginDrift{}, false
		}
		before, err := snap.ReadFile(gitutil.RepoRelative(top, manifest.Path))
		if err != nil || manifestFields(before).version != now.version {
			return lint.PluginDrift{}, false
		}
		return lint.PluginDrift{
			Plugin:  firstNonEmptyString(now.name, filepath.Base(bundle)),
			File:    manifest.Path,
			Version: now.version,
			Changed: changedFiles(nowHashes, thenHashes),
		}, true
	}
	return lint.PluginDrift{}, false
}

type manifestInfo struct{ name, version string }

func manifestFields(data []byte) manifestInfo {
	var m struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return manifestInfo{}
	}
	return manifestInfo{name: m.Name, version: m.Version}
}

// changedFiles names up to maxDriftFiles files whose recorded hash differs,
// including files added or removed, sorted.
func changedFiles(now, then map[string]string) []string {
	var changed []string
	for path, hash := range now {
		if then[path] != hash {
			changed = append(changed, path)
		}
	}
	for path := range then {
		if _, still := now[path]; !still {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	if len(changed) > maxDriftFiles {
		extra := len(changed) - maxDriftFiles
		changed = append(changed[:maxDriftFiles], "and "+strconv.Itoa(extra)+" more")
	}
	return changed
}

func outputBytes(o config.OutputFile) []byte {
	if o.RawContent != nil {
		return o.RawContent
	}
	return []byte(o.Content)
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
