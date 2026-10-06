package lint

// Codes of the `ai-rulez publish` report (internal/publish, docs/publish.md).
// They are registered here so `validate --explain AR9N0` works and the AR9
// block allocation is checked in one place; `validate` never emits them.
// TestAllocatedBlocksCoverLiteralsInOtherPackages keeps them equal to the
// constants in internal/publish.
const (
	CodePublishPreflight    = "AR9N0"
	CodePublishBundleUnsafe = "AR9N1"
	CodePublishSecret       = "AR9N2"
	CodePublishSource       = "AR9N3"
	CodePublishTarget       = "AR9N4"
	CodePublishVerify       = "AR9N5"
)

func init() {
	for _, code := range []string{
		CodePublishPreflight, CodePublishBundleUnsafe, CodePublishSecret,
		CodePublishSource, CodePublishTarget, CodePublishVerify,
	} {
		SetAnalyzer(code, AnalyzerPlugin, ScopeBundle)
	}
	registerRules(
		RuleInfo{CodePublishPreflight, "publish-preflight-failed", SeverityError, "a preflight gate of `publish` failed: strict validation, the lock check or plugin verification (reported by publish, never by validate)"},
		RuleInfo{CodePublishBundleUnsafe, "publish-bundle-unsafe", SeverityError, "the bundle holds a symlink, a path outside the project or a name that cannot name a release file (publish only)"},
		RuleInfo{CodePublishSecret, "publish-secret-found", SeverityError, "the secret scan of the bundle found a credential (publish only)"},
		RuleInfo{CodePublishSource, "publish-source-unreleasable", SeverityError, "the plugin has no version, or the source tree is dirty or has no commit (publish only)"},
		RuleInfo{CodePublishTarget, "publish-target-failed", SeverityError, "the upload step failed: gh is missing, the release exists or gh exited non-zero (publish only)"},
		RuleInfo{CodePublishVerify, "publish-verify-mismatch", SeverityError, "`publish verify` found a digest, manifest or archive mismatch in a dist directory (publish only)"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodePublishPreflight: {
			Why:  "A bundle must be reviewed, locked and generated before anyone downloads it, so `publish` runs `validate --strict`, `lock --check` and `verify --plugin` first and writes nothing when one fails.",
			Bad:  "A skill edited after `ai-rulez lock`, or plugin files hand-edited since `generate --plugin`",
			Good: "Run `ai-rulez lock` and `ai-rulez generate --plugin`, commit, and publish again",
		},
		CodePublishBundleUnsafe: {
			Why:  "A release archive must hold only regular files below its root, so a symlink or an escaping path would let the archive reach files that were never reviewed.",
			Bad:  "A plugin skill directory that is a symlink to `~/skills`",
			Good: "Copy the content into the project instead of linking it",
		},
		CodePublishSecret: {
			Why:  "A published archive cannot be recalled. The same patterns as the security scan (cloud keys, tokens, private keys, credential assignments) are applied to every file of the bundle.",
			Bad:  "A hook script that carries `AWS_SECRET_ACCESS_KEY=...` literally",
			Good: "Read the value from the environment at run time and rotate the leaked credential",
		},
		CodePublishSource: {
			Why:  "A release is identified by its commit and `[plugin] version`; a dirty tree or a missing version makes the bundle impossible to reproduce.",
			Bad:  "`[plugin]` without `version`, or uncommitted changes next to the generated bundle",
			Good: "Set `version`, commit, and publish; use `--allow-dirty` only for a throwaway build",
		},
		CodePublishTarget: {
			Why:  "Uploads go through the platform's own CLI so ai-rulez never handles credentials. The step stops when that CLI is missing, the release already exists (releases are immutable) or it fails.",
			Bad:  "`publish --to github-release --execute --yes` without `gh` on PATH, or for a tag that already has a release",
			Good: "Install and authenticate `gh`, push the tag, and use `--force` only to replace the assets of an existing release",
		},
		CodePublishVerify: {
			Why:  "`publish verify` recomputes SHA256SUMS, the manifest and the archive contents, so a file changed after the build, or an archive that no longer matches its manifest, is caught before it is installed.",
			Bad:  "An edited `ai-rulez.lock` next to a manifest that records the original digest",
			Good: "Download the release again, or rebuild it with `ai-rulez publish`",
		},
	})
}
