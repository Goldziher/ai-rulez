package lint

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/harnesslimits"
)

// The size-over and key-misspelt predicates of the harness trap table, the
// stale-table check (AR9C0) and the codes of the limit traps.

// Codes of the harness traps added with the limits table.
const (
	CodeHarnessTableStale      = "AR9C0"
	CodeClaudeKeySpelling      = "AR9C7"
	CodeClaudeListingTruncated = "AR9C8"
	CodeHarnessLimitExceeded   = "AR9C9"
	measureFrontmatterChars    = "frontmatter-chars"
	measureFileChars           = "file-chars"
	measureFileBytes           = "file-bytes"
	measureChainBytes          = "chain-bytes"
)

func init() {
	registerRules(
		RuleInfo{CodeHarnessTableStale, "harness-table-stale", SeverityWarning, "a row of the harness trap or limits table was last verified more than [lint.traps] max_table_age_days ago (off unless the setting is above 0)"},
		RuleInfo{CodeClaudeKeySpelling, "claude-frontmatter-key-spelling", SeverityWarning, "a Claude Code skill or subagent file spells a frontmatter key in a variant (underscore for hyphen, wrong case) that Claude Code silently ignores"},
		RuleInfo{CodeClaudeListingTruncated, "claude-listing-truncated", SeverityWarning, "a generated Claude Code skill has description plus when_to_use past the skill listing cap, so the rest is cut off"},
		RuleInfo{CodeHarnessLimitExceeded, "harness-limit-exceeded", SeverityWarning, "a generated instruction file or chain is past the documented size limit of its harness (Codex AGENTS.md chain, Devin and Antigravity rule files, Kilo REVIEW.md), so the rest is not loaded"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeHarnessTableStale: {
			Why:  "The vendor limits and trap rows are checked by hand against the vendor pages; an old date means the rule may describe a harness that has changed.",
			Bad:  "`[lint.traps] max_table_age_days = 90` with a row verified 200 days ago",
			Good: "Re-check the row against its source, update `quote` and `verified_on`",
		},
		CodeClaudeKeySpelling: {
			Why:  "Claude Code ignores a frontmatter field it does not recognise without reporting an error, so `user_invocable` silently does nothing.",
			Bad:  "`disable_model_invocation: true` in a SKILL.md, or `max_turns: 5` in a subagent",
			Good: "`disable-model-invocation: true`; `maxTurns: 5`",
		},
		CodeClaudeListingTruncated: {
			Why:  "Claude Code cuts the combined description and when_to_use text at 1,536 characters in the skill listing, so a trigger phrase past the cap never reaches the model.",
			Bad:  "A generated skill whose description and when_to_use add up to 2,000 characters",
			Good: "Put the key use case first and keep the pair under 1,536 characters",
		},
		CodeHarnessLimitExceeded: {
			Why:  "Codex stops reading AGENTS.md files past project_doc_max_bytes, Devin and Antigravity truncate a rule file past their per-file limit, and Kilo truncates REVIEW.md past 10,000 characters, so the content past it is never loaded.",
			Bad:  "A generated `.devin/rules/style.md` of 15,000 characters",
			Good: "Split the rule, shorten it, or move detail into a skill",
		},
	})
}

// resolveLimits copies each size-over row's provenance from its limits row, so
// the table states a vendor quote and date once.
func resolveLimits(traps []Trap) error {
	for i := range traps {
		t := &traps[i]
		if t.Predicate.Kind != predSizeOver {
			continue
		}
		l, ok := harnesslimits.Get(t.Predicate.LimitID)
		if !ok {
			return fmt.Errorf("traps.toml: %s size-over names unknown limit %q", t.Code, t.Predicate.LimitID)
		}
		t.Predicate.limit = l
		t.Source, t.Quote, t.VerifiedOn = l.Source, l.Quote, l.VerifiedOn
	}
	return nil
}

func (p TrapPredicate) isChain() bool {
	return p.Kind == predSizeOver && p.Measure == measureChainBytes
}

func normalizeKey(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(s))
}

func (p TrapPredicate) evalMisspelt(content []byte) []trapHit {
	fm := parseFrontmatterDoc(parseDoc(string(content)))
	present := map[string]bool{}
	for _, k := range fm.keys {
		present[k.Name] = true
	}
	var hits []trapHit
	for _, k := range fm.keys {
		if slices.Contains(p.Canonical, k.Name) {
			continue
		}
		for _, c := range p.Canonical {
			if normalizeKey(k.Name) == normalizeKey(c) {
				hit := trapHit{line: k.Line, detail: fmt.Sprintf("%q is not a key; the key is %q", sanitizeScannerText(k.Name), c)}
				// Renaming onto a key that is already there would write it twice,
				// which strict YAML parsers reject: offer the fix for the first
				// misspelling of an absent key only.
				if !present[c] {
					hit.fix = keyRenameFix(content, k.Line, k.Name, c)
					present[c] = true
				}
				hits = append(hits, hit)
				break
			}
		}
	}
	return hits
}

