package commands

import (
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Output formats shared by the commands that print a report.
const (
	formatText = "text"
	formatJSON = "json"
	// formatValuesAnnotation holds the values a --format flag accepts, so the
	// root command can reject any other one with a single wording.
	formatValuesAnnotation = "ai-rulez-format-values"
)

// checkFormat validates a --format value against the values the command accepts.
// An empty value is always accepted: it means the command's default.
func checkFormat(value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return oops.Errorf("unknown --format %q (use %s)", value, strings.Join(allowed, ", "))
}

// checkFormatFlag validates a --format value of a command that prints text or json.
func checkFormatFlag(value string) error {
	return checkFormat(value, []string{formatText, formatJSON})
}

// addFormatFlag registers --format on fs. initial is the flag's own default and
// shown the default help prints: they differ for a command that tells "no
// --format given" from "text" (validate --format implies --strict), so help
// still names the same default everywhere. allowed lists the accepted values.
func addFormatFlag(fs *pflag.FlagSet, target *string, initial, shown string, allowed ...string) {
	fs.StringVar(target, "format", initial, "Output format: "+strings.Join(allowed, ", "))
	f := fs.Lookup("format")
	f.DefValue = shown // used only to print the default in help
	f.Annotations = map[string][]string{formatValuesAnnotation: allowed}
}

// addJSONAlias registers the hidden, deprecated --json (and its shorthand) of a
// command that predates --format. normalizeFormatFlags maps it onto --format json.
func addJSONAlias(fs *pflag.FlagSet, asJSON *bool, short string) {
	if short != "" {
		fs.BoolVarP(asJSON, "json", short, false, "Output as JSON (deprecated: use --format json)")
	} else {
		fs.BoolVar(asJSON, "json", false, "Output as JSON (deprecated: use --format json)")
	}
	_ = fs.MarkDeprecated("json", "use --format json") //nolint:errcheck // the flag was just registered
}

// addJSONFlagAlias adds the hidden --json alias to a command that already has
// --format with a json value; normalizeFormatFlags maps it onto --format json.
func addJSONFlagAlias(fs *pflag.FlagSet) {
	addJSONAlias(fs, new(bool), "")
}

// addJSONFormat gives a command that only had --json the standard --format text|json
// and keeps --json as the alias. The command keeps reading asJSON.
func addJSONFormat(fs *pflag.FlagSet, asJSON *bool, short string) {
	addFormatFlag(fs, new(string), "", formatText, formatText, formatJSON)
	addJSONAlias(fs, asJSON, short)
}

// normalizeFormatFlags validates --format against the values its command declared
// and keeps --format and the deprecated --json in step: --json means --format json,
// and --format json sets a command's json switch.
func normalizeFormatFlags(cmd *cobra.Command) error {
	ff := cmd.Flags().Lookup("format")
	if ff == nil {
		return nil
	}
	if allowed := ff.Annotations[formatValuesAnnotation]; len(allowed) > 0 {
		if err := checkFormat(ff.Value.String(), allowed); err != nil {
			return err
		}
	}
	jf := cmd.Flags().Lookup("json")
	if jf == nil {
		return nil
	}
	if jf.Changed && jf.Value.String() == valueTrue {
		if ff.Changed && ff.Value.String() != formatJSON {
			return oops.Hint("Drop --json, or use --format json.").
				Errorf("--json conflicts with --format %s", ff.Value.String())
		}
		return ff.Value.Set(formatJSON) //nolint:wrapcheck // a string flag cannot fail to take "json"
	}
	if ff.Value.String() == formatJSON {
		return jf.Value.Set(valueTrue) //nolint:wrapcheck // a bool flag cannot fail to take "true"
	}
	return nil
}
