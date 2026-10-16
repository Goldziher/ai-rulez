// Command testshard compiles one Go test package and runs exhaustive partitions
// in separate processes, preserving isolation for tests that change cwd or globals.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

const maxPatternBytes = 24000 // Leave room below Windows' 32767-character command line limit.

type options struct {
	shards                int
	timeout, buildTimeout time.Duration
	logDir, target        string
	patterns              []string
}

func main() {
	o, err := parseOptions(os.Args[1:])
	if err == nil {
		err = run(context.Background(), o, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "testshard:", err)
		os.Exit(1)
	}
}

func parseOptions(args []string) (options, error) {
	var o options
	f := flag.NewFlagSet("testshard", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.IntVar(&o.shards, "shards", 4, "number of separate test processes (1-64)")
	f.DurationVar(&o.timeout, "timeout", 45*time.Minute, "timeout of each test partition")
	f.DurationVar(&o.buildTimeout, "build-timeout", 30*time.Minute, "timeout for package discovery and compilation")
	f.StringVar(&o.logDir, "log-dir", "", "directory to retain per-shard logs (default: temporary logs cleaned after the run)")
	f.StringVar(&o.target, "package", "./cmd/commands", "package to split; the remaining packages run through ordinary go test")
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if f.NArg() == 0 || strings.TrimSpace(o.target) == "" || strings.HasPrefix(o.target, "-") {
		return o, fmt.Errorf("package patterns are required (for example ./cmd/... and ./internal/...)")
	}
	o.patterns = f.Args()
	for _, p := range o.patterns {
		if strings.TrimSpace(p) == "" || strings.HasPrefix(p, "-") {
			return o, fmt.Errorf("invalid package pattern %q", p)
		}
	}
	if o.shards < 1 || o.shards > 64 || o.timeout <= 0 || o.buildTimeout <= 0 || (o.logDir != "" && strings.TrimSpace(o.logDir) == "") {
		return o, fmt.Errorf("shards must be 1-64, timeouts positive, and log-dir empty or a path")
	}
	return o, nil
}

// partition rejects malformed or duplicate names instead of silently losing coverage.
func partition(names []string, count int) ([][]string, error) {
	if len(names) == 0 || count < 1 || count > 64 {
		return nil, fmt.Errorf("need test names and 1-64 shards")
	}
	names = slices.Clone(names)
	slices.Sort(names)
	groups := make([][]string, min(count, len(names)))
	for i, name := range names {
		if !testName(name) || (i > 0 && names[i-1] == name) {
			return nil, fmt.Errorf("invalid or duplicate test name %q", name)
		}
		groups[i%len(groups)] = append(groups[i%len(groups)], name)
	}
	for _, group := range groups {
		if len(pattern(group)) > maxPatternBytes {
			return nil, fmt.Errorf("partition regex exceeds %d bytes; increase -shards", maxPatternBytes)
		}
	}
	return groups, verifyPartitions(names, groups)
}

func testName(name string) bool {
	return (strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Fuzz") || strings.HasPrefix(name, "Example")) &&
		!strings.ContainsAny(name, "/\x00") && !strings.ContainsFunc(name, unicode.IsSpace)
}

func verifyPartitions(names []string, groups [][]string) error {
	counts := make(map[string]int, len(names))
	for _, name := range names {
		counts[name] = 1
	}
	for _, group := range groups {
		if len(group) == 0 {
			return fmt.Errorf("empty test partition")
		}
		for _, name := range group {
			if counts[name] != 1 {
				return fmt.Errorf("test %q is duplicated or was not enumerated", name)
			}
			counts[name]--
		}
	}
	for name, count := range counts {
		if count != 0 {
			return fmt.Errorf("test %q was not assigned", name)
		}
	}
	return nil
}

func pattern(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

func listedNames(data []byte) ([]string, error) {
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "Benchmark") {
			continue // Benchmarks do not run in an ordinary go test invocation.
		}
		if !testName(line) {
			return nil, fmt.Errorf("unexpected test enumeration output %q", line)
		}
		names = append(names, line)
	}
	return names, nil
}

func command(ctx context.Context, dir, executable string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // fixed Go commands or the test binary just compiled
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second // Descendants must not hold inherited output pipes indefinitely.
	return cmd
}

func run(ctx context.Context, o options, out io.Writer) error {
	out = &lockedWriter{w: out}
	fmt.Fprintln(out, "testshard: resolving requested packages and target") //nolint:errcheck // progress
	listCtx, stopList := context.WithTimeout(ctx, o.buildTimeout)
	targets, err := resolvePackages(listCtx, []string{o.target})
	if err != nil {
		stopList()
		return err
	}
	packages, err := resolvePackages(listCtx, o.patterns)
	stopList()
	if err != nil {
		return err
	}
	pkg, rest, err := selectPackages(targets, packages)
	if err != nil {
		return err
	}
	// Warm shared build objects before scheduling both suites, avoiding duplicate cold builds.
	binary, groups, cleanup, prepErr := prepareTarget(ctx, o, pkg, out)
	defer cleanup()
	return runConcurrent(ctx, func(ctx context.Context) error {
		if len(rest) == 0 {
			return nil
		}
		return runRest(ctx, o, rest, out)
	}, func(ctx context.Context) error {
		if prepErr != nil {
			return prepErr
		}
		shardOpts := o
		if shardOpts.logDir == "" {
			shardOpts.logDir = filepath.Join(filepath.Dir(binary), "logs")
		}
		if err := os.MkdirAll(shardOpts.logDir, 0o755); err != nil {
			return err
		}
		fmt.Fprintf(out, "testshard: %d tests, %d processes; logs: %s\n", countNames(groups), len(groups), shardOpts.logDir) //nolint:errcheck // progress
		return runGroups(ctx, shardOpts, pkg.Dir, binary, groups, out)
	})
}

func runRest(ctx context.Context, o options, packages []string, out io.Writer) error {
	fmt.Fprintf(out, "testshard: ordinary go test on %d remaining packages (cache enabled)\n", len(packages)) //nolint:errcheck // progress
	ctx, stop := context.WithTimeout(ctx, o.buildTimeout+2*o.timeout+time.Minute)
	defer stop()
	cmd := command(ctx, "", "go", append([]string{"test", "-timeout", o.timeout.String()}, packages...)...)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remaining packages: %w", err)
	}
	return nil
}