// keyRenameFix is the fix of a misspelt top-level key: the line with the key
// spelled as documented. It is nil unless the line starts with name and a colon.
func keyRenameFix(content []byte, line int, name, canonical string) *lineFix {
	lines := strings.Split(string(content), "\n")
	if line < 1 || line > len(lines) {
		return nil
	}
	old := strings.TrimSuffix(lines[line-1], "\r")
	rest, ok := strings.CutPrefix(old, name+":")
	if !ok {
		return nil
	}
	return &lineFix{old: old, new: canonical + ":" + rest, description: fmt.Sprintf("rename frontmatter key %q to %q", name, canonical)}
}

func (p TrapPredicate) evalSize(content []byte) []trapHit {
	var n int
	switch p.Measure {
	case measureFrontmatterChars:
		fm := parseFrontmatterDoc(parseDoc(string(content)))
		for _, key := range p.Keys {
			if k, ok := fm.top(key); ok {
				n += utf8.RuneCountInString(scalar(k.Value))
			}
		}
	case measureFileChars:
		n = utf8.RuneCount(content)
	case measureFileBytes:
		n = len(content)
	default:
		return nil
	}
	if n <= p.limit.Value {
		return nil
	}
	return []trapHit{{line: 1, detail: sizeDetail(n, p.limit)}}
}

func sizeDetail(n int, l harnesslimits.Limit) string {
	return fmt.Sprintf("%d %s, limit %d, %s", n, l.Unit, l.Value, l.Behavior)
}

// chainFile is a file a chain-bytes trap counts.
type chainFile struct {
	rel, abs  string
	size      int
	generated bool
}

// checkChains reports each AGENTS.md chain (the files from the lint root down
// to a directory) whose combined size is past the limit, at the file where the
// sum first crosses it. A [codex] project_doc_max_bytes setting replaces the
// documented default; 0 or less turns the check off.
func (r *runner) checkChains(traps []Trap, chains map[string][]chainFile) {
	for _, t := range traps {
		if !t.Predicate.isChain() {
			continue
		}
		limit := t.Predicate.limit.Value
		if t.Harness == "codex" && r.cfg.Codex != nil {
			limit = r.cfg.Codex.ProjectDocLimit()
		}
		files := chains[t.Code+"/"+t.Harness]
		if limit <= 0 || len(files) == 0 {
			continue
		}
		sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
		size := map[string]int{}
		for _, f := range files {
			size[path.Dir(f.rel)] = f.size
		}
		for _, f := range files {
			total, prior := 0, 0
			for dir := path.Dir(f.rel); ; dir = path.Dir(dir) {
				total += size[dir]
				if dir != path.Dir(f.rel) {
					prior += size[dir]
				}
				if dir == "." {
					break
				}
			}
			if total > limit && prior <= limit {
				l := t.Predicate.limit
				l.Value = limit
				r.addTrap(t, f.abs, trapHit{line: 1, detail: sizeDetail(total, l)}, f.generated)
			}
		}
	}
}

// checkTableAge reports table rows verified more than max_table_age_days ago.
func (r *runner) checkTableAge() {
	if r.lc.Traps == nil || r.lc.Traps.MaxTableAgeDays <= 0 {
		return
	}
	path := r.configFilePath()
	if path == "" {
		return
	}
	for _, row := range TableRows() {
		days, ok := rowAgeDays(row.VerifiedOn, r.trapClock())
		if ok && days <= r.lc.Traps.MaxTableAgeDays {
			continue
		}
		msg := fmt.Sprintf("%s %s was verified %d days ago (%s), past max_table_age_days = %d; re-check %s", row.Kind, row.ID, days, row.VerifiedOn, r.lc.Traps.MaxTableAgeDays, row.Source)
		if !ok {
			msg = fmt.Sprintf("%s %s has no valid verified_on date; re-check %s", row.Kind, row.ID, row.Source)
		}
		r.add(CodeHarnessTableStale, path, 1, "%s", msg)
	}
}

// trapNow is the package clock of the age checks; tests replace it. An injected
// host clock (WithHost) takes precedence.
var trapNow = time.Now

func (r *runner) trapClock() time.Time {
	if r.host.Clock != nil {
		return r.host.Clock.Now()
	}
	return trapNow()
}

func rowAgeDays(verifiedOn string, now time.Time) (int, bool) {
	t, err := time.Parse("2006-01-02", verifiedOn)
	if err != nil {
		return 0, false
	}
	return int(now.Sub(t).Hours() / 24), true
}

// TableRow is one row of the trap or limits table, as the freshness task lists it.
type TableRow struct {
	Kind       string // trap or limit
	ID         string
	Source     string
	VerifiedOn string
}

// TableRows lists every row of the trap and limits tables with its source and
// verification date.
func TableRows() []TableRow {
	var rows []TableRow
	traps, _ := Traps() //nolint:errcheck // an unreadable table has no rows
	for _, t := range traps {
		rows = append(rows, TableRow{"trap", t.Code + " " + t.Name + " (" + t.Harness + ")", t.Source, t.VerifiedOn})
	}
	limits, _ := harnesslimits.All() //nolint:errcheck // an unreadable table has no rows
	for _, l := range limits {
		rows = append(rows, TableRow{"limit", l.ID, l.Source, l.VerifiedOn})
	}
	return rows
}
