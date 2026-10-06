package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/improve"
)

// improveChildSelf makes the bundled adapters start this test binary again, through the helper process.
func improveChildSelf(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	improveSelf = func() ([]string, error) { return []string{exe, "-test.run=TestImproveHelperProcess", "--"}, nil }
}

func writeImproveConfig(t *testing.T, root, extra string) {
	t.Helper()
	body := "version = \"4.0\"\nname = \"t\"\npresets = [\"claude\"]\n" + extra
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", "config.toml"), []byte(body), 0o600))
}

func TestImprove_ShowAndCleanWorkOnASavedRun(t *testing.T) {
	// Arrange
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := setupImproveRun(t)
	var out, errOut bytes.Buffer
	improveRunCmd.SetOut(&out)
	improveRunCmd.SetErr(&errOut)
	_, err := runImprove(improveRunCmd, "deploy")
	require.NoError(t, err)
	var report improve.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	runDir := filepath.Join(root, ".ai-rulez/local/improve", report.RunID)

	// Act: show as text
	improveFlags.format = formatText
	var shown bytes.Buffer
	improveShowCmd.SetOut(&shown)
	improveShowCmd.SetErr(&errOut)
	require.NoError(t, improveShowCmd.RunE(improveShowCmd, []string{report.RunID}))

	// Assert
	assert.Contains(t, shown.String(), "Run "+report.RunID+" for deploy: accepted")
	assert.Contains(t, shown.String(), "+GOOD advice.")
	assert.NotContains(t, shown.String(), "not signed")

	// Act: show as JSON
	improveFlags.format = formatJSON
	var doc bytes.Buffer
	improveShowCmd.SetOut(&doc)
	require.NoError(t, improveShowCmd.RunE(improveShowCmd, []string{report.RunID}))
	var res improve.ShowResult
	require.NoError(t, json.Unmarshal(doc.Bytes(), &res))
	assert.Equal(t, improve.ShowSchema, res.Schema)
	assert.True(t, res.Signed)

	// Act: clean dry run keeps it, clean removes it
	improveCleanFlags.dryRun = true
	var dry bytes.Buffer
	improveFlags.format = formatText
	improveCleanCmd.SetOut(&dry)
	improveCleanCmd.SetErr(&errOut)
	require.NoError(t, improveCleanCmd.RunE(improveCleanCmd, []string{report.RunID}))
	assert.Contains(t, dry.String(), "Would remove 1 run(s): "+report.RunID)
	assert.DirExists(t, runDir)
	improveCleanFlags.dryRun = false
	var gone bytes.Buffer
	improveCleanCmd.SetOut(&gone)
	require.NoError(t, improveCleanCmd.RunE(improveCleanCmd, []string{report.RunID}))
	assert.Contains(t, gone.String(), "Removed 1 run(s): "+report.RunID)
	assert.NoDirExists(t, runDir)
	require.Error(t, improveShowCmd.RunE(improveShowCmd, []string{report.RunID}), "a cleaned run is gone")
}

func TestImproveClean_AllNeedsConfirmationOrYes(t *testing.T) {
	// Arrange
	resetImproveFlags(t)
	root := improveProject(t)
	run := filepath.Join(root, ".ai-rulez/local/improve/imp-12345678")
	require.NoError(t, os.MkdirAll(run, 0o750))
	improveCleanFlags.all = true
	improveFlags.yes = false
	improveCleanCmd.SetOut(&bytes.Buffer{})
	improveCleanCmd.SetErr(&bytes.Buffer{})
	t.Cleanup(func() { improveCleanFlags.all = false })
	stdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, _ = w.WriteString("n\n")
	_ = w.Close()
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = stdin })

	// Act
	err = improveCleanCmd.RunE(improveCleanCmd, nil)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not confirmed")
	assert.DirExists(t, run)

	// Act: --yes
	improveFlags.yes = true
	require.NoError(t, improveCleanCmd.RunE(improveCleanCmd, nil))
	assert.NoDirExists(t, run)
}

