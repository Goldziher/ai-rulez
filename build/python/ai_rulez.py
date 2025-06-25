"""Entry point for ai-rulez CLI tool."""

import os
import sys
import subprocess
from pathlib import Path


def find_binary():
    """Find the ai-rulez binary."""
    
    if os.environ.get('AI_RULEZ_DEV'):
        binary_name = 'ai-rulez.exe' if sys.platform == 'win32' else 'ai-rulez'
        dev_binary = Path(__file__).parent.parent.parent / binary_name
        if dev_binary.exists():
            return str(dev_binary)
    
    
    binary_name = 'ai-rulez.exe' if sys.platform == 'win32' else 'ai-rulez'
    
    
    if hasattr(sys, 'real_prefix') or (hasattr(sys, 'base_prefix') and sys.base_prefix != sys.prefix):
        
        scripts_dir = Path(sys.prefix) / ('Scripts' if sys.platform == 'win32' else 'bin')
    else:
        
        scripts_dir = Path(sys.executable).parent
    
    binary_path = scripts_dir / binary_name
    if binary_path.exists():
        return str(binary_path)
    
    
    from shutil import which
    binary_in_path = which(binary_name)
    if binary_in_path:
        return binary_in_path
    
    raise RuntimeError(f"Could not find {binary_name} binary. Please reinstall the package.")


def main():
    """Run the ai-rulez binary with command line arguments."""
    try:
        binary = find_binary()
        
        result = subprocess.run([binary] + sys.argv[1:], capture_output=False)
        sys.exit(result.returncode)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()