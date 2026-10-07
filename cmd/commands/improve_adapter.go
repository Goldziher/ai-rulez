package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/Goldziher/ai-rulez/v5/internal/improve/adapter"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
)

// improveSelf is the argv prefix that starts this binary again (the bundled adapters run as
// `ai-rulez improve adapter <name>` children). Tests replace it.
var improveSelf = func() ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, oops.Wrapf(err, "locate the ai-rulez executable")
	}
	return []string{exe}, nil
}

// improveAdapterClientFactory builds the model client of the review-fix adapter; tests replace it.
var improveAdapterClientFactory adapter.ClientFactory

var improveAdapterFlags struct {
	llmConfig      string
	fixerModel     string
	judgeModel     string
	allowSameModel bool
	votes          int
	maxGrowth      int
	rubric         string
}

// improveAdapterCmd is the child process of a bundled adapter: it speaks optimizer protocol v1 on
// standard input and output and edits the workspace in the current directory. It is not meant to be run
// by hand.
var improveAdapterCmd = &cobra.Command{
	Use:    "adapter <name>",
	Short:  "Run a bundled optimizer adapter (internal, started by improve run)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workspace, err := os.Getwd()
		if err != nil {
			return oops.Wrapf(err, "locate the workspace")
		}
		opts := &adapter.Options{}
		if args[0] == adapter.ReviewFix {
			o := &opts.ReviewFix
			if improveAdapterFlags.llmConfig != "" {
				dec := json.NewDecoder(strings.NewReader(improveAdapterFlags.llmConfig))
				dec.DisallowUnknownFields()
				if err := dec.Decode(&o.LLM); err != nil {
					return oops.Wrapf(err, "decode --llm-config")
				}
			}
			o.FixerModel, o.JudgeModel, o.AllowSameModel = improveAdapterFlags.fixerModel, improveAdapterFlags.judgeModel, improveAdapterFlags.allowSameModel
			o.Votes, o.MaxGrowthPercent, o.Rubric, o.Factory = improveAdapterFlags.votes, improveAdapterFlags.maxGrowth, improveAdapterFlags.rubric, improveAdapterClientFactory
		}
		return oops.Wrap(adapter.Serve(commandContext(cmd), args[0], cmd.InOrStdin(), cmd.OutOrStdout(), workspace, opts))
	},
}

