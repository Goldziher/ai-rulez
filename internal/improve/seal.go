package improve

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// ReportMACFile sits beside report.json and binds the run to this machine's
// user: an HMAC of the report under the per-user eval key, which lives outside
// the repository. A report.json that was copied in, edited or produced by
// anything but `improve run` on this machine carries no valid MAC and is not
// applied. The MAC does not stop code running as the same user (the optimizer)
// from reading the key; docs/improve.md says so.
const ReportMACFile = "report.mac"

const macDomain = "ai-rulez improve report v1\n"

func reportMAC(key []byte, runID string, reportJSON []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(macDomain + runID + "\n"))
	h.Write(reportJSON)
	return hex.EncodeToString(h.Sum(nil))
}

// sealReport writes the MAC of the report at runDir. With no key (no user
// config directory) nothing is written and the run cannot be applied.
func sealReport(runDir, runID string, reportJSON []byte) error {
	key := evals.UserKey()
	if key == nil {
		return nil
	}
	return safefs.WriteFileAtomic(filepath.Join(runDir, ReportMACFile), []byte(reportMAC(key, runID, reportJSON)+"\n")) //nolint:wrapcheck // safefs errors name the path
}

// verifyReport reports whether report.json in runDir carries a valid MAC.
func verifyReport(runDir, runID string, reportJSON []byte) bool {
	key := evals.ExistingUserKey()
	if key == nil {
		return false
	}
	got, err := safefs.ReadRegular(filepath.Join(runDir, ReportMACFile))
	if err != nil {
		return false
	}
	want := reportMAC(key, runID, reportJSON)
	return hmac.Equal([]byte(strings.TrimSpace(string(got))), []byte(want))
}
