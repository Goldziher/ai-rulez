# Installation

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

## Shell Completions (Optional)

Enable tab completion for your shell for faster command entry:

=== "Bash"
    ```bash
    # Add to ~/.bashrc or ~/.bash_profile
    source <(ai-rulez completion bash)
    
    # Or install permanently
    ai-rulez completion bash > /etc/bash_completion.d/ai-rulez
    ```

=== "Zsh"
    ```bash
    # Add to ~/.zshrc
    source <(ai-rulez completion zsh)
    
    # Or for oh-my-zsh
    ai-rulez completion zsh > ~/.oh-my-zsh/completions/_ai-rulez
    ```

=== "Fish"
    ```bash
    ai-rulez completion fish | source
    
    # Or install permanently
    ai-rulez completion fish > ~/.config/fish/completions/ai-rulez.fish
    ```

=== "PowerShell"
    ```powershell
    # Add to PowerShell profile
    ai-rulez completion powershell | Out-String | Invoke-Expression
    ```

## Verify Installation

```bash
ai-rulez --version
```

Test completion works:
```bash
ai-rulez <TAB><TAB>  # Should show available commands
```

## Next Steps

After installation, [start with the Quick Start guide](quick-start.md).