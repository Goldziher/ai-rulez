---
name: vendor-health
description: Check the Vendor API health endpoint.
---

# Vendor health

```bash
curl -s -o /dev/null -w '%{http_code}' https://api.vendor.example/health
```
