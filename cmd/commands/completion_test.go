package commands

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const completionProjectConfig = `version = "5.0"
name = "completion"
presets = ["claude"]

[profiles]
backend = ["backend"]
full = ["backend", "frontend"]

[[roles]]
name = "engineer"

[[roles]]
name = "writer"
`

// completionProject writes a project with two domains, two profiles, two roles
// and one rule and skill each, and makes it the working directory.
func completionProject(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	cfg := filepath.Join(root, ".ai-rulez")
	writeFile(t, filepath.Join(cfg, "config.toml"), completionProjectConfig)
	writeFile(t, filepath.Join(cfg, "rules", "style-guide.md"), "# Style\nUse tabs.\n")
	writeFile(t, filepath.Join(cfg, "domains", "backend", "rules", "api-design.md"), "# API\nVersion it.\n")
	writeFile(t, filepath.Join(cfg, "domains", "frontend", "context", "ui.md"), "# UI\nReact.\n")
	writeFile(t, filepath.Join(cfg, "skills", "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Deploy the service. Use when releasing.\n---\nDeploy.\n")
	chdir(t, root)
}

// complete runs the shell's `__complete` entry point and returns the offered
// words and the directive, as a completion script would see them.
func complete(t *testing.T, args ...string) (words []string, directive string) {
	t.Helper()
	var out bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetErr(io.Discard)
	RootCmd.SetArgs(append([]string{cobra.ShellCompRequestCmd}, args...))
	t.Cleanup(func() {
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
		RootCmd.SetArgs(nil)
		walkCommands(RootCmd, resetFlags)
	})
	require.NoError(t, Execute())
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines[:len(lines)-1] {
		words = append(words, strings.SplitN(line, "\t", 2)[0])
	}
	return words, lines[len(lines)-1]
}

// cobra binds the script commands' output to stdout when it creates them, so the
// scripts themselves are checked through the generators the commands call; the
// command tree must offer all four shells, each with help and an example.
func TestCompletionCommandOffersEveryShell(t *testing.T) {
	scripts := map[string]func(*bytes.Buffer) error{
		"bash":       func(b *bytes.Buffer) error { return RootCmd.GenBashCompletionV2(b, true) },
		"zsh":        func(b *bytes.Buffer) error { return RootCmd.GenZshCompletion(b) },
		"fish":       func(b *bytes.Buffer) error { return RootCmd.GenFishCompletion(b, true) },
		"powershell": func(b *bytes.Buffer) error { return RootCmd.GenPowerShellCompletionWithDesc(b) },
	}
	for shell, gen := range scripts {
		t.Run(shell, func(t *testing.T) {
			// Arrange
			cmd, _, err := RootCmd.Find([]string{"completion", shell})
			require.NoError(t, err)
			require.Equal(t, shell, cmd.Name())

			// Act
			var script bytes.Buffer
			require.NoError(t, gen(&script))

			// Assert
			assert.Contains(t, script.String(), "ai-rulez")
			assert.Contains(t, cmd.Example, "ai-rulez completion "+shell)
			assert.NotEmpty(t, cmd.Long)
		})
	}
}

func TestEnumFlagValuesAreCompleted(t *testing.T) {
	completionProject(t)
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"validate", "--format", ""}, []string{"text", "json", "sarif"}},
		{[]string{"validate", "--fail-on", ""}, []string{"error", "info", "none", "warning"}},
		{[]string{"validate", "--fail-on", "w"}, []string{"warning"}},
		{[]string{"convert", "--fail-on", "d"}, []string{"dropped"}},
		{[]string{"add", "check", "x", "--severity", ""}, []string{"critical", "high", "low", "medium"}},
		{[]string{"add", "rule", "x", "--priority", "m"}, []string{"medium", "minimal"}},
		{[]string{"eval", "run", "--harness", ""}, []string{"claude", "codex", "cursor"}},
		{[]string{"eval", "run", "--mode", ""}, []string{"activation", "cases"}},
		{[]string{"search", "--mode", "h"}, []string{"hybrid"}},
		{[]string{"publish", "--to", ""}, []string{"github-release", "npm", "oci"}},
		{[]string{"telemetry", "export", "--to", ""}, []string{"file", "otlp"}},
		{[]string{"tokens", "--tokenizer", ""}, []string{"cl100k_base", "estimate"}},
		{[]string{"cost", "--target", "cla"}, []string{"claude"}},
		{[]string{"convert", "--from", "rulesync,ap"}, []string{"rulesync,apm"}},
		{[]string{"lock", "--kind", "s"}, []string{"skill", "source"}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			// Act
			words, directive := complete(t, tt.args...)

			// Assert
			for _, w := range tt.want {
				assert.Contains(t, words, w)
			}
			assert.Equal(t, ":4", directive, "an enum flag offers no file names")
		})
	}
}

