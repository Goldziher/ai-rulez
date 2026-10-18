package lint

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// invocation is one start of the fake scanner, read back from its --log file.
type invocation struct {
	start, end int64
	skills     []string
}

// invocations parses the --log file: one entry per start, in start order, with
// the skills whose directory was passed and the time the run ended (0 if it never did).
func invocations(t *testing.T, logFile string) []invocation {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var out []invocation
	open := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		ts, perr := strconv.ParseInt(fields[1], 10, 64)
		require.NoError(t, perr)
		argsKey := strings.Join(fields[2:], " ")
		if fields[0] == "end" {
			out[open[argsKey]].end = ts
			continue
		}
		inv := invocation{start: ts}
		for _, f := range fields[2:] {
			if p := filepath.ToSlash(f); strings.Contains(p, "/.ai-rulez/skills/") {
				inv.skills = append(inv.skills, filepath.Base(f))
			}
		}
		sort.Strings(inv.skills)
		open[argsKey] = len(out)
		out = append(out, inv)
	}
	return out
}

func skillsPerRun(t *testing.T, logFile string) [][]string {
	t.Helper()
	var out [][]string
	for _, inv := range invocations(t, logFile) {
		out = append(out, inv.skills)
	}
	return out
}

// skillUnitProject is a staged skills scanner that is handed {skill_dirs}.
func skillUnitProject(t *testing.T, keys string, skills ...string) (p *scannerProject, logFile string) {
	t.Helper()
	logFile = filepath.Join(t.TempDir(), "runs.log")
	p = fakeProject(t, isolationNone, fakeScannerCommand(t, "--log", logFile, "{skill_dirs}")+"inputs = [\"skills\"]\n"+keys, skills...)
	return p, logFile
}

func findingFiles(findings []Finding) []string {
	var out []string
	for _, f := range findings {
		out = append(out, f.RepoPath())
	}
	sort.Strings(out)
	return out
}

func TestSkillDirsAreCachedPerSkill(t *testing.T) {
	// Arrange
	p, logFile := skillUnitProject(t, "", "alpha", "beta", "gamma")
	opts := Options{Scanner: ScannerOptions{Cache: fakeCache(t)}}

	// Act / Assert: the first run hands the scanner every skill.
	first := scannerFindings(p.run(opts))
	assert.Len(t, first, 3)
	assert.Equal(t, [][]string{{"alpha", "beta", "gamma"}}, skillsPerRun(t, logFile))

	// An unchanged tree is served from the cache.
	p.run(opts)
	assert.Len(t, invocations(t, logFile), 1, "nothing changed, nothing is scanned")

	// Editing one skill scans only that one; the others come from the cache and still report.
	p.write(".ai-rulez/skills/beta/SKILL.md", fakeSkill("beta")+"\nOne more step.\n")
	again := scannerFindings(p.run(opts))
	assert.Equal(t, [][]string{{"alpha", "beta", "gamma"}, {"beta"}}, skillsPerRun(t, logFile))
	assert.Equal(t, []string{
		".ai-rulez/skills/alpha/SKILL.md", ".ai-rulez/skills/beta/SKILL.md", ".ai-rulez/skills/gamma/SKILL.md",
	}, findingFiles(again))

	// A new skill is scanned alone.
	p.write(".ai-rulez/skills/delta/SKILL.md", fakeSkill("delta"))
	p.run(opts)
	assert.Equal(t, []string{"delta"}, skillsPerRun(t, logFile)[2])
}

func TestNoScanCacheBypassesThePerSkillCache(t *testing.T) {
	// Arrange
	p, logFile := skillUnitProject(t, "", "alpha", "beta")
	cache := fakeCache(t)
	p.run(Options{Scanner: ScannerOptions{Cache: cache}})

	// Act
	p.run(Options{Scanner: ScannerOptions{Cache: cache, NoCache: true}})

	// Assert
	assert.Equal(t, [][]string{{"alpha", "beta"}, {"alpha", "beta"}}, skillsPerRun(t, logFile))
}