func TestImproveSettings_ConfigFlagsAndTrust(t *testing.T) {
	const table = "[improve]\nmin_gain = 0.4\nmax_rounds = 2\nholdout_tag = \"hold\"\nisolation = \"auto\"\noptimizer = \"repo-optimizer --x\"\nenv_pass = [\"REPO_VAR\"]\n"
	tests := []struct {
		name       string
		trust      bool
		flagArgs   map[string]string
		wantOpt    string
		wantEnv    []string
		wantGain   float64
		wantRounds int
		wantWarn   bool
		wantMode   string
	}{
		{"repository optimizer and env are ignored without trust", false, nil, "", nil, 0.4, 2, true, "auto"},
		{"trust uses them", true, nil, "repo-optimizer --x", []string{"REPO_VAR"}, 0.4, 2, false, "auto"},
		{"a flag wins over the config", true, map[string]string{"min-gain": "0.1", "max-rounds": "5", "isolation": "require", "env-pass": "MINE"}, "repo-optimizer --x", []string{"MINE"}, 0.1, 5, false, "require"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			resetImproveFlags(t)
			root := improveProject(t)
			writeImproveConfig(t, root, table)
			improveFlags.trustRepoOptimizer = tt.trust
			for k, v := range tt.flagArgs {
				require.NoError(t, improveRunCmd.Flags().Set(k, v))
				t.Cleanup(func() { improveRunCmd.Flags().Lookup(k).Changed = false })
			}
			cfg, err := loadConfigForCommand(t.Context(), nil)
			require.NoError(t, err)
			var warn bytes.Buffer

			// Act
			st, err := resolveImproveSettings(improveRunCmd, cfg, &warn)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpt, st.optimizer)
			assert.Equal(t, tt.wantEnv, st.envPass)
			assert.InDelta(t, tt.wantGain, st.minGain, 1e-9)
			assert.Equal(t, tt.wantRounds, st.maxRounds)
			assert.Equal(t, tt.wantMode, string(st.isolation))
			assert.Equal(t, "hold", st.holdoutTag)
			assert.Equal(t, tt.wantWarn, strings.Contains(warn.String(), improve.CodeRepoOptimizerIgnored), warn.String())
		})
	}
}

func TestImproveSettings_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		config string
		mutate func()
		want   string
	}{
		{"invalid table", "[improve]\nmin_gain = 3\n", nil, "min_gain"},
		{"bad isolation flag", "", func() { improveFlags.isolation = "docker" }, "isolation"},
		{"with and adapter", "", func() { improveFlags.with = "x"; improveFlags.adapter = "noop" }, "not both"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			resetImproveFlags(t)
			root := improveProject(t)
			writeImproveConfig(t, root, tt.config)
			if tt.mutate != nil {
				tt.mutate()
			}
			cfg, err := loadConfigForCommand(t.Context(), nil)
			require.NoError(t, err)

			// Act
			_, err = resolveImproveSettings(improveRunCmd, cfg, &bytes.Buffer{})

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestImprove_RepositoryOptimizerIsNotRunWithoutTrust(t *testing.T) {
	// Arrange: the only optimizer comes from the repository config.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := setupImproveRun(t)
	writeImproveConfig(t, root, "[improve]\noptimizer = \"/nonexistent/evil\"\n")
	improveFlags.with = ""
	improveRunCmd.SetOut(&bytes.Buffer{})
	var errOut bytes.Buffer
	improveRunCmd.SetErr(&errOut)

	// Act
	_, err := runImprove(improveRunCmd, "deploy")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "optimizer command is empty")
	assert.Contains(t, errOut.String(), improve.CodeRepoOptimizerIgnored)
	assert.NoDirExists(t, filepath.Join(root, ".ai-rulez/local"))
}

func TestImprove_BundledNoOpAdapterRunsAsAChildAndIsRecorded(t *testing.T) {
	// Arrange
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := setupImproveRun(t)
	improveChildSelf(t)
	improveFlags.with = "builtin:noop"
	improveFlags.isolation = ""
	var out, errOut bytes.Buffer
	improveRunCmd.SetOut(&out)
	improveRunCmd.SetErr(&errOut)

	// Act
	noCandidate, err := runImprove(improveRunCmd, "deploy")

	// Assert
	require.NoError(t, err)
	assert.True(t, noCandidate, "a no-op adapter proposes nothing")
	var report improve.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, "builtin:noop", report.Adapter)
	require.Len(t, report.Rounds, 1)
	assert.Equal(t, "rejected: no change", report.Rounds[0].Decision)
	assert.Contains(t, report.Rounds[0].Summary, "no-op adapter")
	assert.FileExists(t, filepath.Join(root, ".ai-rulez/local/improve", report.RunID, "report.json"))
}

func TestImprove_AdapterFlagIsTheSameAsWithBuiltin(t *testing.T) {
	// Arrange
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	setupImproveRun(t)
	improveChildSelf(t)
	improveFlags.with, improveFlags.adapter = "", "noop"
	var out bytes.Buffer
	improveRunCmd.SetOut(&out)
	improveRunCmd.SetErr(&bytes.Buffer{})

	// Act
	_, err := runImprove(improveRunCmd, "deploy")

	// Assert
	require.NoError(t, err)
	var report improve.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.Equal(t, "builtin:noop", report.Adapter)
}