func TestProjectNamesAreCompleted(t *testing.T) {
	completionProject(t)
	tests := []struct {
		name string
		args []string
		want []string
		not  []string
	}{
		{"remove rule", []string{"remove", "rule", ""}, []string{"style-guide"}, []string{"api-design", "deploy"}},
		{"remove rule by prefix", []string{"remove", "rule", "sty"}, []string{"style-guide"}, []string{"api-design"}},
		{"remove rule in a domain", []string{"remove", "rule", "--domain", "backend", ""}, []string{"api-design"}, []string{"style-guide"}},
		{"show rule in a domain", []string{"show", "rule", "--domain", "backend", ""}, []string{"api-design"}, []string{"style-guide"}},
		{"edit skill", []string{"edit", "skill", ""}, []string{"deploy"}, []string{"style-guide"}},
		{"list --domain", []string{"list", "rules", "--domain", ""}, []string{"backend", "frontend"}, nil},
		{"add rule --domain", []string{"add", "rule", "x", "--domain", "fr"}, []string{"frontend"}, []string{"backend"}},
		{"domain remove", []string{"domain", "remove", ""}, []string{"backend", "frontend"}, nil},
		{"generate --profile", []string{"generate", "--profile", ""}, []string{"backend", "full"}, nil},
		{"generate --profile list", []string{"generate", "--profile", "backend,f"}, []string{"backend,full"}, nil},
		{"profile set-default", []string{"profile", "set-default", ""}, []string{"backend", "full"}, nil},
		{"profile add domains", []string{"profile", "add", "new", "fr"}, []string{"frontend"}, []string{"backend"}},
		{"roles show", []string{"roles", "show", ""}, []string{"engineer", "writer"}, nil},
		{"generate --role", []string{"generate", "--role", "w"}, []string{"writer"}, []string{"engineer"}},
		{"builtins show", []string{"builtins", "show", "sec"}, []string{"security"}, nil},
		{"eval run skills", []string{"eval", "run", ""}, []string{"deploy"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			words, directive := complete(t, tt.args...)

			// Assert
			for _, w := range tt.want {
				assert.Contains(t, words, w)
			}
			for _, w := range tt.not {
				assert.NotContains(t, words, w)
			}
			assert.Equal(t, ":4", directive)
		})
	}
}

func TestCompletionOutsideAProjectOffersNothingAndDoesNotFail(t *testing.T) {
	chdir(t, t.TempDir())

	words, directive := complete(t, "remove", "rule", "")

	assert.Empty(t, words)
	assert.Equal(t, ":4", directive)
}

func TestACommandWithoutArgumentsOffersNoFileNames(t *testing.T) {
	completionProject(t)

	words, directive := complete(t, "doctor", "--strict", "")
	_ = words
	// doctor takes a config file: files are fine there
	assert.NotEqual(t, ":4", directive)

	words, directive = complete(t, "roles", "list", "")
	assert.Empty(t, words)
	assert.Equal(t, ":4", directive)
}

func TestTheOnlyArgumentIsNotCompletedTwice(t *testing.T) {
	completionProject(t)

	words, directive := complete(t, "remove", "rule", "style-guide", "")

	assert.Empty(t, words)
	assert.Equal(t, ":4", directive)
}

// The fixed values a flag completes repeat what its help text names; this fails
// when a value is added to one and not to the other.
func TestCompletedFlagValuesAreNamedInTheFlagHelp(t *testing.T) {
	// Arrange: a directory without a project, so only the fixed values are offered
	chdir(t, t.TempDir())
	var checked int
	walkCommands(RootCmd, func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			fn, ok := c.GetFlagCompletionFunc(f.Name)
			if !ok {
				return
			}
			values, _ := fn(c, nil, "")
			for _, v := range values {
				checked++
				if f.Name == "harness" || f.Name == "target" || f.Name == "targets" {
					continue // help names the flag's meaning, not the list
				}
				assert.Contains(t, f.Usage, v, "%s --%s completes %q, which its help does not name", c.CommandPath(), f.Name, v)
			}
		})
	})
	assert.Greater(t, checked, 100)
}

// Every flag that names a closed set of values in its help has a completion, so a
// new enum flag cannot ship without one.
func TestEnumFlagsAreRegisteredForCompletion(t *testing.T) {
	for _, name := range []string{"format", "fail-on", "severity", "priority", "harness", "tokenizer", "lint-profile", "gate-level", "isolation", "profile", "role", "domain"} {
		var seen int
		walkCommands(RootCmd, func(c *cobra.Command) {
			if f := c.LocalFlags().Lookup(name); f != nil {
				seen++
				_, ok := c.GetFlagCompletionFunc(name)
				exempt := name == "format" && (c.Name() == "init" || c.Name() == "hook")                                   // config and template formats
				exempt = exempt || name == "domain" && (c.Name() == "convert" || c.CommandPath() == "ai-rulez import okf") // names a new domain
				assert.True(t, ok || exempt, "%s --%s has no completion", c.CommandPath(), name)
			}
		})
		assert.Positive(t, seen, name)
	}
}
