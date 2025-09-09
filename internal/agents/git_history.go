package agents

import (
	"os/exec"
	"strings"
)

// analyzeGitHistory analyzes the git repository history for patterns and conventions
func analyzeGitHistory(rootPath string) *GitHistory {
	history := &GitHistory{
		RecentCommits:     []string{},
		CommonPatterns:    []string{},
		CodingConventions: []string{},
	}

	// Check if git is available and this is a git repository
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = rootPath
	if err := cmd.Run(); err != nil {
		return history // Not a git repository or git not available
	}

	history.HasGit = true

	// Get commit count
	cmd = exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = rootPath
	if output, err := cmd.Output(); err == nil {
		countStr := strings.TrimSpace(string(output))
		// Parse count but ignore error if it fails
		count := 0
		for _, c := range countStr {
			if c >= '0' && c <= '9' {
				count = count*10 + int(c-'0')
			} else {
				break
			}
		}
		history.CommitCount = count
	}

	// Only analyze if there's enough history
	if history.CommitCount < 10 {
		return history
	}

	// Get recent commit messages
	cmd = exec.Command("git", "log", "--oneline", "-n", "10", "--format=%s")
	cmd.Dir = rootPath
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				history.RecentCommits = append(history.RecentCommits, line)
			}
		}
	}

	// Analyze commit patterns
	history.CommonPatterns = analyzeCommitPatterns(history.RecentCommits)

	// Analyze recent code changes for conventions
	cmd = exec.Command("git", "diff", "HEAD~5", "--stat")
	cmd.Dir = rootPath
	if output, err := cmd.Output(); err == nil {
		history.CodingConventions = analyzeCodingConventions(string(output))
	}

	return history
}

// analyzeCommitPatterns extracts common patterns from commit messages
func analyzeCommitPatterns(commits []string) []string {
	patterns := []string{}
	prefixCount := make(map[string]int)

	for _, commit := range commits {
		// Check for conventional commit patterns
		if idx := strings.Index(commit, ":"); idx > 0 && idx < 20 {
			prefix := strings.TrimSpace(commit[:idx])
			prefixCount[prefix]++
		}
	}

	// Add frequently used prefixes as patterns
	for prefix, count := range prefixCount {
		if count >= 2 {
			patterns = append(patterns, "Uses '"+prefix+":' commit prefix")
		}
	}

	// Check for specific conventions
	hasTests := false
	hasDocs := false
	for _, commit := range commits {
		lower := strings.ToLower(commit)
		if strings.Contains(lower, "test") {
			hasTests = true
		}
		if strings.Contains(lower, "doc") || strings.Contains(lower, "readme") {
			hasDocs = true
		}
	}

	if hasTests {
		patterns = append(patterns, "Includes test commits")
	}
	if hasDocs {
		patterns = append(patterns, "Maintains documentation")
	}

	return patterns
}

// analyzeCodingConventions extracts coding conventions from git diff stats
func analyzeCodingConventions(diffStats string) []string {
	conventions := []string{}

	lines := strings.Split(diffStats, "\n")
	testFiles := 0
	totalFiles := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "|") {
			totalFiles++
			if strings.Contains(line, "_test.") || strings.Contains(line, ".test.") ||
				strings.Contains(line, "/test/") || strings.Contains(line, "/tests/") {
				testFiles++
			}
		}
	}

	if totalFiles > 0 && testFiles > totalFiles/4 {
		conventions = append(conventions, "Strong test coverage (test files frequently modified)")
	}

	return conventions
}

// buildDirectoryStructureMap builds a map of directory to files for size assessment
func buildDirectoryStructureMap(rootPath string) map[string][]string {
	structure := make(map[string][]string)

	// Use a simple recursive walk for basic directory mapping
	// This is a simplified version - in production, you'd want more sophisticated handling
	cmd := exec.Command("find", ".", "-type", "f", "-name", "*.go", "-o", "-name", "*.js", "-o", "-name", "*.ts", "-o", "-name", "*.py", "-o", "-name", "*.java")
	cmd.Dir = rootPath
	if output, err := cmd.Output(); err == nil {
		files := strings.Split(string(output), "\n")
		for _, file := range files {
			file = strings.TrimSpace(file)
			if file != "" && !strings.Contains(file, "node_modules") && !strings.Contains(file, "vendor") {
				dir := "."
				if idx := strings.LastIndex(file, "/"); idx > 0 {
					dir = file[:idx]
				}
				structure[dir] = append(structure[dir], file)
			}
		}
	}

	return structure
}
