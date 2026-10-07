package mcp

import (
	"os"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.CeilGit()
	os.Exit(m.Run())
}
