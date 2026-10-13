# Strict validation catches a dead rule; generate --check gates drift in CI.
Output docs/assets/validate.gif
Source docs/assets/tapes/settings.tape

Hide
Set TypingSpeed 1ms
Type "export PS1='$ ' && bash scripts/demo/validate.fixture.sh /tmp/ai-rulez-demo/validate && cd /tmp/ai-rulez-demo/validate && clear"
Enter
Wait /^\$$/
Show
Set TypingSpeed 28ms

Type "ai-rulez validate --strict -q"
Enter
Wait /^\$$/
Sleep 4s
Type "ai-rulez validate --explain AR101 | head -5"
Enter
Wait /^\$$/
Sleep 3s
Type "perl -pi -e 's/src/tests/' .ai-rulez/rules/testing.md"
Enter
Wait /^\$$/
Sleep 1s
Type "ai-rulez validate --strict 2>&1 | tail -1"
Enter
Wait /^\$$/
Sleep 3s
Type "clear"
Enter
Wait /^\$$/
Type "echo '- Mock only at system boundaries' >> .ai-rulez/rules/testing.md"
Enter
Wait /^\$$/
Sleep 1s
Type "ai-rulez generate --check; echo $?"
Enter
Wait /^\$$/
Sleep 3s
Type "ai-rulez generate 2>&1 | tail -1"
Enter
Wait /^\$$/
Sleep 2s
Type "ai-rulez generate --check; echo $?"
Enter
Wait /^\$$/
Sleep 4s
