package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
)

const bytesPerKiB = 1024

// cacheStatusDoc is the --format json document of `cache status`.
type cacheStatusDoc struct {
	indexcache.Report
}

// cacheClearDoc is the --format json document of `cache clear`.
type cacheClearDoc struct {
	indexcache.Cleared
}

// newCacheCmd groups the commands that manage the repository index cache of
// `mcp --serve-skills --discover-skills`.
func newCacheCmd() *cobra.Command {
	var asJSON, all bool
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Show or clear the repository index cache",
		Long: `Manage the cache of the repository index that "mcp --serve-skills --discover-skills" keeps
between runs: the summaries of the instruction files, rules, agents and skills it indexed, and the scan
verdicts of content found clean. It never holds a file body. The cache lives in the user cache directory,
or in a gitignored directory of the project when that is not writable; [mcp] index_cache chooses
(user, repo or off).`,
		Example: `  ai-rulez cache status
  ai-rulez cache clear`,
	}
	statusCmd := &cobra.Command{
		Use:   keyStatus,
		Short: "Show where the index cache is, its size and its entry count",
		Long: `Print the location of the index cache of this project, its size on disk, how many file summaries and
scan verdicts it holds, and any problem with it (an unwritable location, a corrupt or unauthentic index
file). Nothing is changed.`,
		Example: `  ai-rulez cache status
  ai-rulez cache status --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := cacheOptions(watchParentContext(cmd))
			if err != nil {
				return fail(err)
			}
			rep, err := indexcache.Inspect(cmd.Context(), opts)
			if err != nil {
				return fail(err)
			}
			return fail(writeCacheStatus(cmd.OutOrStdout(), rep, asJSON))
		},
	}
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Remove the index cache of this project",
		Long: `Remove the index cache of this project from the user cache and from the project directory. The next
"mcp --serve-skills --discover-skills" start rebuilds it. With --all it removes the index caches of every
project in the user cache.`,
		Example: `  ai-rulez cache clear
  ai-rulez cache clear --all`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := cacheOptions(watchParentContext(cmd))
			if err != nil {
				return fail(err)
			}
			clear := indexcache.Clear
			if all {
				clear = indexcache.ClearAll
			}
			cleared, err := clear(cmd.Context(), opts)
			if err != nil {
				return fail(err)
			}
			return fail(writeCacheCleared(cmd.OutOrStdout(), cleared, asJSON))
		},
	}
	clearCmd.Flags().BoolVar(&all, "all", false, "Remove the index caches of every project in the user cache")
	cacheCmd.AddCommand(statusCmd, clearCmd)
	addJSONFormat(statusCmd.Flags(), &asJSON, "")
	addJSONFormat(clearCmd.Flags(), &asJSON, "")
	specNoLocal.Bool(cacheCmd.PersistentFlags(), &noLocal, "Ignore the machine-local config.local.* overlay")
	return cacheCmd
}

// cacheOptions places the index cache of the project the command runs in. A
// directory without a configuration is a project with the defaults.
func cacheOptions(ctx context.Context) (indexcache.Options, error) {
	cfg, err := loadConfigOrNoConfig(ctx, config.WithoutRemote())
	switch {
	case err == nil:
	case errors.Is(err, config.ErrNoConfig):
		root, absErr := filepath.Abs(workingDir())
		if absErr != nil {
			return indexcache.Options{}, oops.Wrapf(absErr, "resolve the working directory")
		}
		cfg = &config.Config{BaseDir: root}
	default:
		return indexcache.Options{}, err
	}
	opts, _ := (&mcp.ServeSetup{Version: Version}).IndexCacheOptions(ctx, cfg, false)
	return opts, nil
}

func writeCacheStatus(w io.Writer, rep indexcache.Report, asJSON bool) error {
	if asJSON {
		return jsondoc.Write(w, cacheStatusDoc{rep}) //nolint:wrapcheck // stdout write
	}
	switch {
	case rep.Kind == indexcache.KindOff:
		fmt.Fprintln(w, "The index cache is off ([mcp] index_cache = \"off\").") //nolint:errcheck // stdout write
	case !rep.Exists:
		fmt.Fprintf(w, "No index cache yet; it would be kept in %s (%s).\n", rep.Dir, rep.Kind) //nolint:errcheck // stdout write
	default:
		where := rep.Kind
		if rep.Fallback {
			where += ", fallback: the user cache is not used"
		}
		fmt.Fprintf(w, "Index cache (%s): %s\n", where, rep.Dir)                                                               //nolint:errcheck // stdout write
		fmt.Fprintf(w, "%d entries, %d %s, %s\n", rep.Entries, rep.Verdicts, plural(rep.Verdicts, "verdict"), size(rep.Bytes)) //nolint:errcheck // stdout write
	}
	for _, warning := range rep.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning) //nolint:errcheck // stdout write
	}
	return nil
}

func writeCacheCleared(w io.Writer, cleared indexcache.Cleared, asJSON bool) error {
	if asJSON {
		return jsondoc.Write(w, cacheClearDoc{cleared}) //nolint:wrapcheck // stdout write
	}
	if len(cleared.Removed) == 0 {
		_, err := fmt.Fprintln(w, "Nothing to remove: there is no index cache.")
		return err //nolint:wrapcheck // stdout write
	}
	for _, dir := range cleared.Removed {
		if _, err := fmt.Fprintf(w, "Removed %s\n", dir); err != nil {
			return err //nolint:wrapcheck // stdout write
		}
	}
	_, err := fmt.Fprintf(w, "Freed %s.\n", size(cleared.Bytes))
	return err //nolint:wrapcheck // stdout write
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func size(n int64) string {
	switch {
	case n >= bytesPerKiB*bytesPerKiB:
		return fmt.Sprintf("%.1f MiB", float64(n)/(bytesPerKiB*bytesPerKiB))
	case n >= bytesPerKiB:
		return fmt.Sprintf("%.1f KiB", float64(n)/bytesPerKiB)
	}
	return fmt.Sprintf("%d bytes", n)
}
