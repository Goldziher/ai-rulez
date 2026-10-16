import subprocess, sys

subprocess.run(["git", "diff", "--stat", sys.argv[1]], check=True)
