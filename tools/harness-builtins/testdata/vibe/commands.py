# Excerpt of https://github.com/mistralai/mistral-vibe/blob/v2.26.1/vibe/cli/commands.py (the _build_commands method)
class CommandRegistry:
    def _build_commands(self) -> dict[str, Command]:
        return {
            "help": Command(
                aliases=frozenset(["/help"]),
                description="Show help message",
                handler="_show_help",
                side_channel=True,
            ),
            "config": Command(
                aliases=frozenset(["/config"]),
                description="Edit config settings",
                handler="_show_config",
            ),
            "model": Command(
                aliases=frozenset(["/model"]),
                description="Select active model",
                handler="_show_model",
            ),
            "skills": Command(
                aliases=frozenset(["/skills"]),
                description="Browse, import, and manage skills",
                handler="_show_skills",
                is_available=lambda ctx: ctx.registry_skills_enabled,
            ),
            "thinking": Command(
                aliases=frozenset(["/thinking"]),
                description="Select thinking level",
                handler="_show_thinking",
            ),
            "reload": Command(
                aliases=frozenset(["/reload"]),
                description="Reload configuration, agent instructions, and skills from disk",
                handler="_reload_config",
            ),
            "clear": Command(
                aliases=frozenset(["/clear", "/new"]),
                description=(
                    "Start a new conversation. Optionally pass a prompt to seed it."
                ),
                handler="_clear_history",
            ),
            "copy": Command(
                aliases=frozenset(["/copy"]),
                description="Copy the last agent message to the clipboard",
                handler="_copy_last_agent_message",
                side_channel=True,
            ),
            "paste-image": Command(
                aliases=frozenset(["/paste-image"]),
                description="Paste an image from the OS clipboard into the prompt",
                handler="_paste_clipboard_image_command",
                side_channel=True,
                is_available=lambda _ctx: (
                    platform.system() == CLIPBOARD_IMAGE_PASTE_SUPPORTED_SYSTEM
                ),
            ),
            "log": Command(
                aliases=frozenset(["/log"]),
                description="Show path to current interaction log file",
                handler="_show_log_path",
                side_channel=True,
            ),
            "log-level": Command(
                aliases=frozenset(["/log-level"]),
                description=(
                    "Change the log level for this session or persist it to config.toml."
                ),
                handler="_log_level_command",
            ),
            "debug": Command(
                aliases=frozenset(["/debug"]),
                description="Toggle debug console",
                handler="action_toggle_debug_console",
                side_channel=True,
            ),
            "compact": Command(
                aliases=frozenset(["/compact"]),
                description="Compact conversation history by summarizing. Optionally pass instructions to guide the summary",
                handler="_compact_history",
            ),
            "exit": Command(
                aliases=frozenset(["/exit", "exit", "quit", ":q", ":quit"]),
                description="Exit the application",
                handler="_exit_app",
                exits=True,
                side_channel=True,
            ),
            "status": Command(
                aliases=frozenset(["/status"]),
                description="Display agent statistics and model and provider details",
                handler="_show_status",
                side_channel=True,
            ),
            "whoami": Command(
                aliases=frozenset(["/whoami"]),
                description="Display the Mistral signed-in user, workspace, and plan",
                handler="_show_whoami",
                side_channel=True,
            ),
            "teleport": Command(
                aliases=frozenset(["/teleport"]),
                description="Teleport session to Vibe Code Web",
                handler="_teleport_command",
            ),
            "remote-project": Command(
                aliases=frozenset(["/remote-project"]),
                description="Select the Vibe Code Web project for this repository",
                handler="_vibe_code_project_command",
            ),
            "proxy-setup": Command(
                aliases=frozenset(["/proxy-setup"]),
                description="Configure proxy and SSL certificate settings",
                handler="_show_proxy_setup",
            ),
            "resume": Command(
                aliases=frozenset(["/resume", "/continue"]),
                description="Browse, resume, or delete saved sessions",
                handler="_show_session_picker",
            ),
            "rename": Command(
                aliases=frozenset(["/rename"]),
                description="Rename the current session",
                handler="_rename_session",
                side_channel=True,
            ),
            "mcp": Command(
                aliases=frozenset(["/mcp", "/connectors"]),
                description=(
                    "Display available MCP servers and connectors. "
                    "Pass a name to list tools; subcommands: add <url> "
                    "[--transport http|streamable-http], status, login <alias>, "
                    "logout <alias>"
                ),
                handler="_show_mcp",
            ),
            "plugins": Command(
                aliases=frozenset(["/plugins"]),
                description="Display the plugins this session is running",
                handler="_show_plugins",
                is_available=lambda ctx: ctx.experimental_harness,
            ),
            "reload-plugins": Command(
                aliases=frozenset(["/reload-plugins"]),
                description="Re-pin this session's plugins and report what changed",
                handler="_reload_plugins",
                is_available=lambda ctx: ctx.experimental_harness,
            ),
            "todo": Command(
                aliases=frozenset(["/todo"]),
                description="Show the current todo list",
                handler="_show_todos",
                is_available=lambda ctx: ctx.experimental_harness,
            ),
            "voice": Command(
                aliases=frozenset(["/voice"]),
                description="Configure voice settings",
                handler="_show_voice_settings",
            ),
            "leanstall": Command(
                aliases=frozenset(["/leanstall"]),
                description="Install the Lean 4 agent (leanstral)",
                handler="_install_lean",
            ),
            "unleanstall": Command(
                aliases=frozenset(["/unleanstall"]),
                description="Uninstall the Lean 4 agent",
                handler="_uninstall_lean",
            ),
            "rewind": Command(
                aliases=frozenset(["/rewind"]),
                description="Rewind to a previous message (or press Esc twice)",
                handler="_start_rewind_mode",
            ),
            "branch": Command(
                aliases=frozenset(["/branch"]),
                description=(
                    "Fork the current conversation into a new resumable session, "
                    "leaving this session unchanged. Resume the copy with "
                    "`vibe --resume <id>`."
                ),
                handler="_branch_session",
            ),
            "retry": Command(
                aliases=frozenset(["/retry"]),
                description=(
                    "Continue an interrupted model response; optionally pass "
                    "additional instructions"
                ),
                handler="_retry",
            ),
            "loop": Command(
                aliases=frozenset(["/loop"]),
                description=(
                    "Schedule a recurring prompt in natural language: `/loop [schedule] [prompt]`"
                    if self._context.experimental_harness
                    else "Schedule a recurring prompt. "
                    "Use `/loop <interval> <prompt>`, `/loop list`, or `/loop cancel <id|all>`"
                ),
                handler="_loop_command",
                forwards_to_model=self._context.experimental_harness,
            ),
            "data-retention": Command(
                aliases=frozenset(["/data-retention"]),
                description="Show data retention information",
                handler="_show_data_retention",
                side_channel=True,
            ),
            "theme": Command(
                aliases=frozenset(["/theme"]),
                description="Select theme",
                handler="_show_theme",
            ),
        }
