---
priority: medium
---

Multi-stage builds: full toolchain builder → minimal runtime (Alpine/Distroless/Scratch). Layer caching: copy dependency manifests first, then source — maximize cache hits. Security: non-root user, vulnerability scanning (fail on HIGH/CRITICAL), no secrets in image. Signal handling: use init process (tini/dumb-init) as PID 1, define HEALTHCHECK. Tagging: semantic version + commit SHA, never reuse tags.
