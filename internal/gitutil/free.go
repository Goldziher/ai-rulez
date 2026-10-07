package gitutil

// Free functions: the same questions answered by git run through runner.Exec, the
// behavior every caller had before the runner was injectable. A caller that wants
// an injected runner builds a Git with New and calls the method.

// ChangedSince is Git.ChangedSince on the default runner.
func ChangedSince(dir, rev string) ([]string, error) {
	return Git{}.ChangedSince(dir, rev)
}

// ChangesSince is Git.ChangesSince on the default runner.
func ChangesSince(dir, rev string) ([]Change, error) {
	return Git{}.ChangesSince(dir, rev)
}

// IgnoreRules is Git.IgnoreRules on the default runner.
func IgnoreRules(dir string, paths []string) (map[string]IgnoreMatch, error) {
	return Git{}.IgnoreRules(dir, paths)
}

// IgnoreRulesMirrored is Git.IgnoreRulesMirrored on the default runner.
func IgnoreRulesMirrored(dir string, paths []string, rewrite func(rel, content string) string) (map[string]IgnoreMatch, error) {
	return Git{}.IgnoreRulesMirrored(dir, paths, rewrite)
}

// IgnoredAmong is Git.IgnoredAmong on the default runner.
func IgnoredAmong(dir string, paths []string) (map[string]bool, error) {
	return Git{}.IgnoredAmong(dir, paths)
}

// InfoExcludePath is Git.InfoExcludePath on the default runner.
func InfoExcludePath(dir string) string {
	return Git{}.InfoExcludePath(dir)
}

// IsLinkedWorktree is Git.IsLinkedWorktree on the default runner.
func IsLinkedWorktree(dir string) bool {
	return Git{}.IsLinkedWorktree(dir)
}

// IsRepo is Git.IsRepo on the default runner.
func IsRepo(dir string) bool {
	return Git{}.IsRepo(dir)
}

// IsTracked is Git.IsTracked on the default runner.
func IsTracked(path string) bool {
	return Git{}.IsTracked(path)
}

// ListFiles is Git.ListFiles on the default runner.
func ListFiles(dir string) (files []string, ok bool, err error) {
	return Git{}.ListFiles(dir)
}

// MergeBase is Git.MergeBase on the default runner.
func MergeBase(dir, rev string) (string, error) {
	return Git{}.MergeBase(dir, rev)
}

// ShowFile is Git.ShowFile on the default runner.
func ShowFile(dir, ref, repoRelPath string) (content []byte, ok bool) {
	return Git{}.ShowFile(dir, ref, repoRelPath)
}

// StageExecutable is Git.StageExecutable on the default runner.
func StageExecutable(absPath string) (changed bool, err error) {
	return Git{}.StageExecutable(absPath)
}

// StagedChanges is Git.StagedChanges on the default runner.
func StagedChanges(dir string) ([]Change, error) {
	return Git{}.StagedChanges(dir)
}

// TopLevel is Git.TopLevel on the default runner.
func TopLevel(dir string) string {
	return Git{}.TopLevel(dir)
}

// TrackedAmong is Git.TrackedAmong on the default runner.
func TrackedAmong(dir string, paths []string) (map[string]bool, error) {
	return Git{}.TrackedAmong(dir, paths)
}

// TrackedFiles is Git.TrackedFiles on the default runner.
func TrackedFiles(dir string) (files map[string]uint32, ok bool, err error) {
	return Git{}.TrackedFiles(dir)
}

// UntrustedLocalFile is Git.UntrustedLocalFile on the default runner.
func UntrustedLocalFile(path string) string {
	return Git{}.UntrustedLocalFile(path)
}
