---
priority: medium
---

Structured logging: JSON output, key=value pairs — never string interpolation in log messages. Log levels: ERROR (unrecoverable), WARN (degraded), INFO (state changes), DEBUG (flow), TRACE (off in prod). Health endpoints: /health with component status, wired to orchestrator probes. Metrics: counters for requests, gauges for connections, histograms for latency — no high-cardinality labels. Propagate request/correlation IDs across service boundaries.