func prepareTarget(ctx context.Context, o options, pkg goPackage, out io.Writer) (binary string, groups [][]string, cleanup func(), prepErr error) {
	cleanup = func() {}
	tmp, err := os.MkdirTemp("", "go-testshard-")
	if err != nil {
		return "", nil, cleanup, err
	}
	cleanup = func() {
		_ = os.RemoveAll(tmp) //nolint:errcheck // own temporary compiled binary
	}
	binary = filepath.Join(tmp, "package.test")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	fmt.Fprintf(out, "testshard: compiling %s\n", pkg.ImportPath) //nolint:errcheck // progress
	buildCtx, cancel := context.WithTimeout(ctx, o.buildTimeout)
	defer cancel()
	build := command(buildCtx, pkg.Dir, "go", "test", "-c", "-o", binary, pkg.ImportPath)
	build.Stdout, build.Stderr = out, out
	if err := build.Run(); err != nil {
		return "", nil, cleanup, fmt.Errorf("compile test binary: %w", err)
	}
	fmt.Fprintln(out, "testshard: enumerating compiled test names") //nolint:errcheck // progress
	listCtx, stop := context.WithTimeout(ctx, time.Minute)
	defer stop()
	data, err := command(listCtx, pkg.Dir, binary, "-test.list=.").Output()
	if err != nil {
		return "", nil, cleanup, fmt.Errorf("enumerate tests: %w", err)
	}
	names, err := listedNames(data)
	if err != nil {
		return "", nil, cleanup, err
	}
	groups, err = partition(names, o.shards)
	if err != nil {
		return "", nil, cleanup, err
	}
	return binary, groups, cleanup, nil
}

func countNames(groups [][]string) int {
	n := 0
	for _, group := range groups {
		n += len(group)
	}
	return n
}

type goPackage struct{ Dir, ImportPath string }

func resolvePackages(ctx context.Context, patterns []string) ([]goPackage, error) {
	var stderr bytes.Buffer
	cmd := command(ctx, "", "go", append([]string{"list", "-json"}, patterns...)...)
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("locate packages: %w: %s", err, stderr.String())
	}
	var packages []goPackage
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var p goPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("read package metadata: %w", err)
		}
		if p.Dir == "" || p.ImportPath == "" {
			return nil, fmt.Errorf("incomplete package metadata")
		}
		packages = append(packages, p)
	}
	return packages, nil
}

func selectPackages(targets, packages []goPackage) (goPackage, []string, error) {
	if len(targets) != 1 {
		return goPackage{}, nil, fmt.Errorf("-package must resolve to exactly one Go package")
	}
	target := targets[0]
	found := false
	seen := map[string]bool{}
	var rest []string
	for _, pkg := range packages {
		if pkg.ImportPath == target.ImportPath {
			found = true
		} else if !seen[pkg.ImportPath] {
			rest = append(rest, pkg.ImportPath)
		}
		seen[pkg.ImportPath] = true
	}
	if !found {
		return target, nil, fmt.Errorf("target %s is not among the requested package patterns", target.ImportPath)
	}
	return target, rest, nil
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func runConcurrent(ctx context.Context, rest, shards func(context.Context) error) error {
	restDone := make(chan error, 1)
	go func() { restDone <- rest(ctx) }()
	shardErr := shards(ctx)
	return errors.Join(<-restDone, shardErr)
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func runGroups(ctx context.Context, o options, dir, binary string, groups [][]string, out io.Writer) error {
	stream := &lockedWriter{w: out}
	results := make(chan error, len(groups))
	for i, group := range groups {
		go func() {
			log, err := os.OpenFile(filepath.Join(o.logDir, fmt.Sprintf("shard-%02d.log", i+1)), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				results <- err
				return
			}
			writer := io.MultiWriter(stream, log)
			fmt.Fprintf(writer, "testshard: shard %d starts (%d tests)\n", i+1, len(group)) //nolint:errcheck // progress
			childCtx, cancel := context.WithTimeout(ctx, o.timeout+time.Minute)
			defer cancel()
			cmd := command(childCtx, dir, binary, "-test.run="+pattern(group), "-test.count=1", "-test.timeout="+o.timeout.String(), "-test.v")
			cmd.Stdout, cmd.Stderr = writer, writer
			err = cmd.Run()
			fmt.Fprintf(writer, "testshard: shard %d finished: %v\n", i+1, err) //nolint:errcheck // progress
			err = errors.Join(err, log.Close())
			if err != nil {
				err = fmt.Errorf("shard %d: %w", i+1, err)
			}
			results <- err
		}()
	}
	var failures []error
	for range groups {
		if err := <-results; err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
