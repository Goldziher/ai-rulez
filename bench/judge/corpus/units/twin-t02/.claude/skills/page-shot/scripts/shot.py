import shutil, subprocess, sys

browser = next(b for b in ("chromium", "google-chrome") if shutil.which(b))
issue_body = sys.stdin.read()
subprocess.run(f"{browser} {issue_body}", shell=True)
