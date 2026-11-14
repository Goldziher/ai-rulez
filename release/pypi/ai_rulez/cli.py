import subprocess
import sys
from pathlib import Path

from .downloader import ensure_binary


def find_bundled_binary(package_dir=None, system=None):
    """Return the binary shipped inside a platform wheel, or None."""
    package_dir = Path(package_dir) if package_dir else Path(__file__).parent
    system = system or sys.platform
    name = "ai-rulez.exe" if system == "win32" else "ai-rulez"
    candidate = package_dir / "bin" / name
    return str(candidate) if candidate.is_file() else None


def exit_code(returncode):
    if returncode < 0:
        return 128 - returncode
    return returncode


def main():
    binary_path = find_bundled_binary() or ensure_binary()

    try:
        result = subprocess.run([binary_path] + sys.argv[1:], check=False)
        sys.exit(exit_code(result.returncode))
    except FileNotFoundError:
        print(f"Binary not found at {binary_path}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Failed to run ai-rulez: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
