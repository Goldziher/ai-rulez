package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// errorClass is a kind of failure a command can end with and how the contract
// renders it.
type errorClass struct {
	name       string
	err        error
	wantStderr string
	wantCode   int
	wantMsg    string // the "error" member of the JSON document; "" when none is written
	wantHint   string
}

func errorClasses() []errorClass {
	return []errorClass{
		{
			name: "plain", err: errors.New("boom"),
			wantStderr: "Error: boom\n", wantCode: 1, wantMsg: "boom",
		},
		{
			name: "hint", err: oops.Hint("try again").Errorf("boom"),
			wantStderr: "Error: boom\n\nHint: try again\n", wantCode: 1, wantMsg: "boom", wantHint: "try again",
		},
		{
			name: "validation details", err: oops.With("errors", []string{"a is wrong", "b is wrong"}).Errorf("invalid configuration"),
			wantStderr: "Error: invalid configuration\n\nValidation errors:\n  - a is wrong\n  - b is wrong\n", wantCode: 1, wantMsg: "invalid configuration",
		},
		{
			name: "findings code", err: failWithCode(exitFindings, errors.New("drift")),
			wantStderr: "Error: drift\n", wantCode: 2, wantMsg: "drift",
		},
		{
			name: "policy loosened", err: fmt.Errorf("check: %w", config.ErrPolicyLoosens),
			wantStderr: "Error: check: " + config.ErrPolicyLoosens.Error() + "\n", wantCode: 2, wantMsg: "check: " + config.ErrPolicyLoosens.Error(),
		},
		{
			name: "already reported", err: exitStatus(exitPartial),
			wantStderr: "", wantCode: 3,
		},
	}
}

func TestReportErrorRendersEveryClassOneWay(t *testing.T) {
	for _, tc := range errorClasses() {
		for _, format := range []string{"", formatText, formatJSON} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				// Arrange
				var stdout, stderr bytes.Buffer

				// Act
				code := ReportError(&stdout, &stderr, format, tc.err)

				// Assert
				assert.Equal(t, tc.wantCode, code)
				assert.Equal(t, tc.wantStderr, stderr.String())
				if format != formatJSON || tc.wantMsg == "" {
					assert.Empty(t, stdout.String())
					return
				}
				var doc map[string]any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
				assert.Equal(t, "error", doc["status"])
				assert.Equal(t, tc.wantMsg, doc["error"])
				assert.InDelta(t, tc.wantCode, doc["exit_code"], 0)
				if tc.wantHint != "" {
					assert.Equal(t, tc.wantHint, doc["hint"])
				} else {
					assert.NotContains(t, doc, "hint")
				}
			})
		}
	}
}

func TestExitCodeFor(t *testing.T) {
	assert.Equal(t, 1, exitCodeFor(errors.New("boom")))
	assert.Equal(t, 2, exitCodeFor(fmt.Errorf("resolve role: %w", config.ErrRoleReference)))
	assert.Equal(t, 2, exitCodeFor(fmt.Errorf("x: %w", errLockedSourceDrift)))
	assert.Equal(t, 2, exitCodeFor(errMovedTag))
	assert.Equal(t, 3, exitCodeFor(fmt.Errorf("wrapped: %w", exitStatus(3))))
	assert.NoError(t, exitStatus(0))
	assert.NoError(t, fail(nil))
}

func TestFailKeepsAnExitErrorsCode(t *testing.T) {
	inner := failWithCode(3, errors.New("partial"))

	assert.Same(t, inner, fail(inner))
}

// Every command is a RunE: os.Exit lives in main only.
func TestEveryRunnableCommandUsesRunE(t *testing.T) {
	walkCommands(RootCmd, func(c *cobra.Command) {
		assert.Nil(t, c.Run, "%s uses Run; return an error from RunE instead of calling os.Exit", c.CommandPath())
	})
}

// resetFormatFlags puts every --format back to its default: flag values outlive
// an in-process execution, and a stale "json" would change what the next one renders.
func resetFormatFlags() {
	walkCommands(RootCmd, func(c *cobra.Command) {
		if f := c.Flags().Lookup("format"); f != nil {
			_ = f.Value.Set(f.DefValue) //nolint:errcheck // the default is always accepted
			f.Changed = false
		}
	})
}

// commandState is what the table test overrides on a command, to restore after.
type commandState struct {
	runE        func(*cobra.Command, []string) error
	args        cobra.PositionalArgs
	preRun      func(*cobra.Command, []string)
	preRunE     func(*cobra.Command, []string) error
	persistent  func(*cobra.Command, []string)
	persistentE func(*cobra.Command, []string) error
	post        func(*cobra.Command, []string)
	postE       func(*cobra.Command, []string) error
	annotations map[*pflag.Flag]map[string][]string
}

