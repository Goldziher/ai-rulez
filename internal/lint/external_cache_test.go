package lint

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingScanner appends a line to counter on every scan (not on --version)
// and reports one finding on the rule file whose message embeds the run count.
func countingScanner(t *testing.T) (bin, counter string) {
	t.Helper()
	counter = filepath.Join(t.TempDir(), "count")
	bin = fakeBin(t, "count-scan", `if [ "$1" = "--version" ]; then echo "count-scan 3.1.4"; exit 0; fi
echo run >> `+counter+`
printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"C1","level":"error","message":{"text":"found it"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":".ai-rulez/rules/r.md"},"region":{"startLine":4}}}]}]}]}'
`)
	return bin, counter
}

func runs(t *testing.T, counter string) int {
	t.Helper()
	data, err := os.ReadFile(counter)
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	return strings.Count(string(data), "run")
}

func cachedProject(t *testing.T, bin string) (*scannerProject, *ScanCache) {
	t.Helper()
	cache := NewScanCache(filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "secret", "scan.key"))
	require.NotNil(t, cache)
	p := policyProject(t, "\n[lint.scanner_policy]\nisolation = \"none\"\n\n[[lint.external]]\nname = \"count\"\ncommand = [\""+bin+"\", \"{stage}\"]\negress = false\ninputs = [\"rules\", \"skills\"]\n")
	return p, cache
}

func TestScanResultCache(t *testing.T) {
	bin, counter := countingScanner(t)
	p, cache := cachedProject(t, bin)
	opts := Options{Scanner: ScannerOptions{Cache: cache}}

	// Act: the first run misses, the second hits and gives the same findings.
	first := scannerFindings(p.run(opts))
	second := scannerFindings(p.run(opts))

	// Assert
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, 1, runs(t, counter), "the second run must be served from the cache")
	assert.Equal(t, first[0].Message, second[0].Message)
	assert.Equal(t, first[0].File, second[0].File)
	assert.Equal(t, first[0].Line, second[0].Line)
	assert.Equal(t, first[0].Fingerprint(), second[0].Fingerprint())

	// A changed staged file is a miss.
	p.write(".ai-rulez/rules/r.md", "# Rule\n\nline two\nBADTOKEN here\nmore\n")
	p.run(opts)
	assert.Equal(t, 2, runs(t, counter))

	// A file that is not staged for the scanner does not invalidate the entry.
	p.write("main.go", "package main\n")
	p.run(opts)
	assert.Equal(t, 2, runs(t, counter))

	// NoCache bypasses reads and writes.
	p.run(Options{Scanner: ScannerOptions{Cache: cache, NoCache: true}})
	assert.Equal(t, 3, runs(t, counter))
}

func TestScanCacheKeyCoversTheScannerAndItsConfiguration(t *testing.T) {
	bin, counter := countingScanner(t)
	p, cache := cachedProject(t, bin)
	opts := Options{Scanner: ScannerOptions{Cache: cache}}
	p.run(opts)
	require.Equal(t, 1, runs(t, counter))

	// Different arguments: a miss.
	cfg, err := os.ReadFile(filepath.Join(p.root, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	p.write(".ai-rulez/config.toml", strings.Replace(string(cfg), `"{stage}"]`, `"{stage}", "--extra"]`, 1))
	p.run(opts)
	assert.Equal(t, 2, runs(t, counter), "changed arguments must miss")

	// A replaced binary (new size) is a miss.
	require.NoError(t, os.WriteFile(bin, append(mustRead(t, bin), []byte("# upgraded\n")...), 0o755)) //nolint:gosec // test script
	p.run(opts)
	assert.Equal(t, 3, runs(t, counter), "a changed scanner binary must miss")

	// Severity mapping changes the findings: a miss.
	p.write(".ai-rulez/config.toml", strings.Replace(string(cfg), `"{stage}"]`, `"{stage}", "--extra"]`, 1)+"severity_map = { \"C*\" = \"low\" }\n")
	p.run(opts)
	assert.Equal(t, 4, runs(t, counter))
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestScanCacheRejectsTamperedAndPlantedEntries(t *testing.T) {
	bin, counter := countingScanner(t)
	p, cache := cachedProject(t, bin)
	opts := Options{Scanner: ScannerOptions{Cache: cache}}
	p.run(opts)
	require.Equal(t, 1, runs(t, counter))
	var entries []string
	require.NoError(t, filepath.Walk(cache.dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() && strings.HasSuffix(path, ".json") {
			entries = append(entries, path)
		}
		return err
	}))
	require.Len(t, entries, 1)

	tests := []struct {
		name   string
		tamper func(t *testing.T, path string)
	}{
		{"edited payload", func(t *testing.T, path string) {
			var e map[string]any
			require.NoError(t, json.Unmarshal(mustRead(t, path), &e))
			e["payload"] = json.RawMessage(`{"findings":[]}`)
			data, err := json.Marshal(e)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0o600))
		}},
		{"truncated", func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, mustRead(t, path)[:20], 0o600))
		}},
		{"swapped for a symlink", func(t *testing.T, path string) {
			target := filepath.Join(t.TempDir(), "target.json")
			require.NoError(t, os.WriteFile(target, mustRead(t, path), 0o600))
			require.NoError(t, os.Remove(path))
			testutil.SymlinkOrSkip(t, target, path)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := runs(t, counter)
			tt.tamper(t, entries[0])
			got := scannerFindings(p.run(opts))
			require.Len(t, got, 1, "a rejected entry must fall back to a real run")
			assert.Equal(t, before+1, runs(t, counter))
			// The run replaced it with a valid entry: the next run hits.
			p.run(opts)
			assert.Equal(t, before+1, runs(t, counter))
		})
	}
}

