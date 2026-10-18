package mcp

import (
	"os"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.RemoveTestBinary()
	os.Exit(code)
}
