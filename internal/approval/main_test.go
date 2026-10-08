package approval

import (
	"os"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

func TestMain(m *testing.M) {
	signing.UseBackend(sigstore.New())
	os.Exit(m.Run())
}
