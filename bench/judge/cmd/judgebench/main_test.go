package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	judgebench "github.com/Goldziher/ai-rulez/v5/bench/judge"
)

func TestRunLiveReportsPreparationFailure(t *testing.T) {
	m := judgebench.Manifest{Root: filepath.Join(t.TempDir(), "missing")}
	err := runLive(m, options{maxCost: 1}, "unused", t.TempDir())
	require.ErrorContains(t, err, "copy the corpus")
}
