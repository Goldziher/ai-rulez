package lint

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// batchProject is a per-skill scanner over skills s1..sN with the given extra keys and scanner arguments.
func batchProject(t *testing.T, policy, keys string, n int, args ...string) (p *scannerProject, logFile string) {
	t.Helper()
	logFile = filepath.Join(t.TempDir(), "runs.log")
	names := make([]string, n)
	for i := range names {
		names[i] = "s" + string(rune('1'+i))
	}
	all := append([]string{"--log", logFile}, args...)
	all = append(all, "{skill_dirs}")
	p = fakeProject(t, isolationNone+policy, fakeScannerCommand(t, all...)+"inputs = [\"skills\"]\n"+keys, names...)
	return p, logFile
}

// maxConcurrent is the largest number of batches that ran at the same time.
func maxConcurrent(runs []invocation) int {
	best := 0
	for _, a := range runs {
		n := 0
		for _, b := range runs {
			if b.start <= a.start && a.start < b.end {
				n++
			}
		}
		best = max(best, n)
	}
	return best
}

func TestBatchSizeSplitsSkillDirsIntoSeparatelyCachedRuns(t *testing.T) {
	// Arrange
	p, logFile := batchProject(t, "", "batch_size = 2\n", 5)
	opts := Options{Scanner: ScannerOptions{Cache: fakeCache(t)}}

	// Act / Assert: five skills, two per run.
	first := scannerFindings(p.run(opts))
	assert.Len(t, first, 5)
	assert.Equal(t, [][]string{{"s1", "s2"}, {"s3", "s4"}, {"s5"}}, skillsPerRun(t, logFile))

	// Each skill is cached on its own, whatever batch it ran in.
	p.run(opts)
	assert.Len(t, invocations(t, logFile), 3, "everything is cached")
	p.write(".ai-rulez/skills/s4/SKILL.md", fakeSkill("s4")+"\nEdited.\n")
	again := scannerFindings(p.run(opts))
	assert.Len(t, again, 5)
	assert.Equal(t, []string{"s4"}, skillsPerRun(t, logFile)[3])
}

func TestEachBatchGetsItsOwnTimeout(t *testing.T) {
	// Arrange: four batches of 300ms each, 1.2s in all, under a 1s timeout per run.
	p, _ := batchProject(t, "", "batch_size = 1\ntimeout = \"1s\"\n", 4, "--sleep", "300ms")

	// Act
	findings := p.run(Options{})

	// Assert
	assert.Empty(t, ofCode(findings, CodeScannerRunFailed), dump(findings))
	assert.Len(t, scannerFindings(findings), 4)
}

func TestParallelRunsBatchesConcurrentlyAndKeepsTheirOutputApart(t *testing.T) {
	tests := []struct {
		name         string
		keys         string
		wantParallel bool
	}{
		{"default is one at a time", "batch_size = 1\n", false},
		{"parallel = 3 overlaps batches", "batch_size = 1\nparallel = 3\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: each batch writes its report to its own {out} file and takes a moment.
			p, logFile := batchProject(t, "", tt.keys, 3, "--sleep", "500ms", "--out", "{out}")

			// Act
			findings := scannerFindings(p.run(Options{}))

			// Assert
			assert.Equal(t, []string{".ai-rulez/skills/s1/SKILL.md", ".ai-rulez/skills/s2/SKILL.md", ".ai-rulez/skills/s3/SKILL.md"}, findingFiles(findings))
			runs := invocations(t, logFile)
			require.Len(t, runs, 3)
			if tt.wantParallel {
				assert.GreaterOrEqual(t, maxConcurrent(runs), 2, "batches overlapped")
			} else {
				assert.Equal(t, 1, maxConcurrent(runs), "batches ran one after the other")
			}
		})
	}
}

func TestBatchKeysAreValidated(t *testing.T) {
	tests := []struct {
		name string
		args []string
		keys string
		want string
	}{
		{"negative batch_size", []string{"{skill_dirs}"}, "batch_size = -1\n", "batch_size"},
		{"parallel above the bound", []string{"{skill_dirs}"}, "batch_size = 1\nparallel = 100\n", "parallel"},
		{"negative parallel", []string{"{skill_dirs}"}, "parallel = -2\n", "parallel"},
		{"batch_size without skill dirs", []string{"{stage}"}, "batch_size = 2\n", "{skill_dirs}"},
		{"parallel with a command that also passes the stage", []string{"{skill_dirs}", "{stage}"}, "batch_size = 2\nparallel = 2\n", "{skill_dirs}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := fakeProject(t, isolationNone, fakeScannerCommand(t, tt.args...)+"inputs = [\"skills\"]\n"+tt.keys, "s1")

			// Act
			findings := p.run(Options{})

			// Assert
			invalid := ofCode(findings, CodeScannerConfigInvalid)
			require.NotEmpty(t, invalid, dump(findings))
			assert.Contains(t, invalid[0].Message, tt.want)
			assert.Empty(t, scannerFindings(findings))
		})
	}
}

func TestMaxTimeoutRaisesTheCeilingOfAScannerTimeout(t *testing.T) {
	tests := []struct {
		name       string
		policy     string
		timeout    string
		wantProb   string
		wantTimout time.Duration
	}{
		{"default ceiling refuses 30m", "", "30m", "exceeds the maximum of 15m0s", 0},
		{"policy raises the ceiling", "\n[lint.scanner_policy]\nmax_timeout = \"1h\"\n", "30m", "", 30 * time.Minute},
		{"a timeout above the raised ceiling is refused", "\n[lint.scanner_policy]\nmax_timeout = \"1h\"\n", "2h", "exceeds the maximum of 1h0m0s", 0},
		{"the ceiling has a hard bound", "\n[lint.scanner_policy]\nmax_timeout = \"7h\"\n", "30m", "max_timeout", 0},
		{"a malformed ceiling", "\n[lint.scanner_policy]\nmax_timeout = \"soon\"\n", "30m", "max_timeout", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := fakeProject(t, tt.policy, fakeScannerCommand(t)+"timeout = \""+tt.timeout+"\"\n")
			cfg := loadNoRemote(t, p.root)

			// Act
			findings := p.run(Options{})
			infos := InspectScanners(cfg, p.root)

			// Assert
			require.Len(t, infos, 1)
			if tt.wantProb == "" {
				assert.Empty(t, ofCode(findings, CodeScannerConfigInvalid), dump(findings))
				assert.Equal(t, tt.wantTimout, infos[0].Timeout)
				return
			}
			invalid := ofCode(findings, CodeScannerConfigInvalid)
			require.NotEmpty(t, invalid, dump(findings))
			assert.Contains(t, invalid[0].Message, tt.wantProb)
		})
	}
}
