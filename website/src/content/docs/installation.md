---
title: "Installation"
description: "ai-rulez is a single binary, distributed through Homebrew, npm, PyPI and Go."
---

`ai-rulez` is a single binary, distributed through Homebrew, npm, PyPI and Go. Install it with the package manager you
already use, or run it without installing.

```bash
brew install goldziher/tap/ai-rulez
ai-rulez version
```

```text
ai-rulez version 5.11.1
```

## Package managers

**Homebrew (macOS, Linux)**

```bash
brew install goldziher/tap/ai-rulez
```

**npm**

```bash
npm install -g ai-rulez
# or
pnpm add -g ai-rulez
```

Requires Node.js 22 or newer. The package downloads the binary for your platform on first run and checks it
against the release checksums.

**pip / pipx**

```bash
pipx install ai-rulez
# or
pip install ai-rulez
```

Requires Python 3.11 or newer.

**uv**

```bash
uv tool install ai-rulez
```

Release binaries are built for Linux and macOS (amd64 and arm64) and Windows (amd64). Linux binaries need glibc 2.36 or
newer; Alpine (musl) is not supported.

## Run without installing

```bash
npx ai-rulez@latest generate
pnpm dlx ai-rulez@latest generate
uvx ai-rulez generate
```

Pin a version (`npx ai-rulez@5.11.1`, `uvx --from ai-rulez==5.11.1 ai-rulez`) when a script or hook must not change
behavior between releases.

## Go

```bash
go install github.com/Goldziher/ai-rulez/v5/cmd/ai-rulez@latest
```

The module path must include `/v5`; without it, `@latest` resolves to an old 1.x release.

`go install` builds with cgo and links the native library shipped in the Go module dynamically, so it needs a C
toolchain to build and the Go module cache to stay in place at run time. For a binary you distribute, use a release
package, or build a statically linked binary from a clone with [mise](https://mise.jdx.dev):

```bash
git clone https://github.com/Goldziher/ai-rulez
cd ai-rulez
mise run build
./bin/ai-rulez version
```

To embed ai-rulez as a library instead, see [Embedding (Go API)](/ai-rulez/embedding/).

## Shell completion

**Bash**

Add to `~/.bashrc`:

```bash
source <(ai-rulez completion bash)
```

**Zsh**

Add to `~/.zshrc`:

```bash
source <(ai-rulez completion zsh)
```

**Fish**

Add to `~/.config/fish/config.fish`:

```bash
ai-rulez completion fish | source
```

**PowerShell**

Add to your PowerShell profile:

```powershell
ai-rulez completion powershell | Out-String | Invoke-Expression
```

## Git hooks

Run `generate` and `validate` before each commit so the generated files never drift from `.ai-rulez/`.

When you create a project, `ai-rulez init --setup-hooks` detects an existing pre-commit, Lefthook or Husky
configuration and adds ai-rulez to it; with none present it does nothing. `--setup-hooks` is an `init` flag, so in an
existing project add the hooks by hand:

**pre-commit / prek**

Add to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/Goldziher/ai-rulez
    rev: v5.11.1
    hooks:
      - id: ai-rulez-recursive
      - id: ai-rulez-validate
```

The hooks run only when a file under a `.ai-rulez/` directory changes. They use the `ai-rulez` on `PATH`, or
download and cache a checksum-verified release binary when there is none.

| Hook id | Runs |
| ------- | ---- |
| `ai-rulez-generate` | `generate` for the root `.ai-rulez/` |
| `ai-rulez-recursive` | `generate --recursive` for every nested `.ai-rulez/` |
| `ai-rulez-validate` | `validate --recursive` |
| `ai-rulez-plugin-generate` | `generate --recursive --plugin --if-configured` |
| `ai-rulez-plugin-verify` | `verify --recursive --plugin --if-configured` |

Add the two plugin hooks if the repository publishes plugin bundles; `--if-configured` makes them do nothing in a
repository without a `[plugin]` or `[marketplace]` block.

**Lefthook**

Add to `lefthook.yml`:

```yaml
pre-commit:
  commands:
    ai-rulez:
      glob: ".ai-rulez/**"
      run: ai-rulez generate --recursive --no-local
    ai-rulez-validate:
      glob: ".ai-rulez/**"
      run: ai-rulez validate --recursive --no-local
```

**Poly**

See [Poly hooks](/ai-rulez/poly-hooks/).

The shipped pre-commit hooks do not pass `--no-local`, so they read a developer's gitignored `config.local.toml` and
`.ai-rulez/local/` content like any local run. To check only the shared configuration, pass `--no-local` as the
Lefthook example does; with pre-commit, override the hook's arguments (`args: [validate, --recursive, --no-local]`). See
[Machine-local configuration in hooks](/ai-rulez/poly-hooks/#machine-local-configuration-in-hooks).

## CI

The `Goldziher/ai-rulez` GitHub Action installs a checksum-verified, cached binary with no Go, Node or Python setup:

```yaml
steps:
  - uses: actions/checkout@v7
  - uses: Goldziher/ai-rulez@v5
    with:
      version: 5.11.1
  - run: ai-rulez generate --check
  - run: ai-rulez validate
```

`validate` fails on errors; add `--strict` to fail on warnings too.

See [GitHub Actions](/ai-rulez/github-actions/) for version pinning, the governance gate and scheduled updates.

## Next steps

Follow the [Quick start](/ai-rulez/quick-start/) to create a configuration and generate your first files.
