package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// warnLegacyLayout says, at info level, that configDir still uses the pre-OKF
// layout (see "ai-rulez migrate okf"). The commands that write the sources (add,
// remove) call it; generate and validate do not, so their output, which the
// golden suite pins byte for byte, stays the same.
func warnLegacyLayout(configDir string) {
	if config.IsLegacyLayout(configDir) {
		logger.Info(config.LegacyLayoutMessage, "path", configDir)
	}
}
