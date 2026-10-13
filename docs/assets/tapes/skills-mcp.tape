# Static skills versus skills served on demand over MCP: the always-loaded cost drops, find_skill and load_skill fetch the rest.
Output docs/assets/skills-mcp.gif
Source docs/assets/tapes/settings.tape

Hide
Type "export PS1='$ ' && bash scripts/demo/skills-mcp.fixture.sh /tmp/ai-rulez-demo/skills-mcp && cd /tmp/ai-rulez-demo/skills-mcp && clear"
Enter
Sleep 3s
Show

Type "# twelve skills, all delivered statically"
Enter
Type "ls -C .ai-rulez/skills"
Enter
Sleep 3s
Type "ai-rulez tokens | grep -E 'always loaded|skill listing|Headline'"
Enter
Sleep 4s

Ctrl+L
Type "# serve them over MCP instead"
Enter
Type `perl -pi -e 's/"static"/"served"/' .ai-rulez/config.toml`
Enter
Type "tail -n 2 .ai-rulez/config.toml"
Enter
Sleep 2s
Type "ai-rulez generate"
Enter
Sleep 5s
Type "ai-rulez tokens | grep -E 'always loaded|skill listing|Headline'"
Enter
Sleep 4s

Ctrl+L
Type "# the assistant side: find a skill, then load it"
Enter
Type `python3 mcp-call.py find_skill '{"task": "add a column to users"}'`
Enter
Sleep 4s
Type `python3 mcp-call.py load_skill '{"name": "db-migrations"}' 5`
Enter
Sleep 4s
Ctrl+L
Sleep 1s
