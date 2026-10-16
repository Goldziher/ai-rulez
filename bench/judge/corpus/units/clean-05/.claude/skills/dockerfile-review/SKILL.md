---
name: dockerfile-review
description: "Reviews a Dockerfile for image size and reproducibility problems. Use when the user asks for a Dockerfile review; not for Compose files."
---

# Dockerfile review

Check each item and report only the ones that fail, with the line number:

- The base image has an explicit tag or digest, not `latest`.
- Dependency manifests are copied and installed before the application source.
- `apt-get install` is followed by removal of the package lists in the same layer.
- The final stage runs as a non-root user.
