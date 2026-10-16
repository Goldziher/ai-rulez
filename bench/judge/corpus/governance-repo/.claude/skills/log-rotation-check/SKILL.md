---
name: log-rotation-check
description: "Checks that log rotation is configured. Use when a service disk fills up."
---

# Log rotation check

## Steps

1. Read the service's logrotate configuration.
2. Confirm it names a size or age limit and a retention count.

## Verify

Run `logrotate --debug` against the configuration and confirm that it prints no errors.

## Not for

Application-level log levels; those are changed in the service configuration.

Contact the platform channel if this fails.
