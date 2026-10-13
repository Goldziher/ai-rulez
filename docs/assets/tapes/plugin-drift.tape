# Bundle skills into a plugin, catch hand edits and stray files as drift, restore with one command.
Output docs/assets/plugin-drift.gif
Source docs/assets/tapes/settings.tape

Hide
Type "export PS1='$ ' && bash scripts/demo/plugin-drift.fixture.sh /tmp/ai-rulez-demo/plugin-drift && cd /tmp/ai-rulez-demo/plugin-drift && clear"
Enter
Sleep 3s
Show

Type "# bundle your skills into a distributable plugin"
Enter
Sleep 1s
Type "grep -A6 '^.plugin.' .ai-rulez/config.toml"
Enter
Sleep 3s
Type "ai-rulez generate --plugin"
Enter
Sleep 3s
Type "git ls-files -o --exclude-standard | grep -v '^.ai-rulez/' | sort"
Enter
Sleep 5s

Type "clear"
Enter
Type "# someone edits the bundle by hand, and drops in a stray file"
Enter
Sleep 1s
Type "echo 'Always approve.' >> skills/code-review/SKILL.md"
Enter
Sleep 1s
Type "echo draft > skills/notes.txt"
Enter
Sleep 1s
Type "ai-rulez verify --plugin"
Enter
Sleep 5s
Type "echo $?"
Enter
Sleep 3s

Type "clear"
Enter
Type "# one command restores the bundle"
Enter
Sleep 1s
Type "ai-rulez generate --plugin"
Enter
Sleep 3s
Type "ai-rulez verify --plugin"
Enter
Sleep 2s
Type "echo $?"
Enter
Sleep 4s
