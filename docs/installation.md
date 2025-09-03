# Installation

Install `ai-rulez` using your preferred package manager.

## Package Managers

=== "Homebrew (macOS/Linux)"
    ```bash
    brew install goldziher/tap/ai-rulez
    ```

=== "Go"
    ```bash
    go install github.com/Goldziher/ai-rulez/cmd@latest
    ```

=== "npm"
    ```bash
    npm install -g ai-rulez
    ```

=== "pip"
    ```bash
    pip install ai-rulez
    ```

## Run Without Installing

You can also run `ai-rulez` directly without a permanent installation.

=== "Go"
    ```bash
    go run github.com/Goldziher/ai-rulez/cmd@latest --help
    ```

=== "Python"
    ```bash
    uvx ai-rulez --help
    ```

=== "Node.js"
    ```bash
    npx ai-rulez@latest --help
    ```

## Shell Completion (Recommended)

Enable tab completion for your shell to see all available commands and flags interactively.

!!! tip "Highly Recommended"
    Setting up shell completion is a one-time step that makes the CLI much faster and easier to use. You'll be able to discover all commands just by pressing the `<Tab>` key.

=== "Bash"
    ```bash
    # Add to ~/.bashrc or ~/.bash_profile
    source <(ai-rulez completion bash)
    ```

=== "Zsh"
    ```bash
    # Add to ~/.zshrc
    source <(ai-rulez completion zsh)
    ```

=== "Fish"
    ```bash
    # Add to ~/.config/fish/config.fish
    ai-rulez completion fish | source
    ```

=== "PowerShell"
    ```powershell
    # Add to your PowerShell profile
    ai-rulez completion powershell | Out-String | Invoke-Expression
    ```

## Verify Installation

Check that the installation was successful by running:

```bash
ai-rulez --version
```

---

## Next Steps

- **[Quick Start Guide](quick-start.md)**: Get up and running in minutes.
