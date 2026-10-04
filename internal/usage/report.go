package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"

	"github.com/samber/oops"
)

// SkillUsage is one skill's row in a report.
type SkillUsage struct {
	ID       string `json:"id"`
	Domain   string `json:"domain,omitempty"`
	Owner    string `json:"owner,omitempty"`
	Count    int    `json:"count"`
	LastSeen string `json:"last_seen,omitempty"`
}

// ChangedSkill is a skill used at a content hash other than its current one.
type ChangedSkill struct {
	ID          string   `json:"id"`
	CurrentHash string   `json:"current_hash"`
	LoggedHash  []string `json:"logged_hashes"`
}

// Report joins a usage log with the skills index.
type Report struct {
	Events int `json:"events"`
	// Skipped counts log lines that were not valid entries.
	Skipped int          `json:"skipped_lines"`
	Used    []SkillUsage `json:"used"`
	// Never lists indexed skills with no logged use.
	Never []SkillUsage `json:"never_used"`
	// Changed lists skills whose logged hash differs from the index: used before
	// the latest edit, so the evidence may describe an older version.
	Changed []ChangedSkill `json:"changed"`
	// Unknown lists logged ids that are not in the index (renamed, removed, or a
	// skill from outside this project).
	Unknown []SkillUsage `json:"unknown"`
}

// ReadLog reads a JSON Lines usage log.
func ReadLog(path string) (entries []Entry, skipped int, err error) {
	file, err := os.Open(path) //nolint:gosec // user-chosen log path
	if err != nil {
		return nil, 0, oops.With("path", path).Wrapf(err, "open usage log")
	}
	defer file.Close() //nolint:errcheck // read-only

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if json.Unmarshal(line, &entry) != nil || entry.Event != EventSkillInvoked || entry.ID == "" {
			skipped++
			continue
		}
		entries = append(entries, entry)
	}
	return entries, skipped, oops.Wrapf(scanner.Err(), "read usage log")
}

// BuildReport joins entries with the index. Output is sorted and deterministic.
func BuildReport(index *Index, entries []Entry, skipped int) *Report {
	report := &Report{Events: len(entries), Skipped: skipped}

	type tally struct {
		count  int
		last   string
		hashes map[string]bool
	}
	tallies := map[string]*tally{}
	for i := range entries {
		entry := &entries[i]
		t := tallies[entry.ID]
		if t == nil {
			t = &tally{hashes: map[string]bool{}}
			tallies[entry.ID] = t
		}
		t.count++
		if entry.Time > t.last {
			t.last = entry.Time
		}
		if entry.Hash != "" {
			t.hashes[entry.Hash] = true
		}
	}

	indexed := map[string]bool{}
	for _, record := range index.Skills {
		indexed[record.ID] = true
		row := SkillUsage{ID: record.ID, Domain: record.Domain, Owner: record.Owner}
		t := tallies[record.ID]
		if t == nil {
			report.Never = append(report.Never, row)
			continue
		}
		row.Count, row.LastSeen = t.count, t.last
		report.Used = append(report.Used, row)

		var stale []string
		for hash := range t.hashes {
			if hash != record.Hash {
				stale = append(stale, hash)
			}
		}
		if len(stale) > 0 {
			sort.Strings(stale)
			report.Changed = append(report.Changed, ChangedSkill{ID: record.ID, CurrentHash: record.Hash, LoggedHash: stale})
		}
	}
	for id, t := range tallies {
		if !indexed[id] {
			report.Unknown = append(report.Unknown, SkillUsage{ID: id, Count: t.count, LastSeen: t.last})
		}
	}

	sortUsage(report.Used, true)
	sortUsage(report.Never, false)
	sortUsage(report.Unknown, true)
	sort.Slice(report.Changed, func(a, b int) bool { return report.Changed[a].ID < report.Changed[b].ID })
	return report
}

// sortUsage orders rows by count descending (when byCount) then id, domain.
func sortUsage(rows []SkillUsage, byCount bool) {
	sort.SliceStable(rows, func(a, b int) bool {
		if byCount && rows[a].Count != rows[b].Count {
			return rows[a].Count > rows[b].Count
		}
		if rows[a].ID != rows[b].ID {
			return rows[a].ID < rows[b].ID
		}
		return rows[a].Domain < rows[b].Domain
	})
}