func TestChangedNarrowsTheSkillsHandedToTheScanner(t *testing.T) {
	tests := []struct {
		name    string
		changed []string
		cached  bool
		want    [][]string
		files   []string
	}{
		{"one changed skill", []string{".ai-rulez/skills/beta/references/api.md"}, false, [][]string{{"beta"}}, []string{".ai-rulez/skills/beta/SKILL.md"}},
		{"two changed skills", []string{".ai-rulez/skills/alpha/SKILL.md", ".ai-rulez/skills/gamma/SKILL.md"}, false, [][]string{{"alpha", "gamma"}},
			[]string{".ai-rulez/skills/alpha/SKILL.md", ".ai-rulez/skills/gamma/SKILL.md"}},
		{"a change outside every skill scans nothing", []string{"README.md", ".ai-rulez/rules/r.md"}, false, nil, nil},
		{"nothing changed", []string{}, false, nil, nil},
		{"a changed skill with a cached result is not scanned", []string{".ai-rulez/skills/beta/SKILL.md"}, true, [][]string{{"alpha", "beta", "gamma"}},
			[]string{".ai-rulez/skills/beta/SKILL.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p, logFile := skillUnitProject(t, "", "alpha", "beta", "gamma")
			p.write(".ai-rulez/skills/beta/references/api.md", "api\n")
			scanner := ScannerOptions{Cache: fakeCache(t)}
			if tt.cached {
				p.run(Options{Scanner: scanner})
			}
			scanner.ChangedFiles = tt.changed

			// Act
			findings := scannerFindings(p.run(Options{Scanner: scanner}))

			// Assert
			runs := skillsPerRun(t, logFile)
			if tt.cached {
				assert.Len(t, runs, 1, "the changed skill is served from the cache")
			} else {
				assert.Equal(t, tt.want, runs)
			}
			assert.Equal(t, tt.files, findingFiles(findings))
		})
	}
}

func TestDryRunShowsThePerSkillCacheState(t *testing.T) {
	// Arrange: alpha and beta are cached, then beta changes and gamma is new.
	p, logFile := skillUnitProject(t, "", "alpha", "beta")
	cache := fakeCache(t)
	p.run(Options{Scanner: ScannerOptions{Cache: cache}})
	p.write(".ai-rulez/skills/beta/SKILL.md", fakeSkill("beta")+"\nEdited.\n")
	p.write(".ai-rulez/skills/gamma/SKILL.md", fakeSkill("gamma"))
	var out bytes.Buffer

	// Act
	p.run(Options{Scanner: ScannerOptions{Cache: cache, DryRun: true, Out: &out}})

	// Assert
	plan := out.String()
	assert.Contains(t, plan, ".ai-rulez/skills/alpha: hit")
	assert.Contains(t, plan, ".ai-rulez/skills/beta: miss")
	assert.Contains(t, plan, ".ai-rulez/skills/gamma: miss")
	assert.Contains(t, plan, "cache:     1 hit, 2 miss")
	assert.Len(t, invocations(t, logFile), 1, "a dry run starts nothing")
}

func TestResultsThatCannotBeAttributedToASkillAreNotCached(t *testing.T) {
	// Arrange: besides one result per skill the scanner reports one that names no file.
	logFile := filepath.Join(t.TempDir(), "runs.log")
	p := fakeProject(t, isolationNone, fakeScannerCommand(t, "--log", logFile, "--also-global", "{skill_dirs}")+"inputs = [\"skills\"]\n", "alpha", "beta")
	opts := Options{Scanner: ScannerOptions{Cache: fakeCache(t)}}

	// Act
	first := scannerFindings(p.run(opts))
	second := scannerFindings(p.run(opts))

	// Assert: the location-less result would be lost on a cache hit, so both skills are scanned again.
	assert.Len(t, first, 3)
	assert.Len(t, second, 3)
	assert.Len(t, invocations(t, logFile), 2)
}

