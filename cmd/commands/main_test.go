package commands

import (
	"os"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.CeilGit()
	signing.UseBackend(sigstore.New())
	prepareCommandTree(RootCmd)
	snapshotFlagValues(RootCmd)
	os.Exit(m.Run())
}