func TestImprove_TemplatesAreNotRunnable(t *testing.T) {
	// Arrange
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	setupImproveRun(t)
	improveFlags.with = "builtin:shell"
	improveRunCmd.SetOut(&bytes.Buffer{})
	improveRunCmd.SetErr(&bytes.Buffer{})

	// Act
	_, err := runImprove(improveRunCmd, "deploy")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template")
}

func TestImproveAdapters_ListsAndPrintsTemplates(t *testing.T) {
	// Arrange
	resetImproveFlags(t)
	var list, shell, one bytes.Buffer
	improveAdaptersCmd.SetErr(&bytes.Buffer{})

	// Act
	improveAdaptersCmd.SetOut(&list)
	require.NoError(t, improveAdaptersCmd.RunE(improveAdaptersCmd, nil))
	improveAdaptersCmd.SetOut(&shell)
	require.NoError(t, improveAdaptersCmd.RunE(improveAdaptersCmd, []string{"shell"}))
	improveAdaptersCmd.SetOut(&one)
	require.NoError(t, improveAdaptersCmd.RunE(improveAdaptersCmd, []string{"review-fix"}))
	err := improveAdaptersCmd.RunE(improveAdaptersCmd, []string{"nope"})

	// Assert
	assert.Contains(t, list.String(), "review-fix")
	assert.Contains(t, list.String(), "use: --with builtin:noop")
	assert.Contains(t, shell.String(), "#!/bin/sh")
	assert.Contains(t, one.String(), "--with builtin:review-fix")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown adapter")
}

func TestImproveReviewFixAdapter_ResolvesFromUserConfigAndRefusesWithoutIt(t *testing.T) {
	const userLLM = "[llm]\nprovider = \"openai\"\nmodel = \"judge-x\"\napi_key_env = \"IMPROVE_TEST_API_KEY\"\nbase_url = \"https://llm.example.test/v1\"\nallow_network = true\nbackend = \"openaicompat\"\n"
	tests := []struct {
		name     string
		userCfg  string
		repoCfg  string
		flags    func()
		wantErr  string
		wantArgs []string
	}{
		{name: "no model configured", wantErr: improve.CodeAdapterRefused},
		{name: "network off", userCfg: strings.Replace(userLLM, "allow_network = true", "allow_network = false", 1), repoCfg: "[review.fix]\nmodel = \"fixer-y\"\n", wantErr: improve.CodeAdapterRefused},
		{name: "no fixer model", userCfg: userLLM, wantErr: "no fixer model"},
		{name: "fixer equals judge", userCfg: userLLM, flags: func() { improveFlags.adapterModel = "openai/judge-x" }, wantErr: "both"},
		{name: "fixer from [review.fix]", userCfg: userLLM, repoCfg: "[review.fix]\nmodel = \"fixer-y\"\n", wantArgs: []string{"--fixer-model", "fixer-y"}},
		{name: "fixer from the flag", userCfg: userLLM, flags: func() { improveFlags.adapterModel = "fixer-z" }, wantArgs: []string{"--fixer-model", "fixer-z"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			xdg := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", xdg)
			if tt.userCfg != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(xdg, "ai-rulez"), 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(xdg, "ai-rulez", "config.toml"), []byte(tt.userCfg), 0o600))
			}
			resetImproveFlags(t)
			root := improveProject(t)
			writeImproveConfig(t, root, tt.repoCfg)
			if tt.flags != nil {
				tt.flags()
			}
			cfg, err := loadConfigForCommand(t.Context(), nil)
			require.NoError(t, err)

			// Act
			args, envPass, egress, err := reviewFixAdapterArgs(improveRunCmd, cfg)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			joined := strings.Join(args, " ")
			for i := 0; i < len(tt.wantArgs); i += 2 {
				assert.Contains(t, joined, tt.wantArgs[i]+" "+tt.wantArgs[i+1])
			}
			assert.Contains(t, joined, "--llm-config")
			assert.Contains(t, joined, `"model":"judge-x"`)
			assert.NotContains(t, joined, "IMPROVE_TEST_SECRET", "no key value ever reaches the command line")
			assert.Equal(t, []string{"IMPROVE_TEST_API_KEY"}, envPass, "the key variable is forwarded by name")
			assert.Equal(t, []string{"llm.example.test"}, egress, "the provider host is declared")
		})
	}
}
