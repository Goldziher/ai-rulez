// Excerpt of https://github.com/codewhale-hq/Codewhale/blob/v0.10.1/crates/tui/src/commands/groups/config/mod.rs
    name: "login",
    aliases: &[],
    usage: "/login [status|account|key]",
    description_id: MessageId::CmdLoginDescription,
};
static AUTH_INFO: CommandInfo = CommandInfo {
    name: "auth",
    aliases: &[],
    usage: "/auth xai-device|chatgpt|chatgpt-revoke|orcarouter|orcarouter-revoke",
    description_id: MessageId::CmdAuthDescription,
};
static RAIL_INFO: CommandInfo = CommandInfo {
    name: "workbar",
    // /rail and /sidebar are the names users already know; both now drive
    // the one workbar.
    aliases: &["rail", "sidebar"],
    usage: "/workbar [bottom|top|left|right|off|tasks|agents|context|pinned] [--save]",
    description_id: MessageId::CmdSidebarDescription,
};
static PET_INFO: CommandInfo = CommandInfo {
    name: "pet",
    aliases: &[],
    usage: "/pet [on|off|status|appearance|window|source|export|sound on|off|avatar [key]|action [name|live]|view [name|live]]",
    description_id: MessageId::CmdPetDescription,
};
static SETTINGS_INFO: CommandInfo = CommandInfo {
    name: "settings",
    aliases: &[],
    usage: "/settings [text]",
    description_id: MessageId::CmdSettingsDescription,
};
static STATUSLINE_INFO: CommandInfo = CommandInfo {
    name: "statusline",
    aliases: &[],
    usage: "/statusline",
    description_id: MessageId::CmdStatuslineDescription,
};
static MODE_INFO: CommandInfo = CommandInfo {
    name: "mode",
    aliases: &["jihua", "zidong"],
    usage: "/mode [act|plan|operate|1|2|3]",
    description_id: MessageId::CmdModeDescription,
};
static FULLSCREEN_INFO: CommandInfo = CommandInfo {
