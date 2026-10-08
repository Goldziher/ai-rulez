package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// warnLegacyLayout says, at info level, that configDir still uses the pre-OKF
// layout (see "ai-rulez migrate okf"). Commands that author or check the sources
// call it; generate does not, so its output stays the same.
func warnLegacyLayout(configDir string) {
	if config.IsLegacyLayout(configDir) {
		logger.Info(config.LegacyLayoutMessage, "path", configDir)
	}
}
