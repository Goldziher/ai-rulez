package commands

import (
	"github.com/spf13/pflag"

	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/render"
)

// Statuses of a change document.
const (
	statusCreated = "created"
	statusRemoved = "removed"
	statusUpdated = "updated"
)

// changeResult is the `--format json` document of every command that changes the
// configuration (add, remove, edit, domain, profile, include, skill). Path is
// the file or directory that was created, removed or rewritten; for a profile,
// include or installed skill that is the config file holding the entry.
type changeResult struct {
	Status  string   `json:"status"`
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Domain  string   `json:"domain,omitempty"`
	Path    string   `json:"path,omitempty"`
	Source  string   `json:"source,omitempty"`
	Domains []string `json:"domains,omitempty"`
	Default bool     `json:"default,omitempty"`
	Local   bool     `json:"local,omitempty"`
}

// reportChange writes the result of a change. Under --format json that is the
// document; otherwise the path of what was created, removed or rewritten, alone
// on a line of stdout so a script can capture it (a change with no path of its
// own prints nothing there). Either way it is the command's result, so -q never
// hides it. The confirmation sentence is a separate Info line.
func reportChange(out render.Out, jsonOut bool, res changeResult, printPath bool) error {
	res.Path = displayPath(res.Path)
	if jsonOut {
		return fail(jsondoc.Write(out.Stdout(), res))
	}
	if printPath && res.Path != "" {
		out.Resultln(res.Path)
	}
	return nil
}

// addResultFormat registers --format text|json on a command whose handler asks
// outFor(cmd).JSON(), so the command keeps no flag variable of its own.
func addResultFormat(fs *pflag.FlagSet) {
	var asJSON bool
	addJSONFormat(fs, &asJSON, "j")
}

// displayPath shows p relative to the working directory when it is below it.
func displayPath(p string) string {
	if p == "" || !filepath.IsAbs(p) {
		return p
	}
	wd := workingDir()
	if wd == "" {
		return p
	}
	rel, err := filepath.Rel(wd, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return p
	}
	return rel
}

// explicitConfigDir is the config directory the global -C/--config or
// --config-dir picked, or "" when the project is to be discovered. -C wins (a
// directory, or a file inside one); --config-dir names the directory below the
// working directory unless it is absolute.
func explicitConfigDir() string {
	switch {
	case cfgFile != "":
		return config.ResolveLocalConfigDir(".", cfgFile, "")
	case configDir != "" && filepath.IsAbs(configDir):
		return configDir
	case configDir != "":
		return config.ResolveLocalConfigDir(".", "", configDir)
	}
	return ""
}

// openOperator opens the CRUD operator for the project the global flags select:
// the config directory of -C/--config-dir, else the one discovered in the
// working directory.
func openOperator() (*crud.OperatorImpl, error) {
	if dir := explicitConfigDir(); dir != "" {
		return crud.NewOperatorAt(dir)
	}
	return crud.NewOperator(".")
}