func TestASingleSkillRunCachesEveryResultItProduced(t *testing.T) {
	// Arrange
	logFile := filepath.Join(t.TempDir(), "runs.log")
	p := fakeProject(t, isolationNone, fakeScannerCommand(t, "--log", logFile, "--also-global", "{skill_dirs}")+"inputs = [\"skills\"]\n", "alpha")
	opts := Options{Scanner: ScannerOptions{Cache: fakeCache(t)}}

	// Act
	first := scannerFindings(p.run(opts))
	second := scannerFindings(p.run(opts))

	// Assert
	assert.Len(t, first, 2)
	assert.Len(t, second, 2)
	assert.Len(t, invocations(t, logFile), 1)
}

func TestLockRecordsSumThePerSkillCacheEntries(t *testing.T) {
	// Arrange
	p, _ := skillUnitProject(t, "", "alpha", "beta")
	cache := fakeCache(t)
	p.run(Options{Scanner: ScannerOptions{Cache: cache}})
	records := func() []ScanRecord {
		cfg := loadNoRemote(t, p.root)
		tree, err := LoadTree(p.root)
		require.NoError(t, err)
		got, err := ScanRecords(cfg, tree, Options{Scanner: ScannerOptions{Cache: cache}})
		require.NoError(t, err)
		return got
	}

	// Act / Assert: every skill has an entry, so the scanner has one record with both findings.
	got := records()
	require.Len(t, got, 1)
	assert.True(t, got[0].Cached)
	assert.Equal(t, 2, got[0].Findings)
	assert.Equal(t, "warning", got[0].MaxSeverity)
	assert.True(t, got[0].Pass)

	// A skill without an entry leaves the record uncached.
	p.write(".ai-rulez/skills/beta/SKILL.md", fakeSkill("beta")+"\nEdited.\n")
	got = records()
	require.Len(t, got, 1)
	assert.False(t, got[0].Cached)
}

func TestStageRootPlaceholdersKeepTheWholeRunCached(t *testing.T) {
	// Arrange: {stage} gives the scanner every skill, so one skill cannot be keyed alone.
	logFile := filepath.Join(t.TempDir(), "runs.log")
	p := fakeProject(t, isolationNone, fakeScannerCommand(t, "--log", logFile, "{stage}", "{skill_dirs}")+"inputs = [\"skills\"]\n", "alpha", "beta")
	opts := Options{Scanner: ScannerOptions{Cache: fakeCache(t)}}
	p.run(opts)

	// Act
	p.write(".ai-rulez/skills/beta/SKILL.md", fakeSkill("beta")+"\nEdited.\n")
	p.run(opts)

	// Assert
	assert.Equal(t, [][]string{{"alpha", "beta"}, {"alpha", "beta"}}, skillsPerRun(t, logFile))
}

// runnerFor builds the runner of the project the way RunWith does, for tests
// that look at a stage listing.
func (p *scannerProject) runnerFor(opts Options) *runner {
	p.t.Helper()
	cfg := loadNoRemote(p.t, p.root)
	tree, err := LoadTree(p.root)
	require.NoError(p.t, err)
	opts.External = true
	ruleTables()
	r, err := newRunner(cfg, tree, opts)
	require.NoError(p.t, err)
	if cfg.Lint != nil {
		r.lc = *cfg.Lint
	}
	r.resolveSettings()
	r.collect()
	return r
}

func TestChangedDirsHoldEveryAncestor(t *testing.T) {
	got := changedDirs([]string{"a/b/c.md", "x.md", "a/d/e.md"})
	assert.Equal(t, map[string]bool{"a": true, "a/b": true, "a/d": true}, got)
}

