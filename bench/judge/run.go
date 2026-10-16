package judgebench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// maxPathsPerRun keeps a batch under the unit limit of one judge run.
const maxPathsPerRun = 150

// RunOptions configure a live run of the corpus through `ai-rulez judge`.
type RunOptions struct {
	// Bin is the ai-rulez binary.
	Bin string
	// BaseEnv is the environment the judge process starts from; the caller owns reading the process environment.
	BaseEnv []string
	// Env is added to BaseEnv (AWS_PROFILE, AWS_REGION, ...).
	Env []string
	// MaxCostUSD is the hard cap of the whole run; each batch gets what is left.
	MaxCostUSD float64
	// ExtraArgs are passed to every judge call (--model, --adjudicator-model, --no-cache, ...).
	ExtraArgs []string
	// OutDir, when set, receives the raw JSON report of each batch.
	OutDir string
	// Only limits the run to these unit ids (nil runs all).
	Only map[string]bool
	// Log receives progress lines.
	Log io.Writer
}

// Prepare copies the corpus to a fresh git repository, so the judge sees a project whose root is the
// corpus (ancestor conventions stop there) and never the repository the benchmark lives in.
func Prepare(m Manifest) (dir string, err error) {
	dir, err = os.MkdirTemp("", "judgebench-")
	if err != nil {
		return "", fmt.Errorf("make a work directory: %w", err)
	}
	src := m.CorpusDir()
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755) //nolint:gosec // a temporary copy
		}
		data, rerr := os.ReadFile(p) //nolint:gosec // the repository's own corpus
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		return dir, fmt.Errorf("copy the corpus: %w", err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.name=bench", "-c", "user.email=bench@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "corpus"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // fixed git arguments in a temporary directory
		if out, cerr := cmd.CombinedOutput(); cerr != nil {
			return dir, fmt.Errorf("git %s: %w\n%s", args[0], cerr, out)
		}
	}
	return dir, nil
}

// batch is one judge invocation.
type batch struct {
	name  string
	paths []string
	extra []string
}

func plan(m Manifest, opts RunOptions, rubricsDir string) []batch {
	var plain, rubric []string
	for i := range m.Units {
		u := &m.Units[i]
		if opts.Only != nil && !opts.Only[u.ID] {
			continue
		}
		if u.RubricSkill != "" {
			rubric = append(rubric, u.Path())
		} else {
			plain = append(plain, u.Path())
		}
	}
	var out []batch
	for i := 0; i < len(plain); i += maxPathsPerRun {
		out = append(out, batch{name: "main-" + strconv.Itoa(i/maxPathsPerRun+1), paths: plain[i:min(i+maxPathsPerRun, len(plain))]})
	}
	if len(rubric) > 0 {
		out = append(out, batch{name: "rubric", paths: rubric, extra: []string{"--skill", "path:" + filepath.Join(rubricsDir, "acme-skill-standards")}})
	}
	return out
}

// RunLive runs the corpus through the judge in batches and returns the merged report.
func RunLive(ctx context.Context, m Manifest, opts RunOptions) (*Report, error) {
	work, err := Prepare(m)
	defer os.RemoveAll(work) //nolint:errcheck // a temporary directory
	if err != nil {
		return nil, err
	}
	merged := &Report{}
	remaining := opts.MaxCostUSD
	// The judge runs in the throwaway repository, so the rubric is named by an absolute path.
	rubrics, err := filepath.Abs(filepath.Join(m.Root, "rubrics"))
	if err != nil {
		return nil, fmt.Errorf("resolve the rubrics directory: %w", err)
	}
	for _, b := range plan(m, opts, rubrics) {
		if remaining <= 0 {
			return merged, fmt.Errorf("the spend cap of $%.2f is used up before batch %s", opts.MaxCostUSD, b.name)
		}
		args := append([]string{"judge"}, b.paths...)
		args = append(args, "--allow-llm", "--format", "json", "--explain-drops", "--allow-partial", "--max-cost", strconv.FormatFloat(remaining, 'f', 4, 64))
		args = append(args, opts.ExtraArgs...)
		args = append(args, b.extra...)
		cmd := exec.CommandContext(ctx, opts.Bin, args...) //nolint:gosec // the benchmark runs the binary it was given
		cmd.Dir = work
		cmd.Env = append(slices.Clone(opts.BaseEnv), opts.Env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if opts.Log != nil {
			fmt.Fprintf(opts.Log, "batch %s: %d paths, $%.2f left\n", b.name, len(b.paths), remaining) //nolint:errcheck // a progress line that cannot be written must not fail the run
		}
		runErr := cmd.Run()
		if opts.OutDir != "" {
			_ = os.MkdirAll(opts.OutDir, 0o755)                                                           //nolint:errcheck,gosec // best effort
			_ = os.WriteFile(filepath.Join(opts.OutDir, "report-"+b.name+".json"), stdout.Bytes(), 0o600) //nolint:errcheck // best effort
			_ = os.WriteFile(filepath.Join(opts.OutDir, "stderr-"+b.name+".txt"), stderr.Bytes(), 0o600)  //nolint:errcheck // best effort
		}
		rep, derr := Decode(bytes.NewReader(stdout.Bytes()))
		if derr != nil {
			return merged, fmt.Errorf("batch %s: %w (judge: %s)\n%s", b.name, derr, fmt.Sprint(runErr), stderr.String())
		}
		remaining -= rep.Run.Provenance.CostUSD
		merged.merge(rep)
	}
	return merged, nil
}

// merge adds the units and totals of another report.
func (r *Report) merge(o *Report) {
	r.Units = append(r.Units, o.Units...)
	r.Run.Models = o.Run.Models
	r.Run.Estimate.Units += o.Run.Estimate.Units
	r.Run.Estimate.Calls += o.Run.Estimate.Calls
	r.Run.Estimate.CostUSD += o.Run.Estimate.CostUSD
	r.Run.Estimate.ExpectedCostUSD += o.Run.Estimate.ExpectedCostUSD
	p, q := &r.Run.Provenance, o.Run.Provenance
	p.Calls += q.Calls
	p.CacheHits += q.CacheHits
	p.InputTokens += q.InputTokens
	p.OutputTokens += q.OutputTokens
	p.CostUSD += q.CostUSD
	g, h := &r.Summary.Grounding, o.Summary.Grounding
	g.Candidates += h.Candidates
	g.QuoteVerified += h.QuoteVerified
	g.DroppedUngrounded += h.DroppedUngrounded
	g.DroppedByAdjud += h.DroppedByAdjud
	g.Repaired += h.Repaired
	g.BelowMinConfidence += h.BelowMinConfidence
	if r.Summary.Decision == "" || o.Summary.Decision == "fail" || (o.Summary.Decision == "incomplete" && r.Summary.Decision == statusPass) {
		r.Summary.Decision = o.Summary.Decision
	}
}

// WriteJSON stores a merged report.
func WriteJSON(path string, rep *Report) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the report: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// Names joins unit ids for a log line.
func Names(units []Unit) string {
	ids := make([]string, len(units))
	for i := range units {
		ids[i] = units[i].ID
	}
	return strings.Join(ids, ",")
}
