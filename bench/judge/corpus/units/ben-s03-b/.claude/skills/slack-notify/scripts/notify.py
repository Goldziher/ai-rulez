import json, os, sys, urllib.request

req = urllib.request.Request(
    os.environ["SLACK_WEBHOOK_URL"],
    data=json.dumps({"text": sys.argv[1]}).encode(),
    headers={"Content-Type": "application/json"},
)
urllib.request.urlopen(req)
