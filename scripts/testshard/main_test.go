package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPartitionIsExhaustiveDisjointAndPreservesKinds(t *testing.T) {
	names := []string{"TestZ", "Example", "FuzzSeed", "TestA", "ExampleThing"}
	groups, err := partition(names, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPartitions(names, groups); err != nil {
		t.Fatal(err)
	}
	var all []string
	for _, group := range groups {
		match := regexp.MustCompile(pattern(group))
		for _, name := range names {
			if match.MatchString(name) != slices.Contains(group, name) || match.MatchString(name+"Suffix") {
				t.Fatalf("partition regex %q does not match its exact names", match)
			}
		}
		all = append(all, group...)
	}
	slices.Sort(all)
	want := slices.Clone(names)
	slices.Sort(want)
	if !slices.Equal(want, all) {
		t.Fatalf("lost coverage: %v versus %v", want, all)
	}
	if names[0] != "TestZ" {
		t.Fatal("partition mutated the caller's enumeration")
	}
}

func TestPartitionRejectsInvalidInputs(t *testing.T) {
	for _, names := range [][]string{nil, {"TestA", "TestA"}, {"BenchmarkA"}, {""}, {"TestA/subtest"}, {"TestA\nTestB"}} {
		if _, err := partition(names, 4); err == nil {
			t.Fatalf("accepted invalid names %q", names)
		}
	}
	for _, count := range []int{0, -1, 65} {
		if _, err := partition([]string{"TestA"}, count); err == nil {
			t.Fatalf("accepted shard count %d", count)
		}
	}
	if _, err := partition([]string{"Test" + strings.Repeat("A", maxPatternBytes)}, 1); err == nil {
		t.Fatal("accepted a regex exceeding Windows command line budget")
	}
	groups, err := partition([]string{"TestA"}, 4)
	if err != nil || len(groups) != 1 {
		t.Fatalf("small suites should produce no empty partitions: %v %v", groups, err)
	}
}

func TestPartitionVerificationRejectsOmissionsAndDuplicates(t *testing.T) {
	for _, groups := range [][][]string{{{"TestA"}}, {{"TestA"}, {"TestA", "TestB"}}, {{"TestA"}, {"TestB", "TestExtra"}}, {{"TestA", "TestB"}, {}}} {
		if err := verifyPartitions([]string{"TestA", "TestB"}, groups); err == nil {
			t.Fatalf("accepted incomplete or duplicate coverage %v", groups)
		}
	}
}

func TestEnumerationPreservesTestsExamplesAndFuzzSeeds(t *testing.T) {
	names, err := listedNames([]byte("TestOne\r\nBenchmarkOne\r\nExampleThing\r\nFuzzInputs\r\n"))
	if err != nil || !slices.Equal(names, []string{"TestOne", "ExampleThing", "FuzzInputs"}) {
		t.Fatalf("enumeration = %v, %v", names, err)
	}
	for _, data := range []string{"", "unexpected init output\nTestOne", "TestOne\n\nTestTwo"} {
		if _, err := listedNames([]byte(data)); err == nil {
			t.Fatalf("accepted invalid enumeration %q", data)
		}
	}
}

func TestOptionsRejectInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"-shards", "0", "./cmd/..."}, {"-shards", "65", "./cmd/..."}, {"-timeout", "0", "./cmd/..."}, {"-build-timeout", "-1s", "./cmd/..."}, {"-log-dir", " ", "./cmd/..."}, {"-package", "", "./cmd/..."}, {"-unknown"}, {""}, {"./cmd/...", "-oops"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatalf("accepted invalid arguments %q", args)
		}
	}
	o, err := parseOptions([]string{"-shards", "2", "-timeout", "10m", "./cmd/...", "./internal/..."})
	if err != nil || o.shards != 2 || o.timeout != 10*time.Minute || o.target != "./cmd/commands" || len(o.patterns) != 2 {
		t.Fatalf("options = %+v, %v", o, err)
	}
	if o.logDir != "" {
		t.Fatal("default logs must belong to the temporary binary directory")
	}
	if _, err := parseOptions([]string{"-log-dir", "", "./cmd/commands"}); err != nil {
		t.Fatalf("an unset Windows log-directory environment value must work: %v", err)
	}
}

func TestPackageSelectionPreservesEveryRequestedPackageOnce(t *testing.T) {
	target := goPackage{Dir: "/repo/cmd", ImportPath: "example/cmd"}
	_, rest, err := selectPackages([]goPackage{target}, []goPackage{{ImportPath: "example/a"}, target, {ImportPath: "example/a"}, {ImportPath: "example/b"}})
	if err != nil || !slices.Equal(rest, []string{"example/a", "example/b"}) {
		t.Fatalf("rest = %v, %v", rest, err)
	}
	if _, _, err := selectPackages([]goPackage{target}, []goPackage{{ImportPath: "example/a"}}); err == nil {
		t.Fatal("accepted a target outside the requested suite")
	}
	if _, _, err := selectPackages([]goPackage{target, target}, nil); err == nil {
		t.Fatal("accepted multiple sharding targets")
	}
}

func TestRunGroupsRunsAllPartitionsAndPropagatesFailure(t *testing.T) {
	t.Setenv("AI_RULEZ_TESTSHARD_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	o := options{timeout: time.Minute, logDir: t.TempDir()}
	var out bytes.Buffer
	err = runGroups(t.Context(), o, dir, exe, [][]string{{"TestShardChildSuccess"}, {"TestShardChildFailure"}}, &out)
	if err == nil || !strings.Contains(err.Error(), "shard 2") {
		t.Fatalf("missing failure: %v", err)
	}
	for _, name := range []string{"shard-01.log", "shard-02.log"} {
		data, err := os.ReadFile(filepath.Join(o.logDir, name))
		if err != nil || !bytes.Contains(data, []byte("finished")) {
			t.Fatalf("partition did not finish/log: %s %v", data, err)
		}
		if err := os.Remove(filepath.Join(o.logDir, name)); err != nil {
			t.Fatalf("a completed shard left its log open: %v", err)
		}
	}
	if !strings.Contains(out.String(), "child success") {
		t.Fatal("test output was not streamed")
	}
}

func TestRunGroupsHonorsExternalCancellation(t *testing.T) {
	t.Setenv("AI_RULEZ_TESTSHARD_HELPER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = runGroups(ctx, options{timeout: time.Minute, logDir: t.TempDir()}, t.TempDir(), exe, [][]string{{"TestShardChildWait"}}, &bytes.Buffer{})
	if err == nil || time.Since(started) > 5*time.Second {
		t.Fatalf("external cancellation was not bounded: %v", err)
	}
}

func TestShardChildSuccess(t *testing.T) {
	if os.Getenv("AI_RULEZ_TESTSHARD_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	if _, err := os.Stat("marker"); err != nil {
		t.Fatal("test ran outside the package working directory:", err)
	}
	t.Log("child success")
}

func TestShardChildFailure(t *testing.T) {
	if os.Getenv("AI_RULEZ_TESTSHARD_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	t.Error("intentional child failure")
}

func TestShardChildWait(t *testing.T) {
	if os.Getenv("AI_RULEZ_TESTSHARD_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	time.Sleep(10 * time.Second)
}