func TestChangedRunStagesOnlyTheChangedSkillsInFull(t *testing.T) {
	// Arrange: three skills with supporting files; only beta changed.
	p, _ := skillUnitProject(t, "", "alpha", "beta", "gamma")
	for _, s := range []string{"alpha", "beta", "gamma"} {
		p.write(".ai-rulez/skills/"+s+"/references/api.md", "api "+s+"\n")
		p.write(".ai-rulez/skills/"+s+"/scripts/run.sh", "echo "+s+"\n")
	}
	changed := []string{".ai-rulez/skills/beta/references/api.md"}
	listing := func(narrow bool) []string {
		r := p.runnerFor(Options{Scanner: ScannerOptions{ChangedFiles: changed}})
		sc := r.scanners()[0]
		files := r.stageFilesFor(sc)
		if narrow {
			files = r.stageFilesToScan(sc)
		}
		var rels []string
		for _, f := range files {
			rels = append(rels, f.rel)
		}
		return rels
	}

	// Act
	full, narrow := listing(false), listing(true)

	// Assert: the changed skill lists exactly what a full run lists; the others only their SKILL.md.
	var wantNarrow []string
	for _, rel := range full {
		if strings.Contains(rel, "/beta/") || strings.HasSuffix(rel, "/SKILL.md") {
			wantNarrow = append(wantNarrow, rel)
		}
	}
	assert.Equal(t, wantNarrow, narrow)
	assert.Greater(t, len(full), len(narrow))
}

func TestChangedRunReportsTheSameFindingsAsAFullRun(t *testing.T) {
	tests := []struct {
		name    string
		changed []string
	}{
		{"a resource of one skill", []string{".ai-rulez/skills/beta/references/api.md"}},
		{"a skill file and a resource of two skills", []string{".ai-rulez/skills/alpha/SKILL.md", ".ai-rulez/skills/gamma/scripts/run.sh"}},
		{"a deleted file inside a skill", []string{".ai-rulez/skills/beta/references/gone.md"}},
		{"nothing in a skill", []string{"README.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p, _ := skillUnitProject(t, "", "alpha", "beta", "gamma")
			for _, s := range []string{"alpha", "beta", "gamma"} {
				p.write(".ai-rulez/skills/"+s+"/references/api.md", "api "+s+"\n")
				p.write(".ai-rulez/skills/"+s+"/scripts/run.sh", "echo "+s+"\n")
			}
			full := scannerFindings(p.run(Options{Scanner: ScannerOptions{}}))
			// Act
			narrow := scannerFindings(p.run(Options{Scanner: ScannerOptions{ChangedFiles: tt.changed}}))
			// Assert: exactly the findings of the full run that lie in a skill holding a changed file that exists.
			var want []Finding
			for _, f := range full {
				for _, c := range tt.changed {
					_, statErr := os.Stat(filepath.Join(p.root, filepath.FromSlash(c)))
					if statErr == nil && strings.HasPrefix(c, filepath.ToSlash(filepath.Dir(f.RepoPath()))+"/") { // a deleted file is not a file of the skill
						want = append(want, f)
						break
					}
				}
			}
			assert.Equal(t, findingFiles(want), findingFiles(narrow))
		})
	}
}

// The "served from the cache" log counts the skills read from the cache, not the
// ones --changed left out, and says nothing under --no-scan-cache (#461).
func TestCacheServedLogCountsOnlyCachedSkills(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string // substring of the log; "" means the line must be absent
	}{
		{"changed run without a cache", Options{Scanner: ScannerOptions{NoCache: true, ChangedFiles: []string{".ai-rulez/skills/beta/SKILL.md"}}}, ""},
		{"changed run with a cold cache", Options{Scanner: ScannerOptions{Cache: nil, ChangedFiles: []string{".ai-rulez/skills/beta/SKILL.md"}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p, _ := skillUnitProject(t, "", "alpha", "beta", "gamma")
			rec := &testutil.LogRecorder{}
			// Act
			p.runWith(tt.opts, WithHost(ambient.Host{Log: rec}))
			// Assert
			assert.NotContains(t, strings.Join(rec.Lines(), "\n"), "served from the cache")
		})
	}
}
