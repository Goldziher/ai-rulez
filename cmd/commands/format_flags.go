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

// addJSONFormat gives a command that prints text or JSON the standard --format
// text|json flag; --format json sets the command's asJSON switch. v5 has no
// separate --json/-j: the unused short argument keeps call sites uniform.
func addJSONFormat(fs *pflag.FlagSet, asJSON *bool, _ string) {
	fs.Var(jsonFormat{dst: asJSON}, "format", "Output format: "+formatText+", "+formatJSON)
	f := fs.Lookup("format")
	f.DefValue = formatText
	f.Annotations = map[string][]string{formatValuesAnnotation: {formatText, formatJSON}}
}

// normalizeFormatFlags validates --format against the values its command declared.
func normalizeFormatFlags(cmd *cobra.Command) error {
	ff := cmd.Flags().Lookup("format")
	if ff == nil {
		return nil
	}
	if allowed := ff.Annotations[formatValuesAnnotation]; len(allowed) > 0 {
		return checkFormat(ff.Value.String(), allowed)
	}
	return nil
}

// jsonFormat is the value of a --format flag on a command that prints either
// text or JSON. It sets the command's existing boolean.
type jsonFormat struct{ dst *bool }

func (f jsonFormat) String() string {
	if f.dst != nil && *f.dst {
		return formatJSON
	}
	return formatText
}

func (f jsonFormat) Set(v string) error {
	switch v {
	case formatText:
		*f.dst = false
	case formatJSON:
		*f.dst = true
	default:
		return oops.Errorf("unknown format %q (use text or json)", v)
	}
	return nil
}

func (jsonFormat) Type() string { return "string" }
