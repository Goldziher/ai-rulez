package signing_test

import (
	"os"
	"testing"

	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.CeilGit()
	UseBackend(New())
	os.Exit(m.Run())
}
