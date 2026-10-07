package includes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestCheckLocalOverride(t *testing.T) {
	overlay := func(list, name, override string) *config.LocalOverlay {
		return &config.LocalOverlay{Doc: map[string]any{list: []any{map[string]any{"name": name, "local_override": override}}}}
	}
	tests := []struct {
		name    string
		mode    LockMode
		overlay *config.LocalOverlay
		wantErr bool
	}{
		{name: "committed override without lock enforcement is honored", mode: LockAuto},
		{name: "committed override under --locked is refused", mode: LockRequire, wantErr: true},
		{name: "committed override under --frozen is refused", mode: LockFrozen, wantErr: true},
		{name: "overlay override under --locked is honored", mode: LockRequire, overlay: overlay("includes", "shared", "../x")},
		{name: "overlay override for another entry does not cover this one", mode: LockRequire, overlay: overlay("includes", "other", "../x"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{LocalOverlay: tt.overlay, LockPolicy: config.LockPolicy{Mode: tt.mode}}

			// Act
			err := checkLocalOverride(cfg, "includes", "shared")

			// Assert
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, config.ErrLockViolation)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestResolver_CreateSource_RefusesCommittedLocalOverrideUnderLock(t *testing.T) {
	// Arrange
	r := &Resolver{baseDir: t.TempDir(), cfg: &config.Config{LockPolicy: config.LockPolicy{Mode: LockRequire}}}

	// Act
	_, err := r.createSource(context.Background(), &config.IncludeConfig{Name: "shared", Source: "https://example.com/r.git", LocalOverride: "../x"})

	// Assert
	assert.ErrorIs(t, err, config.ErrLockViolation)
}
