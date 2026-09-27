---
name: container-standards
description: "Container build and runtime standards: multi-stage builds into a minimal runtime (Alpine/Distroless/Scratch), manifest-before-source layer ordering for cache hits, non-root user, vulnerability scan failing on HIGH/CRITICAL, no secrets in the image, tini/dumb-init as PID 1, HEALTHCHECK, and semver+SHA tags that are never reused. Load when writing or reviewing a Dockerfile, docker-compose file, container build pipeline, or image tagging scheme."
---

- **Multi-stage builds**: full toolchain builder → minimal runtime (Alpine/Distroless/Scratch).
- **Layer caching**: copy dependency manifests first, then source — maximize cache hits.
- **Security**: non-root user, vulnerability scanning (fail on HIGH/CRITICAL), no secrets in the image.
- **Signal handling**: use an init process (tini/dumb-init) as PID 1, and define a `HEALTHCHECK`.
- **Tagging**: semantic version + commit SHA, never reuse a tag.
