# One source, many tools: init, generate, see what each tool received.
Output docs/assets/quickstart.gif
Source docs/assets/tapes/settings.tape

Hide
Type "export PS1='$ ' && bash scripts/demo/fixture.sh quickstart /tmp/ai-rulez-demo/quickstart && cd /tmp/ai-rulez-demo/quickstart && clear"
Enter
Sleep 2s
Show

Type "# one source of truth for every AI coding tool"
Enter
Sleep 1s
Type "tail -n +11 .ai-rulez/rules/code-quality.md"
Enter
Sleep 4s
Type "grep presets .ai-rulez/config.toml | tail -1"
Enter
Sleep 2s
Type "ai-rulez generate"
Enter
Sleep 3s
Type "git ls-files -o --exclude-standard | grep -v '^.ai-rulez/' | sort"
Enter
Sleep 5s
