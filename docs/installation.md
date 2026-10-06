# Installation

Install `ai-rulez` using your preferred package manager.

## Package Managers

=== "Homebrew (macOS/Linux)"
    ```bash
    brew install goldziher/tap/ai-rulez
    ```

=== "npm"
    ```bash
    npm install -g ai-rulez
    ```

=== "pip"
    ```bash
    pip install ai-rulez
    ```

=== "uv tool"
    ```bash
    uv tool install ai-rulez
    ```

!!! note "Go"
    The module path is `github.com/Goldziher/ai-rulez/v5`, and the binary is the
    `cmd/ai-rulez` package. Install it with the `/v5` path (a path without it
    resolves to an old 1.x build):

    ```bash
    go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest
    ```

    Or build from a clone:

    ```bash
    git clone https://github.com/Goldziher/ai-rulez
    cd ai-rulez
    go build -o ai-rulez ./cmd/ai-rulez
    ```

## Run Without Installing

You can also run `ai-rulez` directly without a permanent installation.

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
    Add to `~/.bashrc` or `~/.bash_profile`:

    ```bash
    source <(ai-rulez completion bash)
    ```

=== "Zsh"
    Add to `~/.zshrc`:

    ```bash
    source <(ai-rulez completion zsh)
    ```

=== "Fish"
    Add to `~/.config/fish/config.fish`:

    ```bash
    ai-rulez completion fish | source
    ```

=== "PowerShell"
    Add to your PowerShell profile:

    ```powershell
    ai-rulez completion powershell | Out-String | Invoke-Expression
    ```

## Verify Installation

Check that the installation was successful:

```bash
ai-rulez version
```

---

## Next Steps

- **[Quick Start Guide](quick-start.md)**: Get up and running in minutes.
