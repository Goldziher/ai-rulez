package commands

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/exposure"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

// sharedWordsShown is the number of shared words a text row of `exposure collisions` prints.
const sharedWordsShown = 8

// collisionFlags are the flags of `exposure collisions` besides the group's.
type collisionFlags struct {
	minScore float64
	limit    int
}

func newExposureCollisionsCmd(f *exposureFlags) *cobra.Command {
	var cf collisionFlags
	cmd := &cobra.Command{
		Use:   "collisions",
		Short: "Rank skill and agent pairs of different areas whose listings overlap",
		Long: `Compare the listing of every skill and agent a session sees at start (name, description and
when_to_use) with every other one of a different area, and rank the pairs by the Jaccard index of their
words: the lowercase runs of a letter and two or more letters or digits, less common stopwords. Two
listings that share many words compete for the same prompts, so the model may pick the one of the
wrong area. An unclassified listing counts as an area of its own. The command needs an area map.

--min-score defaults to [exposure] collision_min_score when it is set, else 0.1; --limit 0 prints every
pair. --format json prints schema/exposure-collisions.v1.schema.json.`,
		Example: `  ai-rulez exposure collisions
  ai-rulez exposure collisions --min-score 0.2 --limit 50
  ai-rulez exposure collisions --areas tools/context/areas.toml --plugins none --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fail(runExposureCollisions(cmd.Context(), cmd.OutOrStdout(), f, &cf, cmd.Flags().Changed("min-score")))
		},
	}
	cmd.Flags().Float64Var(&cf.minScore, "min-score", exposure.DefaultCollisionMinScore, "Lowest word overlap (Jaccard index, 0 to 1) reported (default: [exposure] collision_min_score, else 0.1)")
	cmd.Flags().IntVar(&cf.limit, "limit", exposure.DefaultCollisionLimit, "Pairs printed, highest score first; 0 prints every pair")
	addJSONFormat(cmd.Flags(), &f.asJSON, "")
	return cmd
}

func runExposureCollisions(ctx context.Context, w io.Writer, f *exposureFlags, cf *collisionFlags, scoreSet bool) error {
	if cf.limit < 0 {
		return oops.Errorf("--limit must not be negative, got %d", cf.limit)
	}
	in, err := loadExposure(ctx, f)
	if err != nil {
		return err
	}
	if in.areas == nil {
		return oops.Errorf("no area map: configure [exposure] areas or areas_file, or pass --areas")
	}
	minScore, err := collisionMinScore(cf.minScore, scoreSet, in.exp)
	if err != nil {
		return err
	}
	rep := exposure.NewCollisionReport(in.inv, in.areas, minScore, cf.limit)
	if f.asJSON {
		return jsondoc.Write(w, rep) //nolint:wrapcheck // stdout write
	}
	return writeCollisionsText(w, rep)
}

// collisionMinScore is --min-score when given, else [exposure] collision_min_score when it is set, else the
// flag's default.
func collisionMinScore(flag float64, changed bool, exp *config.ExposureConfig) (float64, error) {
	if !changed && exp != nil && exp.CollisionMinScore > 0 {
		return exp.CollisionMinScore, nil
	}
	if flag < 0 || flag > 1 || math.IsNaN(flag) {
		return 0, oops.Errorf("--min-score must be between 0 and 1, got %v", flag)
	}
	return flag, nil
}

func writeCollisionsText(w io.Writer, rep *exposure.CollisionReport) error {
	fmt.Fprintf(w, "Collisions of %s (plugins %s, min score %.2f): %d of %d %s among %d %s\n", //nolint:errcheck // stdout write
		rep.Root, rep.Plugins, rep.MinScore, len(rep.Collisions), rep.Total, plural(rep.Total, "pair"),
		rep.ArtifactsCompared, plural(rep.ArtifactsCompared, "listing"))
	if len(rep.Collisions) == 0 {
		fmt.Fprintln(w, "\nno collisions") //nolint:errcheck // stdout write
		return nil
	}
	rows := [][]string{{"SCORE", "FIRST", colArea, "SECOND", colArea, "SHARED"}}
	for i := range rep.Collisions {
		c := &rep.Collisions[i]
		rows = append(rows, []string{
			fmt.Sprintf("%.2f", c.Score), string(c.FirstKind) + " " + c.FirstName, c.FirstArea,
			string(c.SecondKind) + " " + c.SecondName, c.SecondArea, sharedText(c.Shared),
		})
	}
	fmt.Fprintln(w) //nolint:errcheck // stdout write
	return writeTable(w, rows)
}

func sharedText(words []string) string {
	if len(words) <= sharedWordsShown {
		return strings.Join(words, ", ")
	}
	return fmt.Sprintf("%s (+%d)", strings.Join(words[:sharedWordsShown], ", "), len(words)-sharedWordsShown)
}
