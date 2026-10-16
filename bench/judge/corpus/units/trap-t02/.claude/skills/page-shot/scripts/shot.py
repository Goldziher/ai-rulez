import shutil, subprocess, sys

browser = next(b for b in ("chromium", "google-chrome") if shutil.which(b))
subprocess.run([browser, "--headless", "--screenshot", sys.argv[1]], check=True)
