// Package telemetry records and optionally exports identifier-only events about
// which skills, rules, agents, commands and context files an AI harness loaded.
//
// The design has three layers, each usable alone:
//
//   - An Event model (versioned, closed set of fields, validated on the way in).
//   - Emitters: a local JSONL log (the usage log, extended with new event kinds),
//     a bounded on-disk spool feeding an OTLP/HTTP exporter, and a fan-out.
//   - Settings resolution with a trust rule: a repository config can switch local
//     recording on but can never enable network export or choose the endpoint.
//
// Privacy is structural. Events carry identifiers (kind, id, digest, harness,
// role, a salted session hash, outcome, load reason). They never carry prompts,
// file contents, command arguments or absolute paths, and the exporter builds
// its payload from a closed allowlist (allowlist.go) rather than filtering.
//
// Hooks are short-lived processes, so the recording path never touches the
// network: it appends to a spool file and, at most, starts a detached flush.
package telemetry
