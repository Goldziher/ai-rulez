package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const benchPluginConfig = `
[plugin]
name = "bench"
description = "Benchmark skills."
version = "1.4.0"
repository = "https://github.com/acme/skills"
runtimes = ["claude", "agent-plugins"]

[plugin.author]
name = "Jane"
`

const benchARDConfig = `
[ard]
publisher = "acme.test"
namespace = "conventions"

[ard.queries]
docs = ["search the acme docs", "find the api reference"]
`

// benchChdir switches into dir for the benchmark.
func benchChdir(b *testing.B, dir string) {
	b.Helper()
	prev, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = os.Chdir(prev) })
}

// quiet sends stdout and stderr to the null device while fn runs, so the
// command output does not drown the benchmark lines.
func quiet(b *testing.B, fn func()) {
	b.Helper()
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer null.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = null, null
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	fn()
}

func benchCommandProject(b *testing.B, files int, extra string, generate bool) string {
	b.Helper()
	b.Setenv("HOME", b.TempDir())
	b.Setenv("SOURCE_DATE_EPOCH", "1790000000")
	cliLockPolicy.Mode, cliLockPolicy.Offline = includes.LockAuto, false
	root := testutil.BuildBenchTree(b, files, testutil.BenchTreeOptions{Git: true, ExtraConfig: extra, Presets: []string{"claude"}})
	benchChdir(b, root)
	if generate {
		quiet(b, func() {
			if code := runRecursiveGenerate(); code != 0 {
				b.Fatalf("generate exited %d", code)
			}
			cfg, err := config.LoadConfig(context.Background(), ".", config.WithoutLocal())
			if err != nil {
				b.Fatal(err)
			}
			if err := generator.NewGenerator(cfg).GeneratePlugin(""); err != nil {
				b.Fatal(err)
			}
		})
	}
	return root
}

// BenchmarkLock writes the lock (digesting every authored item) and checks it.
func BenchmarkLock(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		b.Run("write/"+s.Name, func(b *testing.B) {
			benchCommandProject(b, s.Files, "", false)
			b.ReportAllocs()
			quiet(b, func() {
				for range b.N {
					if code := writeLockAt("", "", nil); code != 0 {
						b.Fatalf("lock exited %d", code)
					}
				}
			})
		})
		b.Run("check/"+s.Name, func(b *testing.B) {
			benchCommandProject(b, s.Files, "", false)
			quiet(b, func() {
				if code := writeLockAt("", "", nil); code != 0 {
					b.Fatalf("lock exited %d", code)
				}
			})
			b.ReportAllocs()
			b.ResetTimer()
			quiet(b, func() {
				for range b.N {
					if code := checkLockAt(""); code != 0 {
						b.Fatalf("lock --check exited %d", code)
					}
				}
			})
		})
	}
}

// BenchmarkPublishDryRun builds the release in memory for each emitter set.
func BenchmarkPublishDryRun(b *testing.B) {
	for _, tc := range []struct {
		name  string
		emit  []string
		extra string
	}{
		{"bundle", nil, benchPluginConfig},
		{"agent-plugins", []string{"agent-plugins"}, benchPluginConfig},
		{"ard", []string{"ard"}, benchPluginConfig + benchARDConfig},
	} {
		for _, s := range testutil.BenchSizes() {
			b.Run(tc.name+"/"+s.Name, func(b *testing.B) {
				root := benchCommandProject(b, s.Files, tc.extra, true)
				quiet(b, func() {
					if code := writeLockAt("", "", nil); code != 0 {
						b.Fatalf("lock exited %d", code)
					}
				})
				for _, args := range [][]string{{"add", "-A"}, {"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "bench"}} {
					testutil.Git(b, root, args...)
				}
				resetPublishBenchFlags()
				publishDryRun, publishEmit = true, tc.emit
				b.Cleanup(resetPublishBenchFlags)
				b.ReportAllocs()
				b.ResetTimer()
				quiet(b, func() {
					for range b.N {
						var out bytes.Buffer
						if err := runPublish(context.Background(), &out); err != nil {
							b.Fatal(err)
						}
					}
				})
			})
		}
	}
}

func resetPublishBenchFlags() {
	publishTo, publishDist, publishTag, publishRepo, publishFormat = "", "dist", "", "", ""
	publishDryRun, publishExecute, publishYes, publishForce, publishAllowDirty = false, false, false, false, false
	publishEmit = nil
}

// BenchmarkOKF exports the project as an OKF bundle (write, then the no-op
// check) and validates the bundle it wrote.
func BenchmarkOKF(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		b.Run("export/"+s.Name, func(b *testing.B) {
			benchCommandProject(b, s.Files, "", false)
			okfCheck = false
			b.ReportAllocs()
			b.ResetTimer()
			quiet(b, func() {
				for range b.N {
					var out bytes.Buffer
					if code := runOKFExport(context.Background(), nil, &out); code != 0 {
						b.Fatalf("okf export exited %d: %s", code, out.String())
					}
				}
			})
		})
		b.Run("validate/"+s.Name, func(b *testing.B) {
			root := benchCommandProject(b, s.Files, "", false)
			var out bytes.Buffer
			if code := runOKFExport(context.Background(), nil, &out); code != 0 {
				b.Fatalf("okf export exited %d: %s", code, out.String())
			}
			bundle := filepath.Join(root, "docs", "okf")
			okfFormat, okfFailOn = "", "none"
			b.Cleanup(func() { okfFormat, okfFailOn = "", "error" })
			b.ReportAllocs()
			b.ResetTimer()
			quiet(b, func() {
				for range b.N {
					out.Reset()
					if code := runOKFValidate(context.Background(), bundle, &out); code != 0 {
						b.Fatalf("okf validate exited %d: %s", code, out.String())
					}
				}
			})
		})
	}
}
