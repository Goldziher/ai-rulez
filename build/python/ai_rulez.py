#!/usr/bin/env python3
"""Entry point for ai-rulez CLI tool."""

import os
import sys
import subprocess
from pathlib import Path


def find_binary():
    """Find the ai-rulez binary."""
    # Check if running in development
    if os.environ.get('AI_RULEZ_DEV'):
        binary_name = 'ai-rulez.exe' if sys.platform == 'win32' else 'ai-rulez'
        dev_binary = Path(__file__).parent.parent.parent / binary_name
        if dev_binary.exists():
            return str(dev_binary)
    
    # Check standard installation locations
    binary_name = 'ai-rulez.exe' if sys.platform == 'win32' else 'ai-rulez'
    
    # Check in Scripts directory (where pip installs executables)
    if hasattr(sys, 'real_prefix') or (hasattr(sys, 'base_prefix') and sys.base_prefix != sys.prefix):
        # In virtual environment
        scripts_dir = Path(sys.prefix) / ('Scripts' if sys.platform == 'win32' else 'bin')
    else:
        # System installation
        scripts_dir = Path(sys.executable).parent
    
    binary_path = scripts_dir / binary_name
    if binary_path.exists():
        return str(binary_path)
    
    # Last resort: check PATH
    from shutil import which
    binary_in_path = which(binary_name)
    if binary_in_path:
        return binary_in_path
    
    raise RuntimeError(f"Could not find {binary_name} binary. Please reinstall the package.")


def main():
    """Run the ai-rulez binary with command line arguments."""
    try:
        binary = find_binary()
        # Pass through all command line arguments
        result = subprocess.run([binary] + sys.argv[1:], capture_output=False)
        sys.exit(result.returncode)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()