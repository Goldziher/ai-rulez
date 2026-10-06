package importer

// remoteKind says how the content of a remote source is imported.
type remoteKind string

const (
	// remoteSkills is a rulesync source: named skills (and rules) of a git
	// repository. Skills become [[installed_skills]] pinned to the commit that was
	// fetched; rules are copied.
	remoteSkills remoteKind = "skills"
	// remotePackage is an APM dependency: a package whose .apm/ content is copied
	// into the tree, or a single skill that becomes an [[installed_skills]] entry.
	remotePackage remoteKind = "package"
)

// Remote is a source the input names but does not hold: a git repository that
// convert only reads when it is asked to fetch (--fetch). Plan records it; Plan
// itself never touches the network.
type Remote struct {
	Kind remoteKind
	// Origin is the input file and field that named the source, for findings.
	Origin string
	// URL is the https git URL.
	URL string
	// Ref is the tag, branch or commit as written; "" means the default branch.
	Ref string
	// Commit is a commit a lock file of the input pinned the source to; "" when none.
	Commit string
	// Path is the subdirectory of the repository the source selects.
	Path string
	// Skills and Rules name what to take from a remoteSkills source; empty Skills
	// with no Rules means every skill.
	Skills, Rules []string
	// SkillsPath and RulesPath are the directories of those, inside Path's repository.
	SkillsPath, RulesPath string
}
