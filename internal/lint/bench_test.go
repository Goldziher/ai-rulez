package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func benchProject(b *testing.B, files int) (*config.Config, *Tree) {
	b.Helper()
	root := testutil.BuildBenchTree(b, files, testutil.BenchTreeOptions{Git: true})
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		b.Fatal(err)
	}
	tree, err := LoadTree(root)
	if err != nil {
		b.Fatal(err)
	}
	return cfg, tree
}

// BenchmarkLoadTree indexes the git-tracked files of the synthetic project.
func BenchmarkLoadTree(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		root := testutil.BuildBenchTree(b, s.Files, testutil.BenchTreeOptions{Git: true})
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := LoadTree(root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRunFull runs every analyzer over the synthetic project.
func BenchmarkRunFull(b *testing.B) {
	for _, s := range testutil.BenchSizes() {
		cfg, tree := benchProject(b, s.Files)
		b.Run(s.Name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := Run(cfg, tree); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkAnalyzers runs each analyzer alone over the medium project, to show
// which one dominates a full run.
func BenchmarkAnalyzers(b *testing.B) {
	cfg, tree := benchProject(b, 200)
	for _, name := range AnalyzerNames() {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := RunWith(cfg, tree, Options{Analyzers: []string{name}}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// benchSecretText is prose with a few embedded credentials.
func benchSecretText(lines int, withSecrets bool) string {
	text := testutil.BenchBody(lines)
	if withSecrets {
		text += "aws_access_key_id = AKIAIOSFODNN7EXAMPLE\n" +
			"token: ghp_0123456789abcdefghijklmnopqrstuvwxyzAB\n" +
			"password = \"Sup3rS3cretValue99xyz\"\n" +
			"-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----\n"
	}
	return text
}

// BenchmarkSecretScan times the credential patterns over clean and dirty text.
func BenchmarkSecretScan(b *testing.B) {
	for _, withSecrets := range []bool{false, true} {
		kind := "clean"
		if withSecrets {
			kind = "dirty"
		}
		for _, lines := range []int{20, 400} {
			text := benchSecretText(lines, withSecrets)
			name := kind + "/" + map[int]string{20: "small", 400: "large"}[lines]
			b.Run("DetectSecret/"+name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				for range b.N {
					DetectSecret(text)
				}
			})
			b.Run("RedactSecrets/"+name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				for range b.N {
					_ = RedactSecrets(text)
				}
			})
			b.Run("ScanText/"+name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				for range b.N {
					_ = ScanText("a.md", text)
				}
			})
		}
	}
}

// BenchmarkSecretScanLongLine feeds the patterns one very long line.
func BenchmarkSecretScanLongLine(b *testing.B) {
	text := strings.Repeat("abcdefghij0123456789 ", 20000)
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	for range b.N {
		DetectSecret(text)
	}
}
