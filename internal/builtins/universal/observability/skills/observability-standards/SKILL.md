---
name: observability-standards
description: "Observability standards: JSON structured logging with key=value fields and never string interpolation, the ERROR/WARN/INFO/DEBUG/TRACE level taxonomy, a /health endpoint with per-component status wired to orchestrator probes, counter/gauge/histogram metric choice with no high-cardinality labels, and correlation-ID propagation across service boundaries. Load when adding logging, metrics, tracing or a health endpoint to a service, or reviewing what a service emits."
---

- **Structured logging**: JSON output, `key=value` pairs — never string interpolation in log messages.
- **Log levels**: ERROR (unrecoverable), WARN (degraded), INFO (state changes), DEBUG (flow), TRACE (off in prod).
- **Health endpoints**: `/health` with component status, wired to orchestrator probes.
- **Metrics**: counters for requests, gauges for connections, histograms for latency — no high-cardinality labels.
- Propagate request/correlation IDs across service boundaries.
