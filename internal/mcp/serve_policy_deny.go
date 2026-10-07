package mcp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
)

// policyLoader is the part of the CLI's policy enforcer (*policy.Enforcer) that
// names the policy files in force.
type policyLoader interface {
	Load() (*policy.Resolved, error)
}

// policyFiles lists the local files of the organization policy cfg was loaded
// under (--policy, AI_RULEZ_POLICY, the managed path and what they extend), so
// the live reload sees an edit to them: a digest added to deny_digests then
// applies without a restart. Remote policies are not watched.
func policyFiles(cfg *config.Config) []string {
	loader, ok := cfg.Policy().(policyLoader)
	if !ok {
		return nil
	}
	res, err := loader.Load()
	if err != nil || res == nil {
		return nil
	}
	var out []string
	for i := range res.Layers {
		if path := res.Layers[i].Path; filepath.IsAbs(path) {
			out = append(out, path)
		}
	}
	return out
}

// takeServedDenials takes out of cfg's policy outcome the sources.deny_digests
// violations (AR747) that name a served skill the lock pins, and returns their
// digests with the policy's message. The server refuses exactly those skills
// instead of refusing to start: a denied served skill is content the server
// can leave out, unlike a denied include, which stays a load failure.
//
// The policy reports a served pin as `served skill "<name>": <lock> pins
// <digest>, ...`. A violation is taken only when it names a pin of the lock
// with exactly that name and digest, so anything else (or a change of the
// message) keeps failing the load: the fallback is the strict behavior.
func takeServedDenials(cfg *config.Config, lock *lockfile.File) map[string]string {
	out := cfg.PolicyOutcome
	if out == nil || out.Warn || lock == nil || len(lock.Served) == 0 {
		return nil
	}
	denied := map[string]string{}
	kept := make([]config.PolicyViolation, 0, len(out.Violations))
	for _, v := range out.Violations {
		if digest, ok := servedDenial(v, lock); ok {
			denied[digest] = v.Message
			continue
		}
		kept = append(kept, v)
	}
	if len(denied) == 0 {
		return nil
	}
	trimmed := *out
	trimmed.Violations = kept
	cfg.PolicyOutcome = &trimmed
	return denied
}

// servedDenial reports the digest of the lock's served pin that an AR747
// violation names, if it names one.
func servedDenial(v config.PolicyViolation, lock *lockfile.File) (string, bool) {
	if v.Code != lint.CodeDigestDenied {
		return "", false
	}
	for i := range lock.Served {
		e := &lock.Served[i]
		prefix := fmt.Sprintf("served skill %q: %s pins %s,", e.Name, lockfile.FileName, e.Digest)
		if e.Digest != "" && strings.HasPrefix(v.Message, prefix) {
			return e.Digest, true
		}
	}
	return "", false
}
