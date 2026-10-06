package gitutil

// ChangesBetween is Git.ChangesBetween on the default runner.
func ChangesBetween(dir, base, head string) ([]Change, error) {
	return Git{}.ChangesBetween(dir, base, head)
}

// FirstParentCommits is Git.FirstParentCommits on the default runner.
func FirstParentCommits(dir string, n int) ([]Commit, error) {
	return Git{}.FirstParentCommits(dir, n)
}
