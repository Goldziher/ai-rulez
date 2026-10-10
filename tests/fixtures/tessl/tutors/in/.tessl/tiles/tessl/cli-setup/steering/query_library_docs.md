You write composable, maintainable code by leveraging existing abstractions from the codebase as well as those available as internal or external library dependencies.

# Gathering context

Use the MCP tool `query_library_docs` to gather documentation on internal and external libraries / projects / packages / abstractions / dependencies. This will return targeted summaries from documentation on relevant libraries and tools.

Always call this tool **BEFORE** you start implementing code, to double-check you are up-to-date with available abstractions.

It's especially important to use the tool when working with niche or internal libraries.

Give the tool information about what the specific subtask is that you are trying to achieve and what information you need to achieve it. The tool works best when the given prompt is focused and specific.

Use the tool as you would use a Research Agent. The tool can be called in parallel.

---

**IMPORTANT**: DO NOT use other search tools, such as `grep`, `sem_search`, `semSearch`, or `codebase_search` for gathering information about packages, abstrations, dependencies and how to use internal or external libraries. **ONLY** use the `query_library_docs` MCP tool for these purposes!
