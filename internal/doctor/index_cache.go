package doctor

import (
	"context"
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/indexcache"
)

// CheckIndexCache is the check of the repository index cache.
const CheckIndexCache = "index-cache"

// checkIndexCache reports which cache location `mcp --serve-skills
// --discover-skills` uses, its size and what is wrong with it: an unwritable
// location or an index file that is corrupt, of another version or not
// authentic. It says nothing for a project that does not discover and has no
// cache.
func checkIndexCache(ctx context.Context, s *state) []Finding {
	if s.cfg == nil || s.opts.IndexCache == nil {
		return nil
	}
	rep, err := indexcache.Inspect(ctx, s.opts.IndexCache(s.cfg))
	if err != nil {
		return []Finding{{Check: CheckIndexCache, Severity: SeverityWarning, Message: err.Error()}}
	}
	discovers := s.cfg.MCP != nil && s.cfg.MCP.DiscoverSkills != "" && s.cfg.MCP.DiscoverSkills != config.DiscoverOff
	if !rep.Exists && !discovers {
		return nil
	}
	var out []Finding
	switch {
	case rep.Kind == indexcache.KindOff:
		out = append(out, Finding{Check: CheckIndexCache, Severity: SeverityInfo, Message: "the index cache is off ([mcp] index_cache = \"off\")"})
	case rep.Exists:
		where := rep.Kind
		if rep.Fallback {
			where += " fallback"
		}
		out = append(out, Finding{Check: CheckIndexCache, Severity: SeverityInfo,
			Message: fmt.Sprintf("the index cache (%s) is %s: %d bytes, %d entries, %d verdicts", where, rep.Dir, rep.Bytes, rep.Entries, rep.Verdicts)})
	default:
		out = append(out, Finding{Check: CheckIndexCache, Severity: SeverityInfo,
			Message: fmt.Sprintf("the index cache (%s) will be kept in %s", rep.Kind, rep.Dir)})
	}
	for _, w := range rep.Warnings {
		out = append(out, Finding{Check: CheckIndexCache, Severity: SeverityWarning, Message: w,
			Hint: "run `ai-rulez cache clear`, or set [mcp] index_cache = \"repo\" or \"off\""})
	}
	return out
}
