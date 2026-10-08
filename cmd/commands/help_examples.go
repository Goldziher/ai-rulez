package commands

import "github.com/spf13/cobra"

// helpExamples are the invocations shown under "Examples:" in the help of the
// most-used commands. help_examples_test.go parses every line against the real
// command tree, and the CLI end-to-end tests run the read-only ones, so an
// example cannot name a flag or subcommand that does not exist.
func helpExamples() map[*cobra.Command]string {
	return map[*cobra.Command]string{
		InitCmd: `  ai-rulez init
  ai-rulez init my-project --yes
  ai-rulez init --from auto --yes
  ai-rulez init --domains backend,frontend --yes`,
		GenerateCmd: `  ai-rulez generate
  ai-rulez generate --dry-run
  ai-rulez generate --profile backend
  ai-rulez generate --check
  ai-rulez generate --recursive
  ai-rulez generate --locked`,
		ValidateCmd: `  ai-rulez validate
  ai-rulez validate --strict
  ai-rulez validate --changed
  ai-rulez validate --explain AR001
  ai-rulez validate --format sarif --output ai-rulez.sarif
  ai-rulez validate --fix --dry-run`,
		LockCmd: `  ai-rulez lock
  ai-rulez lock --check
  ai-rulez lock --diff
  ai-rulez lock --outdated
  ai-rulez lock --content-only`,
		ApproveCmd: `  ai-rulez approve --list
  ai-rulez approve --diff include:shared
  ai-rulez approve include:shared --reviewer alice@example.org --note "read run.sh" --yes
  ai-rulez approve --revoke include:shared`,
		VerifyCmd: `  ai-rulez verify
  ai-rulez verify --approvals
  ai-rulez verify --attestation
  ai-rulez verify --self`,
		PublishCmd: `  ai-rulez publish --dry-run
  ai-rulez publish --marketplace --sbom
  ai-rulez publish --to github-release --execute --yes`,
		ImportCmd: `  ai-rulez import okf ./docs/okf --dry-run
  ai-rulez import okf ./docs/okf --into context --domain kb`,
		ConvertCmd: `  ai-rulez convert --list
  ai-rulez convert --dry-run
  ai-rulez convert --write
  ai-rulez convert --from rulesync --dry-run
  ai-rulez convert --write --merge`,
		AddCmd: `  ai-rulez add rule code-quality
  ai-rulez add rule api-design --domain backend --priority high
  ai-rulez add skill code-reviewer
  ai-rulez add rule my-scratch-notes --local`,
		RemoveCmd: `  ai-rulez remove rule code-quality
  ai-rulez remove rule api-design --domain backend
  ai-rulez remove skill code-reviewer --yes`,
		RolesCmd: `  ai-rulez roles list
  ai-rulez roles show engineer
  ai-rulez roles resolve engineer
  ai-rulez roles list --format json`,
		TelemetryCmd: `  ai-rulez telemetry status
  ai-rulez telemetry doctor
  ai-rulez telemetry preview --limit 2
  ai-rulez telemetry export --to file usage.ndjson`,
	}
}

// applyHelpExamples attaches the examples; root.go's init calls it once every
// command variable exists.
func applyHelpExamples() {
	for cmd, example := range helpExamples() {
		cmd.Example = example
	}
}
