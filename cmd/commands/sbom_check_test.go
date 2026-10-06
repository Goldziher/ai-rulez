package commands

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestSBOMCheck_JudgesApprovalsAtTheTimeOfTheCommittedDocument(t *testing.T) {
	// Arrange: an approval that expires on 2026-06-01, an SBOM committed before that
	root := sbomProject(t, sbomBaseConfig+"\n[governance]\nrequire_approval = [\"kind:rule\"]\n", map[string]string{"rules/r.md": "# R\n"})
	cfg, err := config.LoadConfig(context.Background(), root, config.WithoutLocal())
	require.NoError(t, err)
	snap, err := govview.Snapshot(cfg, "", true, "9.9.9")
	require.NoError(t, err)
	lock := &lockfile.File{}
	contentlock.Build(lock, snap)
	var digest string
	for _, it := range lock.Item {
		if it.Kind == "rule" && it.ID == "r" {
			digest = it.Digest
		}
	}
	require.NotEmpty(t, digest)
	lock.SetApproval(lockfile.Approval{Kind: "rule", ID: "r", Digest: digest, Reviewer: "alice@example.org",
		Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-03-01T00:00:00Z", Expires: "2026-06-01"})
	require.NoError(t, lockfile.Save(cfg.ConfigDir, lock))
	committed := filepath.Join(root, "sbom.cdx.json")
	stamp := "2026-03-02T00:00:00Z"
	t.Cleanup(func() { sbomNow = time.Now })
	sbomNow = func() time.Time { return time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) }
	var out, errOut = new(bytes.Buffer), new(bytes.Buffer)
	require.Equal(t, 0, runSBOM(out, errOut, sbomFlags{format: "cyclonedx", output: committed, timestamp: stamp}, true), errOut.String())

	// Act: the check runs after the approval expired
	sbomNow = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }
	out, errOut = new(bytes.Buffer), new(bytes.Buffer)
	code := runSBOM(out, errOut, sbomFlags{format: "cyclonedx", output: committed, timestamp: stamp, check: true}, true)

	// Assert
	assert.Equal(t, 0, code, errOut.String())
}