func TestScanCacheIsPerSecret(t *testing.T) {
	bin, counter := countingScanner(t)
	p, cache := cachedProject(t, bin)
	p.run(Options{Scanner: ScannerOptions{Cache: cache}})
	// Another user's secret cannot validate the entry.
	other := NewScanCache(cache.dir, filepath.Join(t.TempDir(), "other", "scan.key"))
	p.run(Options{Scanner: ScannerOptions{Cache: other}})
	assert.Equal(t, 2, runs(t, counter))
	// And the secret file is private.
	info, err := os.Stat(cache.secretPath)
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o077)
}

func TestEgressScannersAreNeverCached(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "count")
	bin := fakeBin(t, "egress-count", `echo run >> `+counter+"\necho '"+sarifFor("E1", "note", "x")+"'\n")
	cache := NewScanCache(filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "k"))
	p := policyProject(t, "\n[lint.scanner_policy]\nisolation = \"none\"\n\n[[lint.external]]\nname = \"e\"\ncommand = [\""+bin+"\", \"{stage}\"]\negress = true\ninputs = [\"rules\"]\n")
	opts := Options{AllowEgress: []string{"e"}, Scanner: ScannerOptions{Cache: cache}}
	p.run(opts)
	p.run(opts)
	assert.Equal(t, 2, runs(t, counter))
}

func TestCachedOutOfScopeResultsStayCounted(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "count")
	bin := fakeBin(t, "oos-scan", `if [ "$1" = "--version" ]; then echo 1.0.0; exit 0; fi
echo run >> `+counter+`
printf '{"version":"2.1.0","runs":[{"results":[{"ruleId":"X","level":"error","message":{"text":"elsewhere"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"/etc/passwd"},"region":{"startLine":1}}}]}]}]}'
`)
	cache := NewScanCache(filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "k"))
	p := policyProject(t, "\n[lint.scanner_policy]\nisolation = \"none\"\n\n[[lint.external]]\nname = \"o\"\ncommand = [\""+bin+"\", \"{stage}\"]\negress = false\ninputs = [\"rules\"]\n")
	opts := Options{Scanner: ScannerOptions{Cache: cache}}
	for range 2 {
		findings := p.run(opts)
		assert.Empty(t, scannerFindings(findings))
		assert.Len(t, ofCode(findings, CodeScannerOutOfScope), 1, dump(findings))
	}
	assert.Equal(t, 1, runs(t, counter))
}

