package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/llm"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	llmJSON      bool
	llmPing      bool
	llmMaxOutput int
)

// LLMCmd groups the read-only LLM access commands. Neither subcommand sends a
// prompt: doctor only pings (one token) when asked and allowed, and estimate
// never calls a model.
var LLMCmd = &cobra.Command{
	Use:   "llm",
	Short: "Inspect the [llm] model-access configuration (read-only)",
	Long: `Inspect how ai-rulez would reach a language model.

  ai-rulez llm doctor      print the resolved backend, model, endpoint host, network gate and cache
  ai-rulez llm estimate    estimate tokens and cost of sending a file, without calling anything

ai-rulez never calls a model unless [llm] allow_network = true. See docs/llm.md.`,
}

var llmDoctorCmd = &cobra.Command{
	Use:   "doctor [config-file]",
	Short: "Print the resolved LLM setup; --ping makes one 1-token call",
	Long: `Print the resolved backend, model, base_url host (never the key), whether the key
variable is set (never its value), whether network use is allowed, and the cache directory.
Environment overrides (AI_RULEZ_LLM_*) are applied.

--ping makes one 1-token health call. It refuses unless allow_network is true. The same
information appears as the "llm" section of "ai-rulez doctor".`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runLLMDoctor(watchParentContext(cmd), args, cmd.OutOrStdout()); err != nil {
			fmtError(err)
			os.Exit(1)
		}
	},
}

var llmEstimateCmd = &cobra.Command{
	Use:   "estimate <file>",
	Short: "Estimate the tokens and cost of sending a file as a prompt (no call)",
	Long: `Approximate the prompt tokens of <file> (about four bytes per token) and the worst-case
cost with the configured model, from the built-in price table or [llm] price_input_per_mtok /
price_output_per_mtok. Nothing is sent. An unknown model prints "cost unknown".`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runLLMEstimate(watchParentContext(cmd), args[0], cmd.OutOrStdout()); err != nil {
			fmtError(err)
			os.Exit(1)
		}
	},
}

func init() {
	LLMCmd.AddCommand(llmDoctorCmd, llmEstimateCmd)
	LLMCmd.PersistentFlags().BoolVar(&llmJSON, "json", false, "Print JSON")
	LLMCmd.PersistentFlags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay")
	LLMCmd.PersistentFlags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	llmDoctorCmd.Flags().BoolVar(&llmPing, "ping", false, "Make one 1-token call (needs allow_network = true)")
	llmEstimateCmd.Flags().IntVar(&llmMaxOutput, "max-output", llm.DefaultCompletionCap, "Completion tokens to assume")
}

// loadLLMConfig returns the [llm] table with env overrides applied and the config directory.
func loadLLMConfig(ctx context.Context, args []string) (llm.Config, string, error) {
	cfg, err := loadConfigForCommand(ctx, args, config.WithoutRemote())
	if err != nil {
		return llm.Config{}, "", err
	}
	lc, err := cfg.ResolvedLLM()
	return lc, cfg.ConfigDir, err
}

func runLLMDoctor(ctx context.Context, args []string, out io.Writer) error {
	lc, dir, err := loadLLMConfig(ctx, args)
	if err != nil {
		return err
	}
	opts := llm.Options{ConfigDir: dir}
	d := llm.Diagnose(lc, opts)
	type pingResult struct {
		Attempted bool   `json:"attempted"`
		OK        bool   `json:"ok"`
		Error     string `json:"error,omitempty"`
	}
	var ping pingResult
	if llmPing {
		ping.Attempted = true
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if perr := llm.Ping(pctx, lc, opts); perr != nil {
			ping.Error = llm.RedactSecrets(perr.Error())
		} else {
			ping.OK = true
		}
	}
	if llmJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return oops.Wrapf(enc.Encode(struct {
			llm.Diagnosis
			Ping pingResult `json:"ping"`
		}{d, ping}), "write report")
	}
	d.WriteText(out)
	if ping.Attempted {
		if ping.OK {
			fprintf(out, "ping:            ok\n")
		} else {
			fprintf(out, "ping:            failed: %s\n", ping.Error)
		}
	}
	if len(d.Problems) > 0 || (ping.Attempted && !ping.OK) {
		return oops.Errorf("llm setup has problems")
	}
	return nil
}

func runLLMEstimate(ctx context.Context, path string, out io.Writer) error {
	data, err := os.ReadFile(path) //nolint:gosec // the user names the file
	if err != nil {
		return oops.Wrapf(err, "read %s", path)
	}
	lc, _, err := loadLLMConfig(ctx, nil)
	if err != nil {
		lc = llm.Config{} // estimate works without a project
	}
	res := llm.Estimate(lc, string(data), llmMaxOutput)
	if llmJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return oops.Wrapf(enc.Encode(res), "write estimate")
	}
	cost := "unknown (no price for this model; set price_input_per_mtok and price_output_per_mtok)"
	if res.CostKnown {
		cost = fmt.Sprintf("<= $%.6f", res.CostUSD)
	}
	fprintf(out, "model:         %s\nfile bytes:    %d\nprompt tokens: ~%d\nmax output:    %d\nworst-case cost: %s\n(approximate; nothing was sent)\n",
		orNoneLLM(res.Model), res.Bytes, res.PromptTokens, res.MaxOutput, cost)
	return nil
}

func orNoneLLM(s string) string {
	if s == "" {
		return "(none configured)"
	}
	return s
}

// fprintf writes to a report writer; a failed terminal write has nowhere to be reported.
func fprintf(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, format, args...) //nolint:errcheck // report output
}
