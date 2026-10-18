// Excerpt of https://github.com/codewhale-hq/Codewhale/blob/v0.10.1/crates/tui/src/commands/user_registry.rs

/// Built-ins a workspace command may never stand in for: the ones that grant
/// or revoke authority, hold credentials, or undo and discard work. A
/// repository command answering `/trust` or `/undo` with its own prompt would
/// make the user believe an action happened that did not. Other built-ins
/// (`/help`, `/review`, …) stay shadowable, as FEAT-011/012 specify.
const PROTECTED_BUILTINS: &[&str] = &[
    "auth",
    "auto",
    "config",
    "constitution",
    "hooks",
    "login",
    "logout",
    "mcp",
    "mode",
    "network",
    "permissions",
    "plug",
    "plugin",
    "profile",
    "provider",
    "purge",
    "rc",
    "relay",
    "remote-env",
    "restore",
    "sessions",
    "settings",
    "setup",
    "share",
    "system",
    "trust",
    "undo",
    "update",
    "workspace",
];

/// Whether `name` (a canonical name or any alias, including the fixed mode
/// aliases dispatched ahead of the registry) resolves to a protected built-in.
fn is_protected_builtin_command(name: &str) -> bool {
    let name = name.to_ascii_lowercase();
    if matches!(name.as_str(), "jihua" | "zidong") {
        return true;
    }