func TestDryRunStartsNothingAndPrintsThePlan(t *testing.T) {
	bin, counter := countingScanner(t)
	t.Setenv("AR_PLANTED_API_KEY", "super-secret-value")
	p, cache := cachedProject(t, bin)
	var out bytes.Buffer
	opts := Options{Scanner: ScannerOptions{Cache: cache, DryRun: true, Out: &out}}

	findings := p.run(opts)

	assert.Empty(t, scannerFindings(findings))
	assert.Equal(t, 0, runs(t, counter), "a dry run starts no scanner")
	plan := out.String()
	assert.Contains(t, plan, "scanner count (egress = false")
	assert.Contains(t, plan, bin)
	assert.Contains(t, plan, "<stage>")
	assert.Contains(t, plan, ".ai-rulez/rules/r.md")
	assert.Contains(t, plan, ".ai-rulez/skills/deploy/SKILL.md")
	assert.Contains(t, plan, "cache:     miss")
	assert.Contains(t, plan, "HOME")
	assert.NotContains(t, plan, "super-secret-value", "environment values are never printed")
	assert.NotContains(t, plan, "AR_PLANTED_API_KEY", "a variable outside the allow-list is not part of the scanner's environment")

	// After a real run the plan reports the hit, still without starting anything.
	p.run(Options{Scanner: ScannerOptions{Cache: cache}})
	require.Equal(t, 1, runs(t, counter))
	out.Reset()
	p.run(opts)
	assert.Contains(t, out.String(), "cache:     hit")
	assert.Equal(t, 1, runs(t, counter))
}

func TestDryRunPlansAnInRootScannerWithoutRunningIt(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "count")
	bin := fakeBin(t, "root-count", `echo run >> `+counter+"\n")
	p := policyProject(t, "\n[[lint.external]]\nname = \"root\"\ncommand = [\""+bin+"\"]\negress = false\n")
	var out bytes.Buffer
	findings := p.run(Options{Scanner: ScannerOptions{DryRun: true, Out: &out}})
	assert.Empty(t, scannerFindings(findings))
	assert.Equal(t, 0, runs(t, counter))
	assert.Contains(t, out.String(), "in the project root")
	assert.Contains(t, out.String(), ".ai-rulez/rules/r.md")
}

func TestPluginLayoutStagesSkillsAgentsAndCommandsAtTheRoot(t *testing.T) {
	bin := fakeBin(t, "layout-scan", "echo '{}'\n")
	p := policyProject(t, "\n[[lint.external]]\nname = \"l\"\ncommand = [\""+bin+"\", \"{stage}\"]\negress = false\ninputs = [\"skills\", \"rules\"]\n")
	p.write(".ai-rulez/agents/helper.md", "---\nname: helper\ndescription: Use when helping.\n---\n# Helper\n")
	cfg := loadNoRemote(t, p.root)
	tree, err := LoadTree(p.root)
	require.NoError(t, err)
	r := &runner{cfg: cfg, tree: tree, docs: map[string]doc{}}
	r.collect()
	files := r.stageFiles(map[string]bool{inputSkills: true, inputAgents: true, inputRules: true}, "plugin")
	var rels []string
	for _, f := range files {
		rels = append(rels, f.rel)
	}
	assert.Equal(t, []string{"agents/helper.md", "skills/deploy/SKILL.md", "skills/deploy/references/note.md", "skills/deploy/scripts/run.sh"}, rels)
	for _, f := range files {
		assert.NotEmpty(t, f.source)
	}
}

func TestScanRecordsReadTheCacheWithoutStartingAnything(t *testing.T) {
	bin, counter := countingScanner(t)
	p, cache := cachedProject(t, bin)
	load := func() []ScanRecord {
		cfg := loadNoRemote(t, p.root)
		tree, err := LoadTree(p.root)
		require.NoError(t, err)
		recs, err := ScanRecords(cfg, tree, Options{Scanner: ScannerOptions{Cache: cache}})
		require.NoError(t, err)
		return recs
	}

	// Before any scan: a record without a result, and nothing was started.
	before := load()
	require.Len(t, before, 1)
	assert.False(t, before[0].Cached)
	assert.NotEmpty(t, before[0].Tree)
	assert.Equal(t, 0, runs(t, counter))

	// After a scan: the cached result, still without starting the scanner again.
	p.run(Options{Scanner: ScannerOptions{Cache: cache}})
	after := load()
	require.Len(t, after, 1)
	assert.True(t, after[0].Cached)
	assert.Equal(t, before[0].Tree, after[0].Tree)
	assert.Equal(t, "count-scan 3.1.4", after[0].Version)
	assert.Equal(t, 1, after[0].Findings)
	assert.Equal(t, "error", after[0].MaxSeverity)
	assert.False(t, after[0].Pass, "an error-level finding fails the default threshold")
	assert.Equal(t, 1, runs(t, counter))

	// A change to the staged content has no result.
	p.write(".ai-rulez/rules/r.md", "# Rule\n\nchanged\n")
	changed := load()
	assert.False(t, changed[0].Cached)
	assert.NotEqual(t, before[0].Tree, changed[0].Tree)
}
