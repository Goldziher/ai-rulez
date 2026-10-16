---
name: skill-authoring
description: "Create or improve a skill: decide whether a skill is the right home, interview the user, write a brief SKILL.md, test it on real tasks and iterate. Load when asked to create a skill, turn team knowledge into one, or move a command into one."
---

- Check a skill is the right home before writing one (see `config-placement`).
- `name` equals the directory name and is unique across the project. The `description` says what the skill does and when to use it, with the key terms a user or model would match on.
- Keep the body brief and imperative, 50 to 150 lines. Move bulk to `references/*.md` and link it relatively.
- Explore first. Check that every path, command and flag you cite exists. Do not repeat what a rule already says.
- Self-review for length, repetition, contradictions and unverified claims.
- Test on two or three real tasks. After each, ask what guidance was followed, what had to be explained by hand and where instructions contradict. Apply the fixes and test again.
