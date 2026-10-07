package mcp

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestServeSetup_LoadWithoutLogOrSinkWritesNothing(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	st := &ServeSetup{}
	record, closeSink := st.telemetry(&config.Config{ConfigDir: filepath.Join(dir, ".ai-rulez")})

	// Act
	record(SessionTelemetry{Skill: "kit", Session: "conn-1", Client: "c"})
	closeSink(0)

	// Assert
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez", "local"))
}
