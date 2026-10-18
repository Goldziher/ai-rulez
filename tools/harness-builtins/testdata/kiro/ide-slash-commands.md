<!-- Excerpt of https://kiro.dev/docs/ide/chat/slash-commands.md, fetched 2026-10-18 -->

## Available commands

| Icon | Type |
|------|------|
| `#` | Steering files |

### Steering files

[Steering files](https://kiro.dev/docs/steering.md) (shown with the `#` icon) provide contextual instructions to the agent. Kiro ships with several predefined steering commands, and you can create your own:

**Predefined by Kiro:**

| Command | Description |
|---------|-------------|
| `/architecture-selection` | Performs a systematic architecture analysis on the current feature's requirements and proceeds to design |
| `/quick-spec` | Generates requirements, design, and tasks in a single pass. See [Quick Spec](https://kiro.dev/docs/specs/quick-spec.md) |
| `/bug-fix` | Starts a guided bug-fix workflow for diagnosing and resolving issues |

These three commands are available only in Spec sessions. In Vibe sessions, they don't appear in the `/` menu, and typing one doesn't load its instructions.

```text
/bug-fix The login page throws a 403 after upgrading the auth library
```

**User-created steering files** configured with `inclusion: manual` or `inclusion: auto` appear as slash commands. When selected, the file's contents are added to your current conversation context. Auto-inclusion files also activate automatically when your request matches their `description` field.

See [Steering](https://kiro.dev/docs/steering.md) for configuration options.

### Declarative workflows

In Kiro IDE 1.3 and later, use `/workflow-run` to start a saved recipe directly:
