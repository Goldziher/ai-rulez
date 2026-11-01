---
status: accepted
date: 2026-11-01
decision-makers: ["Project maintainers"]
---

# Make the OpenCode adapter SDK dependency an explicit default-on option

## Context and Problem Statement

[Issue 534](https://github.com/Goldziher/ai-rulez/issues/534) identifies an automatic production dependency on
`@opencode/plugin` in generated npm packages whose adapters do not require the SDK. An unnecessary dependency
also imports its transitive dependency graph into published consumers. Other adapters use the SDK and must
retain it. Package generation must preserve authored dependencies while supporting regeneration of existing
packages, including a package shared with Pi through `package_json = "merge"`.

## Decision Drivers

* Preserve the dependency and configured version range for existing configurations.
* Let an adapter author declare SDK independence without guessing from JavaScript text.
* Remove only dependencies proven to remain generator-owned and unchanged.
* Verify the packed artifact in the actual host, including a control that proves registration was examined.

## Considered Options

* Add an explicit, default-on SDK dependency option with provenance-bound cleanup.
* Remove the automatic SDK dependency from every adapter.
* Infer SDK requirements by inspecting generated or authored JavaScript imports.
* Retain the unconditional dependency and rely on consumer-specific dependency overrides.

## Decision Outcome

Chosen option: "Add an explicit, default-on SDK dependency option with provenance-bound cleanup", because it
preserves existing consumers and required SDK use while permitting a smaller dependency graph for an adapter
whose author establishes that it is SDK-independent.

`plugin.opencode_dependency_enabled` is an optional boolean. The Go configuration retains it as `*bool`:
omitted (`nil`) and explicit `true` keep the existing automatic `@opencode/plugin` dependency and any authored
`opencode_dependency` range. Explicit `false` suppresses only automatic insertion. It does not rewrite adapter
code or remove an explicit dependency declared by the package author.

When regenerating an existing merged Pi/OpenCode package, remove a previous automatic SDK entry only if its
previous provenance sidecar owns `dependencies` and the entire managed package metadata still matches the
recorded content hash. Preserve every unrelated dependency. Missing ownership, edited metadata or an edited
package version prevents that cleanup. Once automatic insertion is disabled, relinquish generator ownership
of `dependencies`, so a later false-to-false regeneration preserves an authored SDK opt-in.

The migration order is therefore to disable the dependency and regenerate before changing a managed package
version. Keep automatic insertion enabled, or explicitly provide the SDK, when an adapter requires it. The
option is separate from `opencode_dependency`, which continues to select the SDK version range.

### Consequences

* Good, because omitted and true values preserve default behavior and existing custom ranges.
* Good, because a verified SDK-independent published package can avoid an unused dependency graph.
* Good, because conservative ownership checks preserve authored dependencies and edited packages.
* Bad, because authors must determine their adapter's SDK requirements and verify its actual host behavior.
* Bad, because edited or unowned existing manifests can retain a former automatic SDK entry and need review.

### Confirmation

The implementation associated with issue 534 is covered by `internal/config/plugin_opencode_dependency_test.go`,
`internal/generator/plugin/opencode_dependency_test.go` and
`internal/generator/plugin/opencode_dependency_lifecycle_test.go`. They cover omitted/true/false settings,
custom ranges, fresh manifests, owned default-to-false cleanup, authored Pi SDK dependencies, false-to-false
persistence, unrelated dependencies and conservative preservation after metadata/version edits. Boolean schema
acceptance and invalid-type rejection are covered by `schema/opencode_dependency_test.go`.

`mise run test:opencode-host` uses the isolated harness in `scripts/mise/test_opencode_host.py` and its isolation
controls. Qualification of the 5.10.0 candidate against official OpenCode v2.2.5 packed an SDK-free fixture and
observed one active plugin registering one command, one skill and one agent instruction. Deleting the fixture
command made the command census zero while the plugin remained active and its skill and agent instruction
remained registered. Default and explicit SDK-required package manifests retained `@opencode/plugin`.
No provider or model request was needed. This qualifies that SDK-free fixture; authors still need to qualify
their own adapter and published npm package.

[Plugin authoring](../docs/plugins.md) documents the switch, range behavior, ownership boundary and migration
order.
