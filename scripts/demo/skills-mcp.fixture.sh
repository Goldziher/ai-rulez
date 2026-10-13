#!/usr/bin/env bash
# Builds the project the skills-mcp tape records in. Usage: skills-mcp.fixture.sh <dir>
# Fresh git repo, static skills, plus the MCP helper copied in. No network, no clock.
set -euo pipefail

dir="${1:?usage: skills-mcp.fixture.sh <dir>}"
here="$(cd "$(dirname "$0")" && pwd)"

rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"
git init -q
git config user.email demo@example.com
git config user.name demo
ai-rulez init acme --yes >/dev/null 2>&1
rm -rf .ai-rulez/skills/ai-rulez .ai-rulez/skills/code-reviewer
cp "$here/mcp-call.py" mcp-call.py

python3 - <<'PY'
import pathlib

cfg = pathlib.Path(".ai-rulez/config.toml")
cfg.write_text(cfg.read_text() + '\n[skills]\ndelivery = "static"\n')

skills = {
    "db-migrations": ("Plan and run database schema changes safely. Use when changing a table or adding a column.", "Database migrations"),
    "incident-response": ("Run an incident from page to postmortem. Use when an alert fires or production is degraded.", "Incident response"),
    "billing-api": ("Work with the billing provider API. Use when touching invoices, subscriptions or webhooks.", "Billing API"),
    "release-checklist": ("Cut and verify a release. Use when tagging a version or preparing release notes.", "Release checklist"),
    "k8s-debugging": ("Debug workloads on Kubernetes. Use when a pod crash-loops or a rollout is stuck.", "Kubernetes debugging"),
    "oauth-setup": ("Configure OAuth and OIDC login flows. Use when adding a sign-in provider or fixing token errors.", "OAuth setup"),
    "perf-profiling": ("Profile and fix slow code paths. Use when latency regresses or CPU usage spikes.", "Performance profiling"),
    "gdpr-requests": ("Handle data export and deletion requests. Use when a user asks for their data or erasure.", "GDPR requests"),
    "feature-flags": ("Roll out features behind flags. Use when adding, ramping or removing a flag.", "Feature flags"),
    "api-versioning": ("Evolve a public API without breaking clients. Use when changing a response shape or endpoint.", "API versioning"),
    "terraform-modules": ("Write and review Terraform modules. Use when changing infrastructure code.", "Terraform modules"),
    "load-testing": ("Run load tests before launch. Use when sizing capacity or checking a new endpoint.", "Load testing"),
}
points = [
    "State the goal and the rollback plan before you change anything.",
    "Make the change small, reversible and easy to review.",
    "Record every decision and exception in the pull request.",
    "Verify against a copy of production data before merging.",
    "Announce the change to the owning team and watch the dashboards.",
]
for name, (desc, title) in skills.items():
    d = pathlib.Path(".ai-rulez/skills") / name
    d.mkdir(parents=True)
    body = "\n".join(f"- {p}" for p in points)
    sections = "\n\n".join(f"## Step {i}\n\n{body}" for i in range(1, 5))
    (d / "SKILL.md").write_text(f"---\nname: {name}\ndescription: {desc}\n---\n\n# {title}\n\n{sections}\n")
PY
