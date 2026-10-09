package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
)

// defaultSuggestReplay is how many merged diffs `verifiers suggest` replays by default.
const defaultSuggestReplay = 10

var (
	suggestKind         string
	suggestMaxProposals int
	suggestWrite        bool
	suggestReplay       int
	suggestFormat       bool
)

// VerifiersSuggestCmd asks a model to propose verifiers for one rule.
var VerifiersSuggestCmd = &cobra.Command{
	Use:   "suggest <id>",
	Short: "Propose verifiers for a rule with a model (a dry run that prints and writes nothing)",
	Long: `Ask the configured model to propose deterministic verifiers for a prose rule, skill,
agent or command, then check every candidate without the model: the declaration must load
like a hand-written one, its own pass and fail examples must behave as claimed, and it is
run against the repository to count how many findings it has today. The candidates are
printed as TOML for a human to review; nothing is written unless you pass --write, which
saves the usable ones to a new .ai-rulez/verifiers/suggested-<id>.toml (an existing file is
never overwritten). A suggestion never contains a command or an llm predicate and starts
at warning severity.

This sends the rule text and a repository summary (directory names and file extensions,
no file content) to the model, so it needs --allow-llm and allow_network = true in the
user config. --estimate prints what would be sent and the cost bound, and calls nothing.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return exitStatus(suggestVerifiers(watchParentContext(cmd), args[0], os.Stdout))
	},
}

func init() {
	VerifiersCmd.AddCommand(VerifiersSuggestCmd)
	f := VerifiersSuggestCmd.Flags()
	f.StringVar(&suggestKind, "kind", "rule", "What the id names: rule, skill, agent or command")
	f.IntVar(&suggestMaxProposals, "max-proposals", 5, "Most candidates to ask for and keep")
	f.IntVar(&suggestReplay, "replay", defaultSuggestReplay, "Try each usable proposal on the last N merged diffs (first-parent history) and report how many it would have flagged; 0 turns it off, more than 100 is capped at 100")
	f.BoolVar(&suggestWrite, "write", false, "Save the usable proposals to .ai-rulez/verifiers/suggested-<id>.toml (never overwrites)")
	f.BoolVar(&verifiersAllowLLM, "allow-llm", false, "Send the rule text and a repository summary to the configured model (needs allow_network in the user config)")
	f.Float64Var(&verifiersMaxCost, "max-cost", defaultVerifiersMaxCost, "Most the call may cost in USD (0 removes this cap; [llm] limits still apply)")
	f.BoolVar(&verifiersEstimate, "estimate", false, "Print what would be sent and the cost bound, and call nothing")
	addJSONFormat(f, &suggestFormat, "")
	f.BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// suggestVerifiers runs `verifiers suggest` and returns the exit code: 0 the
// run completed (even with no proposal), 1 it could not run.
func suggestVerifiers(ctx context.Context, id string, out io.Writer) int {
	cfg, err := loadVerifierConfig(ctx, nil)
	if err != nil {
		renderStderr(err)
		return exitVerifiersCannotRun
	}
	switch suggestKind {
	case "rule", improveKindSkill, "agent", "command":
	default:
		renderStderr(oops.Hint("Use rule, skill, agent or command.").Errorf("unknown --kind %q", suggestKind))
		return exitVerifiersCannotRun
	}
	if suggestWrite && verifiersEstimate {
		renderStderr(oops.Errorf("--write and --estimate cannot be combined: an estimate calls nothing, so there is nothing to write"))
		return exitVerifiersCannotRun
	}
	opts, release, err := verifierLLMOptions(ctx, cfg)
	if err != nil {
		renderStderr(err)
		return exitVerifiersCannotRun
	}
	defer release()
	res, err := verifiers.Suggest(ctx, cfg, verifiers.SuggestOptions{Kind: suggestKind, ID: id, MaxProposals: suggestMaxProposals, Replay: suggestReplay, LLM: *opts})
	if err != nil {
		renderStderr(err)
		return exitVerifiersCannotRun
	}
	written := ""
	if suggestWrite {
		if len(res.Usable()) == 0 {
			logger.Warn("--write: no usable proposal, nothing was written")
		} else if written, err = verifiers.WriteSuggestions(cfg, res); err != nil {
			renderStderr(err)
			return exitVerifiersCannotRun
		}
	}
	if suggestFormat {
		if err := writeJSON(out, res); err != nil {
			renderStderr(err)
			return exitVerifiersCannotRun
		}
		return 0
	}
	if _, err := io.WriteString(out, renderSuggestion(res, written, suggestWrite)); err != nil {
		renderStderr(oops.Wrapf(err, "write suggestions"))
		return exitVerifiersCannotRun
	}
	return 0
}

func renderSuggestion(res *verifiers.SuggestResult, written string, wantWrite bool) string {
	var b strings.Builder
	t := res.Target
	fmt.Fprintf(&b, "Verifier suggestions for %s %q (%s)\n", t.Kind, t.ID, t.Path)
	if res.Estimate != "" {
		b.WriteString("\n" + res.Estimate + "\n")
		return b.String()
	}
	rejected, shown := 0, 0
	for i := range res.Proposals {
		p := &res.Proposals[i]
		if p.Rejected != "" {
			rejected++
			continue
		}
		shown++
		writeProposal(&b, shown, p)
	}
	if rejected > 0 {
		b.WriteString("\nRejected:\n")
		for i := range res.Proposals {
			if p := &res.Proposals[i]; p.Rejected != "" {
				fmt.Fprintf(&b, "  %s: %s\n", p.ID, p.Rejected)
			}
		}
	}
	if len(res.Proposals) == 0 {
		reason := res.SkippedReason
		if reason == "" {
			reason = "the model proposed nothing"
		}
		fmt.Fprintf(&b, "\nNo verifier proposed: %s\n", reason)
	}
	for _, n := range res.Notes {
		b.WriteString("note: " + n + "\n")
	}
	if u := res.LLM; u != nil {
		writeSuggestUsage(&b, u)
	}
	switch {
	case written != "":
		fmt.Fprintf(&b, "Wrote %d proposal(s) to %s. Review them, then run `ai-rulez verifiers test`.\n", len(res.Usable()), written)
	case wantWrite:
	case len(res.Usable()) > 0:
		b.WriteString("Dry run: nothing was written. Re-run with --write to save the usable proposals to .ai-rulez/verifiers/.\n")
	}
	return b.String()
}

// writeProposal renders one usable proposal, numbered n.
func writeProposal(b *strings.Builder, n int, p *verifiers.Proposal) {
	fmt.Fprintf(b, "\n# %d. %s\n# %s\n# findings in the repository today: %d", n, p.ID, p.Rationale, p.Hits)
	if len(p.HitFiles) > 0 {
		fmt.Fprintf(b, " (%s)", strings.Join(p.HitFiles, ", "))
	}
	fmt.Fprintf(b, "; examples: %s\n", p.Examples)
	if r := p.Replay; r != nil {
		fmt.Fprintf(b, "# replay: would have flagged %d of %d merged diff(s)", r.Flagged, r.Diffs)
		if len(r.FlaggedCommits) > 0 {
			fmt.Fprintf(b, " (%s)", strings.Join(r.FlaggedCommits, "; "))
		}
		b.WriteString("\n")
	}
	b.WriteString(p.TOML)
}

// writeSuggestUsage renders the model usage line.
func writeSuggestUsage(b *strings.Builder, u *verifiers.LLMUsage) {
	fmt.Fprintf(b, "\nllm: %d call(s), %d from cache, %d prompt + %d completion tokens, about $%.4f", u.Calls, u.Cached, u.PromptTokens, u.CompletionTokens, u.CostUSD)
	if u.MaxCostUSD > 0 {
		fmt.Fprintf(b, " of $%.2f", u.MaxCostUSD)
	}
	b.WriteString("\n")
}
