package commands

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// chdir switches the working directory for the duration of the test and
// restores it on cleanup. Required because findConfigFilesRecursively walks
// from "." rather than taking a root path.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestFindConfigFilesRecursively(t *testing.T) {
	root := t.TempDir()

	// Real configs that should be discovered.
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"root\"\n")
	writeFile(t, filepath.Join(root, "service-a", ".ai-rulez", "config.yaml"), "version: \"3.0\"\nname: a\n")
	writeFile(t, filepath.Join(root, "service-b", ".ai-rulez", "config.json"), `{"version":"3.0","name":"b"}`)

	// Pruned dirs: must not be descended into.
	writeFile(t, filepath.Join(root, "node_modules", "pkg", ".ai-rulez", "config.yaml"), "x: 1")
	writeFile(t, filepath.Join(root, "target", "debug", "deps", ".ai-rulez", "config.yaml"), "x: 1")
	writeFile(t, filepath.Join(root, ".venv", "lib", ".ai-rulez", "config.yaml"), "x: 1")
	writeFile(t, filepath.Join(root, ".cache", ".ai-rulez", "config.yaml"), "x: 1")
	writeFile(t, filepath.Join(root, "build", ".ai-rulez", "config.yaml"), "x: 1")
	writeFile(t, filepath.Join(root, "vendor", ".ai-rulez", "config.yaml"), "x: 1")

	// Shared rule library: `ai-rulez/` (no dot) with a root config — its
	// nested module configs are for inclusion, not generation.
	writeFile(t, filepath.Join(root, "ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"shared-lib\"\n")
	writeFile(t, filepath.Join(root, "ai-rulez", "modules", "core", ".ai-rulez", "config.yaml"), "name: lib-core")
	writeFile(t, filepath.Join(root, "ai-rulez", "modules", "extra", ".ai-rulez", "config.yaml"), "name: lib-extra")

	// A directory called `ai-rulez/` without a root config is NOT a library
	// (e.g. someone happens to name a subproject this) — descend normally.
	writeFile(t, filepath.Join(root, "namesake", "ai-rulez", "sub", ".ai-rulez", "config.yaml"), "name: namesake")

	// Stale broken symlink simulating Rust target/ artifacts (skip on Windows).
	if runtime.GOOS != "windows" {
		linkDir := filepath.Join(root, "alef", "target", "debug", "deps")
		if err := os.MkdirAll(linkDir, 0o755); err != nil {
			t.Fatalf("mkdir link dir: %v", err)
		}
		if err := os.Symlink(filepath.Join(linkDir, "missing-target.rlib"), filepath.Join(linkDir, "broken.rlib")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	chdir(t, root)
	got := findConfigFilesRecursively()

	sort.Strings(got)

	want := []string{
		filepath.Join(".ai-rulez", "config.toml"),
		filepath.Join("namesake", "ai-rulez", "sub", ".ai-rulez", "config.yaml"),
		filepath.Join("service-a", ".ai-rulez", "config.yaml"),
		filepath.Join("service-b", ".ai-rulez", "config.json"),
	}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("config count mismatch: got %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFindConfigFilesRecursively_ConfigPriority(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".ai-rulez")
	writeFile(t, filepath.Join(cfgDir, "config.toml"), "x = 1")
	writeFile(t, filepath.Join(cfgDir, "config.yaml"), "x: 1")
	writeFile(t, filepath.Join(cfgDir, "config.json"), `{"x":1}`)

	chdir(t, root)
	got := findConfigFilesRecursively()

	if len(got) != 1 {
		t.Fatalf("expected exactly one config, got %v", got)
	}
	if filepath.Base(got[0]) != "config.toml" {
		t.Errorf("expected toml to win, got %s", got[0])
	}
}

func TestSelectRecursivePluginConfigsUsesMarketplaceRoot(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, ".ai-rulez", "config.toml")
	memberConfig := filepath.Join(root, "plugins", "example", ".ai-rulez", "config.toml")
	standaloneConfig := filepath.Join(root, "standalone", ".ai-rulez", "config.toml")
	consumerConfig := filepath.Join(root, "consumer", ".ai-rulez", "config.toml")

	writeFile(t, rootConfig, `
version = "4.0"
name = "marketplace"
[marketplace]
name = "marketplace"
members = ["plugins/example"]
`)
	writeFile(t, memberConfig, `
version = "4.0"
name = "example"
[plugin]
name = "example"
version = "1.0.0"
`)
	writeFile(t, standaloneConfig, `
version = "4.0"
name = "standalone"
[plugin]
name = "standalone"
version = "1.0.0"
`)
	writeFile(t, consumerConfig, `
version = "4.0"
name = "consumer"
[[plugins]]
marketplace = "example"
name = "example"
`)

	got, err := selectRecursivePluginConfigs([]string{memberConfig, consumerConfig, standaloneConfig, rootConfig})
	if err != nil {
		t.Fatalf("select plugin configs: %v", err)
	}
	want := []string{rootConfig, standaloneConfig}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("plugin config count mismatch: got %v, want %v", got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Errorf("[%d] got %q, want %q", index, got[index], want[index])
		}
	}
}
