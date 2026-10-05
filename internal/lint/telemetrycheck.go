package lint

import (
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/telemetry"
)

// Codes of the telemetry checks (see docs/telemetry.md). AR9K is a block of its
// own: AR9D, AR9E and AR9F are proposed by other open designs.
const (
	CodeTelemetryConfigInvalid = "AR9K0"
	CodeTelemetryKeyIgnored    = "AR9K1"
)

// The telemetry checks register themselves so this file is the only place to touch.
func init() {
	registry = append(registry,
		RuleInfo{CodeTelemetryConfigInvalid, "telemetry-config-invalid", SeverityError, "a [telemetry] setting is invalid: bad enum or range, unsupported protocol, non-https endpoint, or a literal credential in headers_env"},
		RuleInfo{CodeTelemetryKeyIgnored, "telemetry-repo-key-ignored", SeverityWarning, "a repository [telemetry] sets a key only user scope may set (allow_network, otlp_endpoint, headers_env, ...); it is ignored"},
	)
}

// checkTelemetry reports an invalid [telemetry] table (AR9K0) and repository
// keys that the trust rule ignores (AR9K1).
func (r *runner) checkTelemetry() {
	t := r.cfg.Telemetry
	if t == nil || r.cfg.ConfigDir == "" {
		return
	}
	file := filepath.Join(r.cfg.ConfigDir, r.cfg.ConfigFile)
	for _, problem := range t.Validate() {
		r.add(CodeTelemetryConfigInvalid, file, 1, "%s", problem)
	}
	if keys := telemetry.PrivilegedKeys(t); len(keys) > 0 {
		r.add(CodeTelemetryKeyIgnored, file, 1,
			"[telemetry] %s set in the repository config: ignored, these keys are honored only in the user config or AI_RULEZ_TELEMETRY_* variables", strings.Join(keys, ", "))
	}
}
