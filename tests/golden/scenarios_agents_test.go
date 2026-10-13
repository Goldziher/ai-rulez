package golden

// agentsSourceFiles is a project whose knowledge lives in the open standards:
// the root AGENTS.md and the .agents directory.
func agentsSourceFiles(configPath, configBody string) map[string]string {
	return map[string]string{
		configPath:  configBody,
		"AGENTS.md": "# Demo\n\nUse tabs. Run `task test` before you push.\n",
		".agents/skills/review/SKILL.md": "---\nname: review\ndescription: Review a change before it merges\n---\n\n" +
			"# Review\n\nRead the diff first, then the tests.\n",
		".agents/skills/review/references/checklist.md": "- Is it tested?\n- Is it documented?\n",
		".agents/rules/go-style.md":                     "---\ndescription: Go style\nglobs: [\"**/*.go\"]\n---\n\nWrap errors with oops.\n",
		".agents/rules/always.md":                       "---\ndescription: Always on\n---\n\nKeep functions small.\n",
		".agents/agents/reviewer.md":                    "---\ndescription: Reviews changes\n---\n\nReview the change for correctness.\n",
	}
}

const agentsSourceConfig = "version = \"5.0\"\nname = \"demo\"\ngitignore = false\npresets = [\"claude\", \"cursor\", \"codex\"]\n"

// agentsSourceScenarios reads the standards and writes every other tool, never
// AGENTS.md or .agents: the same bytes after a second run, a clean check and a
// clean that leaves the source alone.
func agentsSourceScenarios() []scenario {
	steps := []step{
		runEnv(goldenEnv, "generate", "--yes"),
		runEnv(goldenEnv, "generate", "--yes"),
		runEnv(goldenEnv, "generate", "--check"),
		run("clean", "--yes"),
	}
	return []scenario{
		{
			name:  "agents-source",
			files: agentsSourceFiles(".agents/ai-rulez.toml", agentsSourceConfig),
			git:   true,
			full:  true,
			steps: steps,
		},
		{
			name:  "agents-source-in-ai-rulez-config",
			files: agentsSourceFiles(".ai-rulez/config.toml", agentsSourceConfig+"source = \"agents\"\n"),
			git:   true,
			full:  true,
			steps: steps,
		},
	}
}
