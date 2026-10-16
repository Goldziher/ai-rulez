// Command judgebench runs the judge benchmark: it copies the synthetic corpus to a throwaway git
// repository, runs `ai-rulez judge` over it with a real model and prints precision, recall, the
// false-positive rate of the traps, grounding and cost. It spends money: --max-cost is a hard cap.
//
// The corpus is synthetic. The default environment is the Armis Bedrock account used for the
// published numbers (AWS_PROFILE=default, AWS_REGION=eu-north-1); override both with the flags.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	judgebench "github.com/Goldziher/ai-rulez/v5/bench/judge"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "judgebench:", err)
		os.Exit(1)
	}
}

// options are the parsed command line.
type options struct {
	root, bin, profile, region, out, score, extra, ciscoBin, specBin string
	maxCost                                                          float64
	only                                                             map[string]bool
	compete, estimate                                                bool
}

func parseFlags() options {
	var (
		o    options
		only = flag.String("only", "", "comma-separated unit ids to run (default all)")
	)
	flag.StringVar(&o.root, "root", "bench/judge", "the bench/judge directory")
	flag.StringVar(&o.bin, "bin", "", "an ai-rulez binary; default: build ./cmd/ai-rulez")
	flag.Float64Var(&o.maxCost, "max-cost", 5, "hard cap on the spend of the whole run, in USD")
	flag.StringVar(&o.profile, "aws-profile", "default", "AWS_PROFILE for Bedrock")
	flag.StringVar(&o.region, "aws-region", "eu-north-1", "AWS_REGION for Bedrock")
	flag.StringVar(&o.out, "out", "", "directory for the raw reports (default: a temporary one)")
	flag.StringVar(&o.score, "score", "", "score an existing merged report instead of running")
	flag.StringVar(&o.extra, "judge-args", "", "extra arguments for ai-rulez judge, space separated (for example --model bedrock/...)")
	flag.BoolVar(&o.compete, "competitors", false, "also run Cisco skill-scanner and SkillSpector offline (free) over the same corpus and compare the skills they can see")
	flag.StringVar(&o.ciscoBin, "cisco-bin", "skill-scanner", "the Cisco skill-scanner binary")
	flag.StringVar(&o.specBin, "skillspector-bin", "skillspector", "the SkillSpector binary")
	flag.BoolVar(&o.estimate, "estimate", false, "run ai-rulez judge --estimate over the corpus and print the worst-case cost; sends nothing")
	flag.Parse()
	if *only != "" {
		o.only = map[string]bool{}
		for _, id := range strings.Split(*only, ",") {
			o.only[strings.TrimSpace(id)] = true
		}
	}
	return o
}

func run() error {
	o := parseFlags()
	m, err := judgebench.Load(o.root)
	if err != nil {
		return err
	}
	if o.score != "" {
		return scoreExisting(m, o)
	}
	binary, cleanup, err := resolveBinary(o.bin)
	if err != nil {
		return err
	}
	defer cleanup()
	outDir := o.out
	if outDir == "" {
		if outDir, err = os.MkdirTemp("", "judgebench-out-"); err != nil {
			return fmt.Errorf("make the output directory: %w", err)
		}
	}
	return runLive(m, o, binary, outDir)
}

func scoreExisting(m judgebench.Manifest, o options) error {
	f, err := os.Open(o.score) //nolint:gosec // the path the user named
	if err != nil {
		return fmt.Errorf("open the report: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only
	rep, err := judgebench.Decode(f)
	if err != nil {
		return err
	}
	if err := judgebench.Score(m, rep, o.only).Write(os.Stdout); err != nil {
		return err
	}
	if o.compete {
		return compareCompetitors(m, rep, o.only, o.ciscoBin, o.specBin, os.TempDir())
	}
	return nil
}

// resolveBinary returns the given binary or builds one into a temporary directory the cleanup removes.
func resolveBinary(bin string) (binary string, cleanup func(), err error) {
	if bin != "" {
		return bin, func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "judgebench-bin-")
	if err != nil {
		return "", nil, fmt.Errorf("make a directory for the binary: %w", err)
	}
	cleanup = func() { os.RemoveAll(tmp) } //nolint:errcheck,gosec // a temporary directory
	binary = filepath.Join(tmp, "ai-rulez")
	build := exec.Command("go", "build", "-o", binary, "./cmd/ai-rulez") //nolint:gosec // fixed arguments
	build.Stderr, build.Stdout = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("build ai-rulez: %w", err)
	}
	return binary, cleanup, nil
}

func runLive(m judgebench.Manifest, o options, binary, outDir string) error {
	opts := judgebench.RunOptions{
		Bin: binary, BaseEnv: os.Environ(), MaxCostUSD: o.maxCost, OutDir: outDir, Only: o.only, Log: os.Stderr,
		Env:       []string{"AWS_PROFILE=" + o.profile, "AWS_REGION=" + o.region},
		ExtraArgs: strings.Fields(o.extra),
	}
	if o.estimate {
		opts.ExtraArgs = append(opts.ExtraArgs, "--estimate")
	}
	rep, err := judgebench.RunLive(context.Background(), m, opts)
	if err != nil && (rep == nil || len(rep.Units) == 0) {
		return err
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "judgebench: stopped early:", err)
	}
	if o.estimate {
		e := rep.Run.Estimate
		fmt.Printf("estimate (nothing was sent): %d units, %d calls, worst case $%.2f, expected ~$%.2f\n", e.Units, e.Calls, e.CostUSD, e.ExpectedCostUSD)
		return nil
	}
	if werr := judgebench.WriteJSON(filepath.Join(outDir, "merged.json"), rep); werr != nil {
		return werr
	}
	if err := judgebench.Score(m, rep, o.only).Write(os.Stdout); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "raw reports: %s\n", outDir)
	if o.compete {
		return compareCompetitors(m, rep, o.only, o.ciscoBin, o.specBin, outDir)
	}
	return nil
}

// compareCompetitors scores ai-rulez, Cisco skill-scanner and SkillSpector on the units that hold a
// skill, the only ones the other two read.
func compareCompetitors(m judgebench.Manifest, ours *judgebench.Report, only map[string]bool, ciscoBin, specBin, outDir string) error {
	skills, err := judgebench.SkillUnits(m)
	if err != nil {
		return err
	}
	if only != nil { // compare on the units that were run
		for id := range skills {
			if !only[id] {
				delete(skills, id)
			}
		}
	}
	ctx := context.Background()
	cisco, cerr := judgebench.RunCisco(ctx, ciscoBin, m.CorpusDir(), outDir)
	spec, serr := judgebench.RunSkillSpector(ctx, specBin, m.CorpusDir(), outDir)
	fmt.Printf("\nhead to head on the %d units that hold a skill (security category; medium and above for the others, offline, no model):\n", len(skills))
	fmt.Println(judgebench.CompareHeader)
	fmt.Println(judgebench.Score(m, ours, skills).Row("ai-rulez judge"))
	if cerr == nil {
		fmt.Println(judgebench.Score(m, cisco, skills).Row("Cisco (static)"))
	} else {
		fmt.Fprintln(os.Stderr, "judgebench: cisco:", cerr)
	}
	if serr == nil {
		fmt.Println(judgebench.Score(m, spec, skills).Row("SkillSpector"))
	} else {
		fmt.Fprintln(os.Stderr, "judgebench: skillspector:", serr)
	}
	return nil
}