var improveAdaptersCmd = &cobra.Command{
	Use:   "adapters [name]",
	Short: "(experimental) List the bundled optimizer adapters, or print a template",
	Long: `Without a name, list the bundled adapters and templates. With a name, print its template (shell,
research) or describe the runnable adapter (noop, review-fix).

  builtin:noop         changes nothing; for trying the protocol
  builtin:review-fix   judges SKILL.md with the review rubric and applies a verified fix; needs [llm] model
                       and allow_network in the user config and a fixer model different from the judge
                       (--adapter-model or [review.fix] model). Its API key variable is forwarded to the
                       adapter and the provider host is declared as egress.

Select one with --with builtin:<name> (or --adapter <name>, or [improve] optimizer in your user config).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkFormatFlag(improveFlags.format); err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if len(args) == 1 {
			return printAdapter(out, args[0])
		}
		list := adapter.List()
		if improveFlags.format == formatJSON {
			return writeImproveJSON(out, list)
		}
		var b strings.Builder
		for _, a := range list {
			kind := "template"
			if a.Runnable {
				kind = "builtin:" + a.Name
			}
			fmt.Fprintf(&b, "%-20s %s\n", a.Name, a.Summary)
			if a.Runnable {
				fmt.Fprintf(&b, "%-20s use: --with %s\n", "", kind)
			}
		}
		_, err := io.WriteString(out, b.String())
		return oops.Wrap(err)
	},
}

func printAdapter(out io.Writer, name string) error {
	if text, ok := adapter.Template(name); ok {
		_, err := io.WriteString(out, text)
		return oops.Wrap(err)
	}
	for _, a := range adapter.List() {
		if a.Name == name {
			_, err := fmt.Fprintf(out, "%s: %s\nUse: ai-rulez improve run <skill> --with builtin:%s --max-cost N\n", a.Name, a.Summary, a.Name)
			return oops.Wrap(err)
		}
	}
	return oops.Hint("Run `ai-rulez improve adapters` to list them").Errorf("unknown adapter %q", name)
}

func init() {
	f := improveAdapterCmd.Flags()
	f.StringVar(&improveAdapterFlags.llmConfig, "llm-config", "", "Resolved [llm] settings as JSON (never a key: api_key_env names the variable)")
	f.StringVar(&improveAdapterFlags.fixerModel, "fixer-model", "", "Model that writes the fix")
	f.StringVar(&improveAdapterFlags.judgeModel, "judge-model", "", "Model that judges and verifies (default the [llm] model)")
	f.BoolVar(&improveAdapterFlags.allowSameModel, "allow-same-model", false, "Let the fixer and the judge be the same model")
	f.IntVar(&improveAdapterFlags.votes, "votes", 0, "Votes of the judge (default the rubric's maximum)")
	f.IntVar(&improveAdapterFlags.maxGrowth, "max-growth-percent", 0, "How much the fix may grow SKILL.md in percent (default [review.fix] max_growth_percent)")
	f.StringVar(&improveAdapterFlags.rubric, "rubric", "", "Built-in rubric id (default builtin:skill-quality)")
	addFormatFlag(improveAdaptersCmd.Flags(), &improveFlags.format, formatText, formatText, formatText, formatJSON)
	ImproveCmd.AddCommand(improveAdapterCmd, improveAdaptersCmd)
}

// optimizerChoice is the optimizer a run uses.
type optimizerChoice struct {
	argv []string
	// adapter is builtin:<name> for a bundled adapter, empty for --with.
	adapter string
	// envPass and egress are what a bundled adapter needs added to the run's own.
	envPass []string
	egress  []string
}

// resolveOptimizer turns the --with value (or [improve] optimizer) into the command to run.
func resolveOptimizer(cmd *cobra.Command, cfg *config.Config, value string) (*optimizerChoice, error) {
	name, builtin := adapter.Name(value)
	if !builtin {
		argv, err := improve.ParseArgv(value)
		if err != nil {
			return nil, oops.Hint("Pass the optimizer with --with, for example --with 'python optimize.py' or --with builtin:review-fix").Wrap(err)
		}
		// The optimizer runs in the run's workspace: relative paths are the ones the user typed here.
		wd, err := os.Getwd()
		if err != nil {
			return nil, oops.Wrapf(err, "resolve the working directory")
		}
		return &optimizerChoice{argv: improve.ResolveArgv(argv, wd)}, nil
	}
	if !adapter.IsRunnable(name) {
		return nil, oops.Hint("Run `ai-rulez improve adapters "+name+"` to print it").Errorf("%s%s is a template, not a runnable adapter", adapter.Prefix, name)
	}
	self, err := improveSelf()
	if err != nil {
		return nil, err
	}
	argv := append(append([]string(nil), self...), "improve", "adapter", name)
	choice := &optimizerChoice{adapter: adapter.Prefix + name}
	if name == adapter.ReviewFix {
		extra, envPass, egress, err := reviewFixAdapterArgs(cmd, cfg)
		if err != nil {
			return nil, err
		}
		argv = append(argv, extra...)
		choice.envPass, choice.egress = envPass, egress
	}
	choice.argv = argv
	return choice, nil
}

// reviewFixAdapterArgs resolves the model settings of builtin:review-fix in the parent, where the user
// config and the repository config are both known, and refuses before anything runs when the adapter could
// not (AR9J9). The child receives resolved settings, never a key: its API key variable is forwarded by name.
func reviewFixAdapterArgs(cmd *cobra.Command, cfg *config.Config) (args, envPass, egress []string, err error) {
	refuse := func(format string, a ...any) error {
		return oops.Code(improve.CodeAdapterRefused).Hint("docs/improve.md, \"builtin:review-fix\"").Errorf(improve.CodeAdapterRefused+": "+format, a...)
	}
	js, jerr := resolveJudge(cmd, cfg, improveFlags.adapterJudgeModel, rv.DefaultMaxCostUSD, rv.DefaultMaxCalls)
	if jerr != nil {
		return nil, nil, nil, jerr
	}
	if rerr := js.ready(cfg); rerr != nil {
		return nil, nil, nil, refuse("%s", rerr.Error())
	}
	diag := llm.Diagnose(js.lc, llm.Options{ConfigDir: cfg.ConfigDir})
	if len(diag.Problems) > 0 {
		return nil, nil, nil, refuse("the [llm] setup has a problem: %s", strings.Join(diag.Problems, "; "))
	}
	fixer := improveFlags.adapterModel
	if fixer == "" && cfg.Review != nil && cfg.Review.Fix != nil {
		fixer = cfg.Review.Fix.Model
	}
	check := adapter.ReviewFixOptions{LLM: js.lc, FixerModel: fixer, JudgeModel: improveFlags.adapterJudgeModel, AllowSameModel: improveFlags.allowSameModel}
	if cerr := check.Check(); cerr != nil {
		return nil, nil, nil, refuse("%s", strings.TrimPrefix(cerr.Error(), improve.CodeAdapterRefused+": "))
	}
	raw, merr := json.Marshal(js.lc)
	if merr != nil {
		return nil, nil, nil, oops.Wrapf(merr, "encode the [llm] settings")
	}
	args = []string{"--llm-config", string(raw)}
	if fixer != "" {
		args = append(args, "--fixer-model", fixer)
	}
	if improveFlags.adapterJudgeModel != "" {
		args = append(args, "--judge-model", improveFlags.adapterJudgeModel)
	}
	if improveFlags.allowSameModel {
		args = append(args, "--allow-same-model")
	}
	if growth := cfg.Review.FixMaxGrowthPercent(); growth > 0 {
		args = append(args, "--max-growth-percent", strconv.Itoa(growth))
	}
	if js.lc.APIKeyEnv != "" {
		envPass = []string{js.lc.APIKeyEnv}
	}
	host := diag.BaseURLHost
	if host == "" {
		host = config.ReviewProviderDefaultHost
	}
	return args, envPass, []string{host}, nil
}
