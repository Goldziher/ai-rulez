import subprocess
import sys
from .downloader import ensure_binary


def main():
    binary_path = ensure_binary()

    try:
        result = subprocess.run([binary_path] + sys.argv[1:], check=False)
        sys.exit(result.returncode)
    except FileNotFoundError:
        print(f"Binary not found at {binary_path}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Failed to run ai-rulez: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
