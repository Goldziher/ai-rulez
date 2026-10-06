package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// skillSignatureGate applies [signing] require = ["skill"] to the installed
// skills `generate` writes into harness trees: each needs a publisher
// attestation from a trusted signer, like a skill served over MCP. It runs
// before the content scan, so a skill with a forged or missing signature is
// refused as unsigned rather than scanned as if it were trusted.
func skillSignatureGate(cfg *config.Config) error {
	findings, err := signing.CheckInstalledSkills(cfg, time.Now())
	if err != nil {
		return oops.Hint("Add a [[signing.trust]] entry with subject = \"skill\" for the publisher, or drop \"skill\" from [signing] require").
			Wrapf(err, "apply [signing] require to installed skills")
	}
	if len(findings) == 0 {
		return nil
	}
	lines := make([]string, 0, len(findings))
	for _, f := range findings {
		lines = append(lines, fmt.Sprintf("%s: %s %s: %s", f.Name, f.Err.Code, signing.Names[f.Err.Code], f.Err.Reason))
	}
	return oops.Hint("Install a release the publisher signed (`ai-rulez skill update`), or review the skill and trust its signer in [[signing.trust]]").
		Errorf("installed skills fail [signing] require = [\"skill\"]; nothing was written:\n  %s", strings.Join(lines, "\n  "))
}
