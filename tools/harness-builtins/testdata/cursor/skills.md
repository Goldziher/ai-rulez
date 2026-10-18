<!-- excerpt of https://cursor.com/docs/skills.md (2026-10-18) -->
## Built-in Cursor skills

Cursor includes a small set of built in skills to improve your general workflows. These skills are managed by Cursor and appear alongside the skills you add yourself.

| Skill                     | What it does                                                                                         |
| ------------------------- | ---------------------------------------------------------------------------------------------------- |
| `/automate`               | Creates Cursor Automations triggered by schedules, Slack messages, GitHub events, and other sources. |
| `/autopilot`              | Monitors a pull request and addresses feedback, conflicts, failing checks, and follow-up work.       |
| `/canvas`                 | Creates interactive React artifacts that render alongside the conversation.                          |
| `/create-hook`            | Creates Cursor hooks and updates `hooks.json` for agent lifecycle events.                            |
| `/create-rule`            | Creates Cursor rules with the appropriate scope and instructions.                                    |
| `/create-skill`           | Creates Agent Skills, including their structure and `SKILL.md` files.                                |
| `/create-subagent`        | Creates custom subagents with focused roles and delegation instructions.                             |
| `/cursor-blame`           | Investigates AI-authored changes and the prompts that produced them.                                 |
| `/loop`                   | Runs a prompt or skill repeatedly at a specified interval.                                           |
| `/migrate-to-skills`      | Converts eligible dynamic rules and slash commands into Agent Skills.                                |
| `/review`                 | Selects and runs the appropriate code-review agent.                                                  |
| `/review-bugbot`          | Reviews code for likely bugs and regressions with Bugbot.                                            |
| `/review-security`        | Reviews code for security vulnerabilities with Security Review.                                      |
| `/sdk`                    | Helps you build applications and integrations with the Cursor SDK.                                   |
| `/shell`                  | Runs the provided text as a literal shell command.                                                   |
| `/split-to-prs`           | Splits large changes into smaller pull requests.                                                     |
| `/statusline`             | Configures the Cursor CLI status line.                                                               |
| `/update-cli-config`      | Updates Cursor CLI settings in `~/.cursor/cli-config.json`.                                          |
| `/update-cursor-settings` | Finds and updates the appropriate Cursor or VS Code setting.                                         |

You can run any built-in skill by typing `/` in Agent chat and selecting its name. Agent may also use some built-in skills automatically when your request clearly matches their purpose.

