---
name: container-standards
description: "Container build and runtime: multi-stage builds, minimal base image, cache-friendly layers, non-root user, image scanning, no baked secrets, init process, healthcheck, tagging. Use when writing a Dockerfile or image pipeline."
---

- **Multi-stage builds**: full toolchain builder → minimal runtime (Alpine/Distroless/Scratch).
- **Layer caching**: copy dependency manifests first, then source — maximize cache hits.
- **Security**: non-root user, vulnerability scanning (fail on HIGH/CRITICAL), no secrets in the image.
- **Signal handling**: use an init process (tini/dumb-init) as PID 1, and define a `HEALTHCHECK`.
- **Tagging**: semantic version + commit SHA, never reuse a tag.
