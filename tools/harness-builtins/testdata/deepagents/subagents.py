# Excerpt of https://github.com/langchain-ai/deepagents/blob/0c99fd026c4c41a08e6725a5e45d5ac111dcc6bb/libs/deepagents/deepagents/middleware/subagents.py
GENERAL_PURPOSE_SUBAGENT: SubAgent = {
    "name": "general-purpose",
    "description": DEFAULT_GENERAL_PURPOSE_DESCRIPTION,
    "system_prompt": DEFAULT_SUBAGENT_PROMPT,
}
"""Base spec for general-purpose subagent (caller adds model, tools, middleware)."""
