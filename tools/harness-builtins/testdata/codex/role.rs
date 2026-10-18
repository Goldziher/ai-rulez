// excerpt of codex-rs/core/src/agent/role.rs at rust-v0.163.0, bodies shortened
pub const DEFAULT_ROLE_NAME: &str = "default";

mod built_in {
    use super::*;

    pub(super) fn configs() -> &'static BTreeMap<String, AgentRoleConfig> {
        static CONFIG: LazyLock<BTreeMap<String, AgentRoleConfig>> = LazyLock::new(|| {
            BTreeMap::from([
                (
                    DEFAULT_ROLE_NAME.to_string(),
                    AgentRoleConfig {
                        description: Some("Default agent.".to_string()),
                        config_file: None,
                    },
                ),
                (
                    "explorer".to_string(),
                    AgentRoleConfig {
                        description: Some(
                            r#"Use `explorer` for specific codebase questions."#.to_string(),
                        ),
                        config_file: Some("explorer.toml".to_string().parse().unwrap_or_default()),
                    },
                ),
                (
                    "worker".to_string(),
                    AgentRoleConfig {
                        description: Some(r#"Use for execution and production work."#.to_string()),
                        config_file: None,
                    },
                ),
                // Awaiter is temp removed
                //                 (
                //                     "awaiter".to_string(),
                //                     AgentRoleConfig {
                //                         description: None,
                //                     }
                //                 ),
            ])
        });
        &CONFIG
    }
}
