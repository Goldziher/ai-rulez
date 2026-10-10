package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// warnLegacyLayout says, at info level, that configDir still uses the pre-OKF
// layout (see "ai-rulez migrate okf"). generate, validate and list call it, along
// with the commands that write the sources (add, remove), so a user reading a
// legacy tree is told once per command that it is deprecated.
func warnLegacyLayout(configDir string) {
	if config.IsLegacyLayout(configDir) {
		logger.Info(config.LegacyLayoutMessage, "path", configDir)
	}
}
