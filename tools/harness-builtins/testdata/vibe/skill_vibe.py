# Excerpt of https://github.com/mistralai/mistral-vibe/blob/v2.26.1/vibe/core/skills/builtins/vibe.py (long text elided)

SKILL = SkillInfo(
    name="vibe",
    description="...",
    user_invocable=False,
    prompt=_PROMPT_TEMPLATE.replace("__VIBE_VERSION__", __version__),
    source=SkillSource.BUILTIN,
)
