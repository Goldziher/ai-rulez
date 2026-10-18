// Excerpt of https://github.com/block/goose/blob/v1.55.0/crates/goose-cli/src/session/input.rs
fn handle_slash_command(input: &str) -> Option<InputResult> {
    let input = input.trim();

    // Command prefix constants
    const CMD_PROMPTS: &str = "/prompts ";
    const CMD_PROMPT: &str = "/prompt";
    const CMD_PROMPT_WITH_SPACE: &str = "/prompt ";
    const CMD_EXTENSION: &str = "/extension ";
    const CMD_BUILTIN: &str = "/builtin ";
    const CMD_MODE: &str = "/mode ";
    const CMD_MODEL: &str = "/model";
    const CMD_MODEL_WITH_SPACE: &str = "/model ";
    const CMD_CLEAR: &str = "/clear";
    const CMD_NEW: &str = "/new";
    const CMD_COMPACT: &str = "/compact";
    const CMD_SUMMARIZE_DEPRECATED: &str = "/summarize";
    const CMD_EDIT: &str = "/edit";
    const CMD_EDIT_WITH_SPACE: &str = "/edit ";
    const CMD_SKILLS: &str = "/skills";

    match input {
        "/exit" | "/quit" => Some(InputResult::Exit),
        "/?" | "/help" => {
            print_help();
            print_editor_help();
            Some(InputResult::Retry)
        }
        "/t" => Some(InputResult::ToggleTheme),
        s if s.starts_with("/t ") => {
            let t = s
                .strip_prefix("/t ")
                .unwrap_or_default()
                .trim()
                .to_lowercase();
            if ["light", "dark", "ansi"].contains(&t.as_str()) {
                Some(InputResult::SelectTheme(t))
            } else {
                println!(
                    "Theme Unavailable: {} Available themes are: light, dark, ansi",
                    t
                );
                Some(InputResult::Retry)
            }
        }
        "/prompts" => Some(InputResult::ListPrompts(None)),
        s if s.starts_with(CMD_PROMPTS) => {
            // Parse arguments for /prompts command
            let args = s.strip_prefix(CMD_PROMPTS).unwrap_or_default();
            parse_prompts_command(args)
        }
        s if s.starts_with(CMD_PROMPT) => {
            if s == CMD_PROMPT {
                // No arguments case
                Some(InputResult::PromptCommand(PromptCommandOptions {
                    name: String::new(), // Empty name will trigger the error message in the rendering
                    info: false,
                    arguments: HashMap::new(),
                }))
            } else if let Some(stripped) = s.strip_prefix(CMD_PROMPT_WITH_SPACE) {
                // Has arguments case
                parse_prompt_command(stripped)
            } else {
                // Handle invalid cases like "/promptxyz"
                None
            }
        }
        s if s.starts_with(CMD_EXTENSION) => Some(InputResult::AddExtension(
            s.get(CMD_EXTENSION.len()..).unwrap_or("").to_string(),
        )),
        s if s.starts_with(CMD_BUILTIN) => Some(InputResult::AddBuiltin(
            s.get(CMD_BUILTIN.len()..).unwrap_or("").to_string(),
        )),
        s if s.starts_with(CMD_MODE) => Some(InputResult::GooseMode(
            s.get(CMD_MODE.len()..).unwrap_or("").to_string(),
        )),
        s if s == CMD_MODEL => Some(InputResult::Model(ModelCommandOptions::default())),
        s if s.starts_with(CMD_MODEL_WITH_SPACE) => {
            let rest = s
                .get(CMD_MODEL_WITH_SPACE.len()..)
                .unwrap_or("")
                .trim()
                .to_string();
            if rest.is_empty() {
                Some(InputResult::Model(ModelCommandOptions::default()))
            } else if let Some(after_flag) = rest.strip_prefix("--provider ") {
                let parts: Vec<&str> = after_flag.split_whitespace().collect();
                let provider = parts.first().map(|s| s.to_string());
                let model = parts
                    .get(1..)
                    .filter(|parts| !parts.is_empty())
                    .map(|parts| parts.join(" "));
                Some(InputResult::Model(ModelCommandOptions { provider, model }))
            } else if rest == "--provider" {
                Some(InputResult::Model(ModelCommandOptions {
                    provider: Some(String::new()),
                    model: None,
                }))
            } else {
                Some(InputResult::Model(ModelCommandOptions {
                    provider: None,
                    model: Some(rest),
                }))
            }
        }
        s if s == CMD_CLEAR => Some(InputResult::Clear),
        s if s == CMD_NEW => Some(InputResult::New),
        s if s == CMD_COMPACT => Some(InputResult::Compact),
        // Match "/skills" exactly or "/skills " with args - avoids matching e.g. "/skillsextra"
        s if s == CMD_SKILLS || s.starts_with(&format!("{CMD_SKILLS} ")) => {
            let args = s.get(CMD_SKILLS.len()..).unwrap_or("").trim();
            if args.is_empty() {
                Some(InputResult::ListSkills)
            } else {
                let names: Vec<String> = args.split_whitespace().map(String::from).collect();
                Some(InputResult::LoadSkills(names))
            }
        }
        s if s == CMD_SUMMARIZE_DEPRECATED => {
            println!("{}", console::style("⚠️  Note: /summarize has been renamed to /compact and will be removed in a future release.").yellow());
            Some(InputResult::Compact)
        }
        "/r" => Some(InputResult::ToggleFullToolOutput),
        s if s == CMD_EDIT => Some(InputResult::Edit(None)),
        s if s.starts_with(CMD_EDIT_WITH_SPACE) => {
            let prefill = s
                .strip_prefix(CMD_EDIT_WITH_SPACE)
                .unwrap_or_default()
                .trim();
            if prefill.is_empty() {
                Some(InputResult::Edit(None))
            } else {
                Some(InputResult::Edit(Some(prefill.to_string())))
            }
        }
        _ => None,
    }
}
