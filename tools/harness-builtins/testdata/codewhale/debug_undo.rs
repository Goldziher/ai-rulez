// Excerpt of https://github.com/codewhale-hq/Codewhale/blob/v0.10.1/crates/tui/src/commands/groups/debug/undo.rs
use codewhale_command_contract::metadata::{CommandInfo, RegisterCommand};

macro_rules! registration {
    ($ty:ident, $name:literal, $aliases:expr, $key:literal, $caps:expr, $handler:ident) => {
        pub(in crate::commands) struct $ty;
        impl RegisterCommand<CommandResult> for $ty {
            fn info() -> &'static CommandInfo {
                &CommandInfo {
                    name: $name,
                    aliases: $aliases,
                    usage: concat!("/", $name),
                    description_key: $key,
                }
            }
            fn handler() -> CommandHandler<CommandResult> {
                CommandHandler::Contextual {
                    capabilities: $caps,
                    handler: $handler,
                }
            }
        }
    };
}
registration!(
    EditCmd,
    "edit",
    &[],
    "cmd_edit_description",
    Caps::DEBUG_HISTORY,
    edit
);
registration!(
    DiffCmd,
    "diff",
    &[],
    "cmd_diff_description",
    Caps::DEBUG_DIFF,
    diff
);
registration!(
    UndoCmd,
    "undo",
    &[],
    "cmd_undo_description",
    Caps::DEBUG_UNDO.union(Caps::DEBUG_HISTORY),
    undo
);
registration!(
    RetryCmd,
    "retry",
    &["chongshi"],
    "cmd_retry_description",
    Caps::DEBUG_HISTORY,
    retry
);

pub(in crate::commands) const SNAPSHOT_REPO_UNAVAILABLE_PREFIX: &str = "Snapshot repo unavailable";
pub(in crate::commands) const FILES_NOT_REVERTED_NOTE: &str =
    "Workspace files were NOT reverted — only the conversation was rolled back.";

pub(super) fn edit(contexts: CommandContexts<'_>, _: Option<&str>) -> CommandResult {
    let mut parts = contexts.into_parts();
