# Search repository skills

When a task references a repository path or asks for project-specific guidance, search the skills MCP server first.
Use ToolSearch to select `mcp__skills__find_skill` and `mcp__skills__load_skill` if deferred.
Call `find_skill` with the task and repository-relative path, then load the applicable match before concluding no
guidance exists. Skip repository skill discovery for unrelated questions.
