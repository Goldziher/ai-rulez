package cli

import (
	"os"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	e2eutil "github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func TestMain(m *testing.M) {
	testutil.CeilGit()
	code := m.Run()
	e2eutil.RemoveTestBinary()
	os.Exit(code)
}
