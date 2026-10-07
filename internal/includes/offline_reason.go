package includes

// offlineReason names why a source is read from the cache only: the user's
// --offline, or a command that never fetches includes by design (sbom, list).
func offlineReason() string {
	if SkipFetch {
		return "--offline specified"
	}
	return "this command does not fetch includes"
}