func neutralize(c *cobra.Command, runE func(*cobra.Command, []string) error) commandState {
	s := commandState{
		runE: c.RunE, args: c.Args, preRun: c.PreRun, preRunE: c.PreRunE,
		persistent: c.PersistentPreRun, persistentE: c.PersistentPreRunE,
		post: c.PostRun, postE: c.PostRunE, annotations: map[*pflag.Flag]map[string][]string{},
	}
	c.RunE, c.Args, c.PreRun, c.PreRunE, c.PostRun, c.PostRunE = runE, cobra.ArbitraryArgs, nil, nil, nil, nil
	if c != RootCmd {
		c.PersistentPreRun, c.PersistentPreRunE = nil, nil
	}
	c.Flags().VisitAll(func(f *pflag.Flag) {
		kept := map[string][]string{}
		cleared := map[string][]string{}
		for k, v := range f.Annotations {
			if strings.HasPrefix(k, "cobra_annotation_") || k == cobra.BashCompOneRequiredFlag {
				cleared[k] = v
				continue
			}
			kept[k] = v
		}
		if len(cleared) > 0 {
			s.annotations[f] = f.Annotations
			f.Annotations = kept
		}
	})
	return s
}

func (s commandState) restore(c *cobra.Command) {
	c.RunE, c.Args, c.PreRun, c.PreRunE, c.PostRun, c.PostRunE = s.runE, s.args, s.preRun, s.preRunE, s.post, s.postE
	if c != RootCmd {
		c.PersistentPreRun, c.PersistentPreRunE = s.persistent, s.persistentE
	}
	for f, a := range s.annotations {
		f.Annotations = a
	}
}

// Every command x every error class renders the same way: the text on stderr,
// the exit code of the class, and under --format json the error document on
// stdout.
func TestEveryCommandRendersEveryErrorClassTheSameWay(t *testing.T) {
	var leaves []*cobra.Command
	walkCommands(RootCmd, func(c *cobra.Command) {
		if c.RunE != nil && !c.DisableFlagParsing && c.Name() != "help" {
			leaves = append(leaves, c)
		}
	})
	require.Greater(t, len(leaves), 60)

	for _, leaf := range leaves {
		path := strings.Fields(leaf.CommandPath())[1:]
		formats := []string{""}
		if f := leaf.Flags().Lookup("format"); f != nil {
			allowed := f.Annotations[formatValuesAnnotation]
			if slices.Contains(allowed, formatText) {
				formats = append(formats, formatText)
			}
			if slices.Contains(allowed, formatJSON) {
				formats = append(formats, formatJSON)
			}
		}
		for _, tc := range errorClasses() {
			for _, format := range formats {
				t.Run(strings.Join(path, " ")+"/"+tc.name+"/"+format, func(t *testing.T) {
					// Arrange
					state := neutralize(leaf, func(*cobra.Command, []string) error { return tc.err })
					t.Cleanup(func() { state.restore(leaf) })
					resetFormatFlags()
					t.Cleanup(resetFormatFlags)
					args := append([]string{}, path...)
					if format != "" {
						args = append(args, "--format", format)
					}
					var discard bytes.Buffer
					RootCmd.SetOut(&discard)
					RootCmd.SetErr(&discard)
					RootCmd.SetArgs(args)
					t.Cleanup(func() {
						RootCmd.SetOut(nil)
						RootCmd.SetErr(nil)
						RootCmd.SetArgs(nil)
					})

					// Act
					cmd, err := execute()
					var stdout, stderr bytes.Buffer
					require.Error(t, err)
					code := ReportError(&stdout, &stderr, commandFormat(cmd), err)

					// Assert
					assert.Equal(t, tc.wantCode, code)
					assert.Equal(t, tc.wantStderr, stderr.String())
					effective := format
					if f := leaf.Flags().Lookup("format"); f != nil && effective == "" {
						effective = f.DefValue
					}
					if effective == formatJSON && tc.wantMsg != "" {
						var doc map[string]any
						require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
						assert.Equal(t, "error", doc["status"])
						assert.Equal(t, tc.wantMsg, doc["error"])
					} else {
						assert.Empty(t, stdout.String())
					}
				})
			}
		}
	}
}

// A usage error (unknown flag, wrong argument count) is rendered by the same
// renderer and exits 1.
func TestUsageErrorsRenderLikeEveryOtherError(t *testing.T) {
	for _, args := range [][]string{{"generate", "--no-such-flag"}, {"lock", "bogus-subcommand-xyz", "extra"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			resetFormatFlags()
			t.Cleanup(resetFormatFlags)
			var discard, stdout, stderr bytes.Buffer
			RootCmd.SetOut(&discard)
			RootCmd.SetErr(&discard)
			RootCmd.SetArgs(args)
			t.Cleanup(func() {
				RootCmd.SetOut(nil)
				RootCmd.SetErr(nil)
				RootCmd.SetArgs(nil)
			})

			cmd, err := execute()
			require.Error(t, err)
			code := ReportError(&stdout, &stderr, commandFormat(cmd), err)

			assert.Equal(t, 1, code)
			assert.True(t, strings.HasPrefix(stderr.String(), "Error: "), stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}
