package lint

// Codes of the SBOM findings (internal/sbom, docs/sbom.md). AR75x is the SBOM
// block of the allocation table in docs/strict-validation.md. `ai-rulez sbom`
// reports them (AR750 and AR751 with --strict-pins, AR752 with --require-lock,
// AR753 with --check); the registry lists them so the codes, names and
// documentation stay in one place.
const (
	CodeSBOMUnpinned      = "AR750"
	CodeSBOMUnknownCoords = "AR751"
	CodeSBOMLockStale     = "AR752"
	CodeSBOMDrift         = "AR753"
)

func init() {
	registerRules(
		RuleInfo{CodeSBOMUnpinned, "sbom-component-unpinned", SeverityInfo, "an MCP package or remote source in the SBOM cannot be given an exact version: a range, a tag such as latest, or no commit pin"},
		RuleInfo{CodeSBOMUnknownCoords, "sbom-coordinates-unknown", SeverityInfo, "an MCP server has no package URL in the SBOM: its command is not a recognised launcher and no package is declared"},
		RuleInfo{CodeSBOMLockStale, "sbom-lock-out-of-sync", SeverityError, "sbom --require-lock found no ai-rulez.lock, or one that no longer matches the sources, so the SBOM would not describe what the lock pins"},
		RuleInfo{CodeSBOMDrift, "sbom-drift", SeverityError, "sbom --check found the committed SBOM different from the one generated now, or no committed SBOM"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodeSBOMUnpinned: {
			Why:  "A scanner matches a package URL against advisories by version. A package that floats (latest, a range, an image tag, an include on a branch) is a different release on every machine, so the SBOM cannot say what runs. The purl then omits the version.",
			Bad:  "`npx -y some-mcp-server@latest`, `docker run img:1.0`, or an include with `ref = \"main\"` and no lock",
			Good: "`npx -y some-mcp-server@1.4.2`, an image digest (`img@sha256:...`), or `ai-rulez lock` to pin the include to a commit",
		},
		CodeSBOMUnknownCoords: {
			Why:  "Vulnerability scanners need a package URL to find the server. A server started from a binary path or a script has none that can be guessed from the command line.",
			Bad:  "`command = \"/usr/local/bin/my-server\"` with no `package`",
			Good: "`package = \"pkg:npm/%40scope/server@1.4.2\"` on the `[[mcp_servers]]` entry",
		},
		CodeSBOMLockStale: {
			Why:  "With --require-lock the SBOM is a statement about the pinned content. If the lock is missing or stale the document would describe the working tree instead.",
			Bad:  "Editing a rule and running `ai-rulez sbom --require-lock` before `ai-rulez lock`",
			Good: "Run `ai-rulez lock`, then `ai-rulez sbom --require-lock`",
		},
		CodeSBOMDrift: {
			Why:  "A committed SBOM that no longer matches the configuration misleads whoever reads it. The check names the components that were added, removed or changed; the ai-rulez version is ignored.",
			Bad:  "Adding a skill without regenerating `sbom.cdx.json`",
			Good: "`ai-rulez sbom -o sbom.cdx.json` and commit the result",
		},
	})
	for _, code := range []string{CodeSBOMUnpinned, CodeSBOMUnknownCoords, CodeSBOMLockStale, CodeSBOMDrift} {
		SetAnalyzer(code, AnalyzerConfig, ScopeBundle)
	}
}
