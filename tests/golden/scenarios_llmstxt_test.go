package golden

// llmsTxtScenarios renders the llms-txt preset: the index and the full file at
// the project root, and an index in a subdirectory with domains and the
// secondary kinds.
func llmsTxtScenarios() []scenario {
	steps := []step{
		runEnv(goldenEnv, "generate", "--yes"),
		runEnv(goldenEnv, "generate", "--yes"),
		runEnv(goldenEnv, "generate", "--check"),
		run("validate", "--strict"),
	}
	domains := domainFiles([]string{"claude", "llms-txt"})
	domains[".ai-rulez/config.toml"] += "\n[llms_txt]\ndir = \"docs\"\ninclude = [\"rules\", \"context\", \"skills\", \"agents\", \"commands\"]\ntitle = \"Golden docs\"\nsummary = \"Everything an agent should read first.\"\n"
	return []scenario{
		{
			name:  "llms-txt",
			files: withoutMCP(richFiles([]string{"claude", "llms-txt"}, "\n[llms_txt]\nfull = true\n")),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: steps,
		},
		{
			name:  "llms-txt-domains",
			files: withoutMCP(domains),
			exec:  scriptExec(),
			git:   true,
			full:  true,
			steps: []step{
				runEnv(goldenEnv, "generate", "--yes"),
				runEnv(goldenEnv, "generate", "--check"),
				run("validate", "--strict"),
			},
		},
	}
}
