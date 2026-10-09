package parity_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/parity"
)

// reportAll fails the test with one line per problem, so a single run shows everything to fix.
func reportAll(t *testing.T, problems []string) {
	t.Helper()
	if len(problems) > 0 {
		t.Fatalf("%d problem(s):\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}

// The table must describe the code that exists: every runnable command and
// every tool is in it, every entry names something real.
func TestEveryCommandAndToolIsInTheTable(t *testing.T) {
	// Arrange
	tree, tools := commandTree(), realSurfaces(t)

	// Act
	problems := coverage(parity.Capabilities(), tree, tools)

	// Assert
	reportAll(t, problems)
}

func TestTheTableIsWellFormed(t *testing.T) {
	reportAll(t, shape(parity.Capabilities()))
}

// A paired capability must take the same inputs on both sides, or say why not.
func TestPairedCapabilitiesTakeTheSameInputs(t *testing.T) {
	// Arrange
	tree, tools := commandTree(), realSurfaces(t)

	// Act
	problems := contracts(parity.Capabilities(), tree, tools)

	// Assert
	reportAll(t, problems)
}

// The checks above must be able to fail: each of these tables is wrong in one
// way, and the checker has to say so.
func TestTheCheckersCatchEveryKindOfDrift(t *testing.T) {
	tree, tools := commandTree(), realSurfaces(t)
	real := parity.Capabilities()
	without := func(id string) []parity.Capability {
		var out []parity.Capability
		for _, c := range real {
			if c.ID != id {
				out = append(out, c)
			}
		}
		return out
	}
	with := func(id string, edit func(*parity.Capability)) []parity.Capability {
		out := append([]parity.Capability(nil), real...)
		for i := range out {
			if out[i].ID == id {
				edit(&out[i])
			}
		}
		return out
	}

	t.Run("a command with no row", func(t *testing.T) {
		problems := coverage(without("version"), tree, tools)
		assert.Contains(t, strings.Join(problems, "\n"), `command "version" is in no capability`)
	})
	t.Run("a tool with no row", func(t *testing.T) {
		problems := coverage(without("doctor"), tree, tools)
		assert.Contains(t, strings.Join(problems, "\n"), `tool "doctor" of the authoring server is in no capability`)
	})
	t.Run("a row naming a missing command", func(t *testing.T) {
		problems := coverage(with("version", func(c *parity.Capability) { c.CLI = []string{"versionn"} }), tree, tools)
		assert.Contains(t, strings.Join(problems, "\n"), `names command "versionn", which does not exist`)
	})
	t.Run("a row naming a missing tool", func(t *testing.T) {
		problems := coverage(with("version", func(c *parity.Capability) { c.Tool = "get_versionn" }), tree, tools)
		assert.Contains(t, strings.Join(problems, "\n"), `names tool "get_versionn"`)
	})
	t.Run("an exclusion without a reason", func(t *testing.T) {
		problems := shape(with("lock-write", func(c *parity.Capability) { c.Reason = "" }))
		assert.Contains(t, strings.Join(problems, "\n"), "needs a reason")
	})
	t.Run("a flag excluded that does not exist", func(t *testing.T) {
		problems := contracts(with("clean", func(c *parity.Capability) {
			c.CLIFlags = append(c.CLIFlags, parity.Exclusion{Names: []string{"no-such-flag"}, Reason: "a made up flag that must be reported"})
		}), tree, tools)
		assert.Contains(t, strings.Join(problems, "\n"), "--no-such-flag, which does not exist")
	})
	t.Run("a flag with neither a pair nor an exclusion", func(t *testing.T) {
		problems := contracts(with("clean", func(c *parity.Capability) { c.CLIFlags = nil }), tree, tools)
		assert.Contains(t, strings.Join(problems, "\n"), "flags with no tool argument and no exclusion")
	})
	t.Run("a pair of different types", func(t *testing.T) {
		problems := contracts(with("clean", func(c *parity.Capability) {
			c.Flags = append(c.Flags, parity.FlagPair{Flag: "profile", Arg: "dry_run"})
		}), tree, tools)
		assert.Contains(t, strings.Join(problems, "\n"), "but argument \"dry_run\" is boolean")
	})
	t.Run("an argument with no flag", func(t *testing.T) {
		problems := contracts(with("version", func(c *parity.Capability) { c.Flags = []parity.FlagPair{{Flag: "nope"}} }), tree, tools)
		assert.Contains(t, strings.Join(problems, "\n"), "flag --nope does not exist")
	})
	t.Run("the real table passes", func(t *testing.T) {
		require.Empty(t, coverage(real, tree, tools))
	})
}
