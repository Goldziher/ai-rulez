# Build and Distribution

This directory contains the distribution packages for different platforms.

## Structure

- `npm/` - NPM package wrapper
- `python/` - Python/PyPI package wrapper  
- `dist/` - Built binaries (gitignored)

## How It Works

### NPM Package (`airules`)

The NPM package is a thin wrapper that:
1. Downloads the appropriate binary for the user's platform during `postinstall`
2. Provides a Node.js shim that executes the binary

### Python Package (`airules`)

The Python package:
1. Downloads the appropriate binary during `pip install`
2. Provides a Python wrapper script that executes the binary

## Release Process

When a new version is tagged (e.g., `v0.1.0`):

1. GoReleaser builds binaries for all platforms
2. GitHub Actions publishes to:
   - GitHub Releases (binaries)
   - NPM Registry (wrapper)
   - PyPI (wrapper)

## Local Testing

### Test NPM package locally:
```bash
cd build/npm
npm link
airules --help
```

### Test Python package locally:
```bash
cd build/python
pip install -e .
airules --help
```