package skillsource

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestFromConfigCopiesDomain(t *testing.T) {
	// Arrange
	src := config.SkillSourceConfig{Name: "team", URL: "https://example.com/x.git", Domain: "billing"}

	// Act
	spec := FromConfig(&src)

	// Assert
	assert.Equal(t, "billing", spec.Domain)
}
