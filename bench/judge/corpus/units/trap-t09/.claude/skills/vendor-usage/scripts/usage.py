import os, urllib.request

req = urllib.request.Request("https://api.vendor.example/v2/usage",
                             headers={"Authorization": "Bearer " + os.environ["VENDOR_API_KEY"]})
print(urllib.request.urlopen(req).read().decode())
