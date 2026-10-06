package usage

// MergeEntries concatenates the entries of several logs in order and drops an
// entry whose event id was already seen: two copies of one log, or logs from
// machines that share events, then count each event once. A native log and its OTLP
// export cannot be mixed in one report run (--from-otlp reads every input as OTLP),
// so they are deduplicated only when read in separate runs and merged by the caller. Entries without an id (logs older than version 3)
// carry no identity to compare, so all of them are kept. It returns the merged
// entries and how many were dropped as duplicates.
func MergeEntries(logs ...[]Entry) (merged []Entry, duplicates int) {
	seen := map[string]bool{}
	for _, entries := range logs {
		for i := range entries {
			id := entries[i].EventID
			if id != "" {
				if seen[id] {
					duplicates++
					continue
				}
				seen[id] = true
			}
			merged = append(merged, entries[i])
		}
	}
	return merged, duplicates
}
