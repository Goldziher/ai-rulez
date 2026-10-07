package verifiers

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/samber/oops"
)

// ExampleResult is the outcome of one self-test example.
type ExampleResult struct {
	Verifier string `json:"verifier"`
	Example  string `json:"example"`
	Want     string `json:"want"`
	Got      Status `json:"got"`
	OK       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
}

// TestReport is the outcome of `verifiers test`.
type TestReport struct {
	Results []ExampleResult `json:"results"`
	// Untested lists verifiers declared without examples.
	Untested []string `json:"untested,omitempty"`
	// Problems are declarations that could not be loaded (AR9H2).
	Problems []Problem `json:"-"`
}

// Failed reports whether an example did not produce its expected outcome or a
// declaration was invalid.
func (t *TestReport) Failed() bool {
	for _, r := range t.Results {
		if !r.OK {
			return true
		}
	}
	return len(t.Problems) > 0
}

// RunExamples evaluates each verifier's examples offline: every example gets a
// temporary directory holding its synthetic files, in which the listed
// `changed` files count as entirely added. Git and the real project are never
// touched. names restricts the run; an unknown name is an error.
func RunExamples(ctx context.Context, cfg *config.Config, names []string) (*TestReport, error) {
	return RunExamplesWith(ctx, cfg, names, Options{})
}

// RunExamplesWith is RunExamples with run options: AllowExec lets the command
// predicates of the examples run (in the example's temporary directory),
// Runner and Environ replace the process starter and environment.
func RunExamplesWith(ctx context.Context, cfg *config.Config, names []string, opts Options) (*TestReport, error) {
	specs, problems := LoadSpecs(cfg)
	rep := &TestReport{Results: []ExampleResult{}, Problems: problems}
	picked := specs
	if len(names) > 0 {
		want := map[string]bool{}
		for _, n := range names {
			want[n] = true
		}
		picked = nil
		for i := range specs {
			if sp := &specs[i]; want[sp.ID] {
				picked = append(picked, *sp)
				delete(want, sp.ID)
			}
		}
		if len(want) > 0 {
			missing := make([]string, 0, len(want))
			for n := range want {
				missing = append(missing, n)
			}
			sort.Strings(missing)
			return nil, oops.Hint("Self-tests exist only for verifiers declared under .ai-rulez/verifiers/. Run `ai-rulez verifiers list`.").
				Errorf("unknown verifier(s): %v", missing)
		}
	}
	for i := range picked {
		sp := &picked[i]
		if len(sp.Examples) == 0 {
			rep.Untested = append(rep.Untested, sp.ID)
			continue
		}
		for _, ex := range sp.Examples {
			rep.Results = append(rep.Results, runExample(ctx, cfg, sp, ex, opts))
		}
	}
	return rep, nil
}

func runExample(ctx context.Context, cfg *config.Config, sp *Spec, ex Example, opts Options) ExampleResult {
	out := ExampleResult{Verifier: sp.ID, Example: ex.Name, Want: ex.Expect}
	if usesLLM(sp.Require) && opts.LLM == nil {
		// An example cannot say what a model will answer; it needs a real run.
		out.Got, out.OK, out.Message = StatusSkipped, true, "the llm predicate needs a model: not run offline"
		return out
	}
	dir, err := os.MkdirTemp("", "ai-rulez-verifier-test-")
	if err != nil {
		out.Got, out.Message = StatusError, err.Error()
		return out
	}
	defer os.RemoveAll(dir) //nolint:errcheck // throwaway directory
	tree := make([]string, 0, len(ex.Files))
	for name, content := range ex.Files {
		rel, err := cleanRel(name)
		if err != nil {
			out.Got, out.Message = StatusError, err.Error()
			return out
		}
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			out.Got, out.Message = StatusError, err.Error()
			return out
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			out.Got, out.Message = StatusError, err.Error()
			return out
		}
		tree = append(tree, filepath.ToSlash(rel))
	}
	sort.Strings(tree)
	changes := make([]gitutil.Change, 0, len(ex.Changed))
	for _, c := range ex.Changed {
		changes = append(changes, gitutil.Change{Path: filepath.ToSlash(filepath.Clean(c)), Status: 'A', AllAdded: true})
	}
	opts.Since, opts.Staged = "", false
	env := &Env{Cfg: cfg, Root: dir, opts: opts}
	env.scope = newScope(ModeAll, "", changes, tree)
	res := evaluateSpec(ctx, env, sp)
	out.Got, out.Message = res.Status, res.Message
	out.OK = string(res.Status) == ex.Expect
	return out
}
