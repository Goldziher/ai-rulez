# Monorepo: a root config and nested roots, one recursive generate, validation, and a token budget gate.
Output docs/assets/monorepo.gif
Source docs/assets/tapes/settings.tape

Hide
Type "export PS1='$ ' && set -o pipefail && bash scripts/demo/monorepo.fixture.sh /tmp/ai-rulez-demo/monorepo && cd /tmp/ai-rulez-demo/monorepo && clear"
Enter
Sleep 2s
Show

Type "# a monorepo: one root config, two nested roots"
Enter
Sleep 600ms
Type "find . -name .ai-rulez -not -path './.git/*'"
Enter
Sleep 2s
Type "ai-rulez -q generate --recursive"
Enter
Sleep 1500ms
Type "git ls-files -o --exclude-standard | grep -E 'AGENTS|CLAUDE'"
Enter
Sleep 3s
Type "ai-rulez validate --strict --recursive 2>&1 | grep -E 'strict|valid'"
Enter
Sleep 4s

Hide
Type "clear"
Enter
Show

Type "# guard the always-loaded surface with a token budget"
Enter
Sleep 600ms
Type "ai-rulez tokens --budget 400 2>&1 | grep -E 'Headline|Over'"
Enter
Sleep 4s
Type "echo $?"
Enter
Sleep 2s
Type "# add a big always-on rule"
Enter
Sleep 600ms
Type "cp style-guide.draft .ai-rulez/rules/style-guide.md"
Enter
Sleep 600ms
Type "ai-rulez tokens --budget 400 2>&1 | grep -E 'Headline|Over'"
Enter
Sleep 4s
Type "echo $?"
Enter
Sleep 4s
