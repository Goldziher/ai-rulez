---
priority: high
aliases: [parallel]
usage: "/parallelize"
description: "Split tasks among subagents for parallel execution"
---
# Parallelize

Split the tasks you have among subagents. Parallelize effectively while considering any dependencies between tasks.

Guidelines:
1. **Analyze dependencies**: Identify which tasks are independent and which depend on others
2. **Choose subagents**: Select from available subagents the best fit for each piece of work
3. **Ensure permissions**: Make sure each subagent has the required permissions and context for its assigned work
4. **Coordinate results**: Merge results from parallel work streams, resolving any conflicts
5. **Sequential when needed**: Tasks with dependencies must run sequentially — only parallelize truly independent work
