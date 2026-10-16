---
paths: ["**/.claude/skills/**/SKILL.md", "**/.claude/commands/*.md", "**/.claude/agents/*.md"]
---

# Artifact standards

- A skill directory name MUST be kebab-case and equal the `name` field in its frontmatter.
- Every skill, command and agent MUST have a `description` of at most 200 characters.
- A SKILL.md body MUST NOT exceed 120 lines.
- Agent files MUST declare a `tools:` list; an agent without it inherits every tool and is forbidden.
