package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
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
	Short: "Print the resolved LLM setup, optionally with one 1-token call",
	Long: `Print the resolved backend, model, base_url host (never the key), whether the key
variable is set (never its value), whether network use is allowed, and the cache directory.
Environment overrides (AI_RULEZ_LLM_*) are applied.

--ping makes one 1-token health call. It refuses unless allow_network is true. The same
information appears as the "llm" section of "ai-rulez doctor".`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fail(runLLMDoctor(watchParentContext(cmd), args, cmd.OutOrStdout()))
	},
}

var llmEstimateCmd = &cobra.Command{
	Use:   "estimate <file>",
	Short: "Estimate the tokens and cost of sending a file as a prompt (no call)",
	Long: `Approximate the prompt tokens of <file> (a conservative estimate of one token per three bytes) and the worst-case
cost with the configured model, from the built-in price table or [llm] price_input_per_mtok /
price_output_per_mtok. Nothing is sent. An unknown model prints "cost unknown".`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fail(runLLMEstimate(watchParentContext(cmd), args[0], cmd.OutOrStdout()))
	},
}

func init() {
	LLMCmd.AddCommand(llmDoctorCmd, llmEstimateCmd)
	addJSONFormat(LLMCmd.PersistentFlags(), &llmJSON, "")
	LLMCmd.PersistentFlags().BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay")
	LLMCmd.PersistentFlags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	llmDoctorCmd.Flags().BoolVar(&llmPing, "ping", false, "Make one 1-token call (needs allow_network = true)")
	llmEstimateCmd.Flags().IntVar(&llmMaxOutput, "max-output", llm.DefaultCompletionCap, "Completion tokens to assume")
}

// loadLLMConfig returns the effective [llm] setup (trust rule and env overrides applied),
// the user-scope-only repository keys that were ignored, and the config directory.
func loadLLMConfig(ctx context.Context, args []string, projectOptional bool) (lc llm.Config, ignored []string, dir string, err error) {
	cfg, loadErr := loadConfigForCommand(ctx, args, config.WithoutRemote())
	if loadErr != nil {
		if !projectOptional {
			return llm.Config{}, nil, "", loadErr
		}
		cfg = nil // no project: user scope and the environment still apply, and their errors are reported
	}
	res, err := cfg.ResolveLLM(nil)
	if cfg != nil {
		dir = cfg.ConfigDir
	}
	return res.Config, res.Ignored, dir, err
}

func runLLMDoctor(ctx context.Context, args []string, out io.Writer) error {
	lc, ignored, dir, err := loadLLMConfig(ctx, args, false)
	if err != nil {
		return err
	}
	opts := llm.Options{ConfigDir: dir}
	d := llm.Diagnose(lc, opts)
	d.IgnoredRepoKeys = ignored
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
		return jsondoc.Write(out, struct {
			llm.Diagnosis
			Ping pingResult `json:"ping"`
		}{d, ping})
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
	// estimate works without a project, but a broken [llm] table, user config or AI_RULEZ_LLM_* value is an error.
	lc, _, _, err := loadLLMConfig(ctx, nil, true)
	if err != nil {
		return err
	}
	res := llm.Estimate(lc, string(data), llmMaxOutput)
	if llmJSON {
		return jsondoc.Write(out, res)
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
