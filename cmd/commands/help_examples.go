package commands

import (
	"strings"

	"github.com/spf13/cobra"
)

// helpExampleSpec holds the invocations shown under "Examples:" in the help of
// the commands that do not carry an Example of their own. A line "## <path>" opens
// the examples of the command with that path below the root ("add rule",
// "telemetry report evals"; "##" alone is the root); the indented lines that
// follow are the examples. Every command of the tree has examples:
// help_examples_test.go walks the tree, fails when one has none and parses every
// line against the real flags and subcommands, so an example cannot name a flag
// or subcommand that does not exist.
const helpExampleSpec = `
##
  ai-rulez init
  ai-rulez validate
  ai-rulez generate
  ai-rulez generate --check
  ai-rulez doctor
  ai-rulez completion zsh
## init
  ai-rulez init
  ai-rulez init my-project --yes
  ai-rulez init --from auto --yes
  ai-rulez init --domains backend,frontend --yes
## generate
  ai-rulez generate
  ai-rulez generate --dry-run
  ai-rulez generate --profile backend
  ai-rulez generate --check
  ai-rulez generate --recursive
  ai-rulez generate --locked
## validate
  ai-rulez validate
  ai-rulez validate --strict
  ai-rulez validate --changed
  ai-rulez validate --explain AR001
  ai-rulez validate --format sarif --output ai-rulez.sarif
  ai-rulez validate --fix --dry-run
## lock
  ai-rulez lock
  ai-rulez lock --check
  ai-rulez lock --diff
  ai-rulez lock --outdated
  ai-rulez lock --content-only
## approve
  ai-rulez approve --list
  ai-rulez approve --diff include:shared
  ai-rulez approve include:shared --reviewer alice@example.org --note "read run.sh" --yes
  ai-rulez approve --revoke include:shared
## verify
  ai-rulez verify
  ai-rulez verify --approvals
  ai-rulez verify --attestation
  ai-rulez verify --self
## publish
  ai-rulez publish --dry-run
  ai-rulez publish --marketplace --sbom
  ai-rulez publish --to github-release --execute --yes
## publish emit
  ai-rulez publish emit ard
  ai-rulez publish emit agent-plugins --out dist/emit
  ai-rulez publish emit cursor-team-marketplace --profile backend
## publish verify
  ai-rulez publish verify dist
  ai-rulez publish verify dist --key cosign.pub --require-signature
  ai-rulez publish verify ghcr.io/acme/rules:1.0.0 --identity ci@acme.example --issuer https://token.actions.githubusercontent.com
## import
  ai-rulez import okf ./docs/okf --dry-run
  ai-rulez import okf ./docs/okf --into context --domain kb
## import okf
  ai-rulez import okf ./docs/okf --dry-run
  ai-rulez import okf https://github.com/acme/kb@v1.2.0#docs --domain kb
  ai-rulez import okf ./docs/okf --into context --force
## export
  ai-rulez export okf --out ./bundle
  ai-rulez export okf --check
## export okf
  ai-rulez export okf --out ./bundle
  ai-rulez export okf --role engineer --out ./bundle
  ai-rulez export okf --profile backend --out ./bundle --index-style frontmatter
  ai-rulez export okf --check
## okf
  ai-rulez okf validate ./docs/okf
  ai-rulez okf validate https://github.com/acme/kb@v1.2.0#docs --fail-on warning
## okf validate
  ai-rulez okf validate ./docs/okf
  ai-rulez okf validate ./docs/okf --fail-on warning
  ai-rulez okf validate https://github.com/acme/kb@v1.2.0#docs --format json
## convert
  ai-rulez convert --list
  ai-rulez convert --dry-run
  ai-rulez convert --write
  ai-rulez convert --from rulesync --dry-run
  ai-rulez convert --write --merge
## add
  ai-rulez add rule code-quality
  ai-rulez add rule api-design --domain backend --priority high
  ai-rulez add skill code-reviewer
  ai-rulez add rule my-scratch-notes --local
## add rule
  ai-rulez add rule code-quality
  ai-rulez add rule api-design --domain backend --priority high
  ai-rulez add rule go-style --targets claude,cursor --content "Use gofmt."
  ai-rulez add rule my-scratch-notes --local
## add context
  ai-rulez add context architecture
  ai-rulez add context api-notes --domain backend
  ai-rulez add context glossary --content "Tenant: one customer organization."
## add skill
  ai-rulez add skill code-reviewer
  ai-rulez add skill db-migrations --domain backend --description "Write safe schema migrations"
  ai-rulez add skill my-helper --local
## add agent
  ai-rulez add agent reviewer
  ai-rulez add agent release-manager --domain ops --description "Cuts releases"
  ai-rulez add agent my-agent --local
## add command
  ai-rulez add command deploy
  ai-rulez add command lint-all --domain backend --description "Run every linter"
  ai-rulez add command scratch --local
## add check
  ai-rulez add check no-todos --severity medium
  ai-rulez add check sql-injection --domain backend --severity high --targets "src/**/*.go"
  ai-rulez add check secrets --description "No credentials in code" --tools claude
## remove
  ai-rulez remove rule code-quality
  ai-rulez remove rule api-design --domain backend
  ai-rulez remove skill code-reviewer --yes
## remove rule
  ai-rulez remove rule code-quality
  ai-rulez remove rule api-design --domain backend
  ai-rulez remove rule my-scratch-notes --local --yes
## remove context
  ai-rulez remove context architecture
  ai-rulez remove context api-notes --domain backend --yes
## remove skill
  ai-rulez remove skill code-reviewer
  ai-rulez remove skill db-migrations --domain backend --yes
## remove agent
  ai-rulez remove agent reviewer
  ai-rulez remove agent release-manager --domain ops --yes
## remove command
  ai-rulez remove command deploy
  ai-rulez remove command lint-all --domain backend --yes
## remove check
  ai-rulez remove check no-todos
  ai-rulez remove check sql-injection --domain backend --yes
## list
  ai-rulez list rules
  ai-rulez list skills --domain backend
  ai-rulez list --placement
  ai-rulez list rules --format json
## list rules
  ai-rulez list rules
  ai-rulez list rules --domain backend
  ai-rulez list rules --local
  ai-rulez list rules --format json
## list context
  ai-rulez list context
  ai-rulez list context --domain backend --format json
## list skills
  ai-rulez list skills
  ai-rulez list skills --domain backend
  ai-rulez list skills --format json
## list agents
  ai-rulez list agents
  ai-rulez list agents --domain ops --format json
## list commands
  ai-rulez list commands
  ai-rulez list commands --domain backend --format json
## list checks
  ai-rulez list checks
  ai-rulez list checks --domain backend --format json
## show
  ai-rulez show rule code-quality
  ai-rulez show skill code-reviewer --domain backend
  ai-rulez show rule code-quality --format json
## show rule
  ai-rulez show rule code-quality
  ai-rulez show rule api-design --domain backend
  ai-rulez show rule code-quality --format json
## show context
  ai-rulez show context architecture
  ai-rulez show context api-notes --domain backend
## show skill
  ai-rulez show skill code-reviewer
  ai-rulez show skill db-migrations --domain backend --format json
## show agent
  ai-rulez show agent reviewer
  ai-rulez show agent release-manager --domain ops
## show command
  ai-rulez show command deploy
  ai-rulez show command lint-all --domain backend
## show check
  ai-rulez show check no-todos
  ai-rulez show check sql-injection --domain backend --format json
## edit
  ai-rulez edit rule code-quality --content "Prefer small functions."
  ai-rulez edit skill code-reviewer --domain backend --content "Review for races."
  ai-rulez edit rule code-quality --priority high
## edit rule
  ai-rulez edit rule code-quality --content "Prefer small functions."
  ai-rulez edit rule api-design --domain backend --priority high
  ai-rulez edit rule go-style --targets claude,cursor
## edit context
  ai-rulez edit context architecture --content "Services talk over Kafka."
  ai-rulez edit context api-notes --domain backend --priority high
## edit skill
  ai-rulez edit skill code-reviewer --content "Review for data races."
  ai-rulez edit skill db-migrations --domain backend --priority high
## edit agent
  ai-rulez edit agent reviewer --content "You review pull requests."
  ai-rulez edit agent release-manager --domain ops --local
## edit command
  ai-rulez edit command deploy --content "Run the deploy script."
  ai-rulez edit command lint-all --domain backend --local
## edit check
  ai-rulez edit check no-todos --severity high
  ai-rulez edit check sql-injection --domain backend --targets "src/**/*.go"
  ai-rulez edit check secrets --description "No credentials in code"
## domain
  ai-rulez domain add backend
  ai-rulez domain list
  ai-rulez domain remove backend --yes
## domain add
  ai-rulez domain add backend
  ai-rulez domain add frontend --description "React application"
## domain list
  ai-rulez domain list
  ai-rulez domain list --format json
## domain remove
  ai-rulez domain remove backend
  ai-rulez domain remove backend --yes
## profile
  ai-rulez profile add backend backend shared
  ai-rulez profile set-default backend
  ai-rulez profile list
## profile add
  ai-rulez profile add backend backend shared
  ai-rulez profile add full backend frontend --set-default
  ai-rulez profile add mine backend --local
## profile list
  ai-rulez profile list
  ai-rulez profile list --format json
## profile remove
  ai-rulez profile remove backend
  ai-rulez profile remove backend --yes
## profile set-default
  ai-rulez profile set-default backend
  ai-rulez profile set-default full --local
## include
  ai-rulez include add shared https://github.com/acme/rules
  ai-rulez include list
  ai-rulez include remove shared --yes
## include add
  ai-rulez include add shared https://github.com/acme/rules
  ai-rulez include add shared https://github.com/acme/rules --ref v1.2.0 --path rules
  ai-rulez include add shared ../shared-rules --merge-strategy local-override
  ai-rulez include add mine ../my-rules --local
## include list
  ai-rulez include list
  ai-rulez include list --format json
## include remove
  ai-rulez include remove shared
  ai-rulez include remove shared --yes
## skill
  ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg
  ai-rulez skill list
  ai-rulez skill update
  ai-rulez skill remove kreuzberg --yes
## skill install
  ai-rulez skill install kreuzberg --source https://github.com/kreuzberg-dev/kreuzberg
  ai-rulez skill install ai-rulez --source https://github.com/Goldziher/ai-rulez
  ai-rulez skill install my-skill --source ./local-repo --path custom/path
## skill list
  ai-rulez skill list
  ai-rulez skill list --format json
## skill remove
  ai-rulez skill remove kreuzberg
  ai-rulez skill remove kreuzberg --yes
## skill update
  ai-rulez skill update
  ai-rulez skill update kreuzberg
## local
  ai-rulez local init
  ai-rulez local set default dev
  ai-rulez local show
  ai-rulez local unset default
## local init
  ai-rulez local init
## local path
  ai-rulez local path
## local set
  ai-rulez local set default dev
  ai-rulez local set 'presets' '["codex", "!cursor"]'
  ai-rulez local set mcp_servers.github.command npx
  printf %s "$TOKEN" | ai-rulez local set mcp_servers.github.env.GITHUB_TOKEN --stdin
## local show
  ai-rulez local show
  ai-rulez local show --format json
  ai-rulez local show --reveal
## local unset
  ai-rulez local unset default
  ai-rulez local unset mcp_servers.github
## builtins
  ai-rulez builtins list
  ai-rulez builtins show security
## builtins list
  ai-rulez builtins list
  ai-rulez builtins list --format json
## builtins show
  ai-rulez builtins show security
  ai-rulez builtins show security --format json
## roles
  ai-rulez roles list
  ai-rulez roles show engineer
  ai-rulez roles resolve engineer
  ai-rulez roles list --format json
## roles list
  ai-rulez roles list
  ai-rulez roles list --format json
## roles show
  ai-rulez roles show engineer
  ai-rulez roles show engineer --format json
## roles resolve
  ai-rulez roles resolve engineer
  ai-rulez roles resolve engineer --format json
## telemetry
  ai-rulez telemetry status
  ai-rulez telemetry doctor
  ai-rulez telemetry preview --limit 2
  ai-rulez telemetry export --to file usage.ndjson
## telemetry status
  ai-rulez telemetry status
## telemetry doctor
  ai-rulez telemetry doctor --format json
## telemetry enable
  ai-rulez telemetry enable --endpoint https://otel.example.org:4318
## telemetry disable
  ai-rulez telemetry disable
## telemetry preview
  ai-rulez telemetry preview --limit 2
## telemetry flush
  ai-rulez telemetry flush --timeout 10s
## telemetry hook
  ai-rulez telemetry hook --harness claude
## telemetry record
  cat hook-event.json | ai-rulez telemetry record --harness claude
## telemetry prune
  ai-rulez telemetry prune --keep-days 30 --dry-run
## telemetry export
  ai-rulez telemetry export --to file usage.ndjson
## telemetry feedback
  ai-rulez telemetry feedback code-reviewer --kind stale
## telemetry report
  ai-rulez telemetry report
  ai-rulez telemetry report --format json
  ai-rulez telemetry report evals --results results.json
## telemetry report evals
  ai-rulez telemetry report evals --results results.json
  ai-rulez telemetry report evals --results results.json --min-pass-rate 0.8 --format json
## eval
  ai-rulez eval run --estimate
  ai-rulez eval run code-reviewer --runner command --runner-command "./run-case.sh"
  ai-rulez eval import --from tessl ./scenarios --dry-run
## eval run
  ai-rulez eval run --estimate
  ai-rulez eval run code-reviewer --runner command --runner-command "./run-case.sh"
  ai-rulez eval run --mode activation --surface retrieval
  ai-rulez eval run code-reviewer --changed-only --max-cost 5
## eval import
  ai-rulez eval import --from tessl ./scenarios --dry-run
  ai-rulez eval import --from tessl ./scenarios --skill code-reviewer --lift-assertions
## eval calibrate-estimate
  ai-rulez eval calibrate-estimate
  ai-rulez eval calibrate-estimate --harness claude --min-samples 5 --format json
## review
  ai-rulez review
  ai-rulez review code-reviewer --semantic --estimate
  ai-rulez review --since main --gate
  ai-rulez review --format sarif --out review.sarif
## review calibrate
  ai-rulez review calibrate --rubric skill --max-cost 2
  ai-rulez review calibrate --compare calibration.json
## review explain
  ai-rulez review explain AR9G1
  ai-rulez review explain AR9G8
## review fix
  ai-rulez review fix code-reviewer
  ai-rulez review fix code-reviewer --patch fix.patch
  ai-rulez review fix --apply --max-cost 2
## rubric
  ai-rulez rubric list
  ai-rulez rubric show
  ai-rulez rubric lint
## rubric list
  ai-rulez rubric list
## rubric show
  ai-rulez rubric show
  ai-rulez rubric show skill
## rubric lint
  ai-rulez rubric lint
  ai-rulez rubric lint skill
## improve
  ai-rulez improve run code-reviewer --with "my-optimizer --skill {skill}" --dry-run
  ai-rulez improve show 20260101-code-reviewer
  ai-rulez improve apply 20260101-code-reviewer
## improve run
  ai-rulez improve run code-reviewer --with "my-optimizer" --dry-run
  ai-rulez improve run code-reviewer --adapter gepa --max-rounds 3 --max-cost 10
## improve show
  ai-rulez improve show 20260101-code-reviewer
## improve apply
  ai-rulez improve apply 20260101-code-reviewer --yes
## improve pr
  ai-rulez improve pr 20260101-code-reviewer --draft
## improve clean
  ai-rulez improve clean --dry-run
## improve adapters
  ai-rulez improve adapters
## verifiers
  ai-rulez verifiers list
  ai-rulez verifiers run
  ai-rulez verifiers test
  ai-rulez verifiers explain no-todos
## verifiers list
  ai-rulez verifiers list
  ai-rulez verifiers list --format json
## verifiers run
  ai-rulez verifiers run
  ai-rulez verifiers run --since main
  ai-rulez verifiers run --staged --fail-on warning
  ai-rulez verifiers run --format sarif --out verifiers.sarif
## verifiers explain
  ai-rulez verifiers explain no-todos
## verifiers test
  ai-rulez verifiers test
  ai-rulez verifiers test no-todos
## verifiers calibrate
  ai-rulez verifiers calibrate --estimate
  ai-rulez verifiers calibrate no-vague-wording --allow-llm --max-cost 2
## verifiers suggest
  ai-rulez verifiers suggest no-todos --allow-llm --estimate
  ai-rulez verifiers suggest no-todos --allow-llm --max-proposals 3
## scanners
  ai-rulez scanners list
  ai-rulez scanners doctor --all
## scanners list
  ai-rulez scanners list
  ai-rulez scanners list --format json
## scanners doctor
  ai-rulez scanners doctor --all
  ai-rulez scanners doctor semgrep
## scan
  ai-rulez scan
  ai-rulez scan --recursive
  ai-rulez scan --format sarif --output scan.sarif
  ai-rulez scan --changed --fail-on warning
## clean
  ai-rulez clean --dry-run
  ai-rulez clean --yes
  ai-rulez clean --profile backend --yes
## cost
  ai-rulez cost
  ai-rulez cost --top 20
  ai-rulez cost --target claude --budget 8000
  ai-rulez cost --format json
## tokens
  ai-rulez tokens
  ai-rulez tokens --budget 20000
  ai-rulez tokens --by-role
  ai-rulez tokens --format json
## doctor
  ai-rulez doctor
  ai-rulez doctor --strict
  ai-rulez doctor --format json
## guard
  echo '{"tool_name":"Edit","tool_input":{"file_path":"CLAUDE.md"}}' | ai-rulez guard
## mcp
  ai-rulez mcp
  ai-rulez mcp --serve-skills
  ai-rulez mcp --serve-skills --profile backend --no-watch
  ai-rulez mcp --allow list_rules --allow generate_outputs
## sbom
  ai-rulez sbom
  ai-rulez sbom --format spdx-json --output sbom.spdx.json
  ai-rulez sbom --check
## sign
  ai-rulez sign --lock --keyless
  ai-rulez sign --lock --key cosign.key --output ai-rulez.lock.sigstore.json
  ai-rulez sign --sbom sbom.cdx.json --keyless
## trust
  ai-rulez trust update
## trust update
  ai-rulez trust update
## update
  ai-rulez update --dry-run
  ai-rulez update shared
  ai-rulez update --kind include --write-config
## version
  ai-rulez version
  ai-rulez version --format json
## llm
  ai-rulez llm doctor
  ai-rulez llm estimate prompt.md
## llm doctor
  ai-rulez llm doctor
  ai-rulez llm doctor --ping
## llm estimate
  ai-rulez llm estimate prompt.md
  ai-rulez llm estimate prompt.md --max-output 2000
## completion
  ai-rulez completion bash
  ai-rulez completion zsh
  ai-rulez completion fish
  ai-rulez completion powershell
## completion bash
  source <(ai-rulez completion bash)
  ai-rulez completion bash > /etc/bash_completion.d/ai-rulez
## completion zsh
  source <(ai-rulez completion zsh)
  ai-rulez completion zsh > "${fpath[1]}/_ai-rulez"
## completion fish
  ai-rulez completion fish | source
  ai-rulez completion fish > ~/.config/fish/completions/ai-rulez.fish
## completion powershell
  ai-rulez completion powershell | Out-String | Invoke-Expression
  ai-rulez completion powershell > ai-rulez.ps1
`

// helpExamples returns the examples by command path.
func helpExamples() map[string]string {
	examples := map[string]string{}
	var path string
	var lines []string
	flush := func() {
		if lines != nil {
			examples[path] = strings.Join(lines, "\n")
		}
	}
	for _, line := range strings.Split(helpExampleSpec, "\n") {
		if line == "##" || strings.HasPrefix(line, "## ") {
			flush()
			path, lines = strings.TrimSpace(strings.TrimPrefix(line, "##")), []string{}
			continue
		}
		if strings.TrimSpace(line) != "" && lines != nil {
			lines = append(lines, line)
		}
	}
	flush()
	return examples
}

// applyHelpExamples attaches the examples; root.go's init calls it once every
// command exists. A command that declares an Example of its own keeps it.
func applyHelpExamples(root *cobra.Command) {
	for path, example := range helpExamples() {
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil || cmd == nil || cmd.CommandPath() != strings.TrimSpace(root.Name()+" "+path) {
			continue
		}
		if cmd.Example == "" {
			cmd.Example = example
		}
	}
}
