package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOKFIndexStyle(t *testing.T) {
	tests := []struct {
		name    string
		okf     *OKFConfig
		want    string
		wantErr bool
	}{
		{"unset defaults to body", nil, OKFIndexStyleBody, false},
		{"empty section defaults to body", &OKFConfig{}, OKFIndexStyleBody, false},
		{"body", &OKFConfig{IndexStyle: "body"}, OKFIndexStyleBody, false},
		{"frontmatter", &OKFConfig{IndexStyle: "frontmatter"}, OKFIndexStyleFrontmatter, false},
		{"unknown", &OKFConfig{IndexStyle: "yaml"}, "yaml", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{OKF: tt.okf}
			// Act
			err := cfg.validateOKF()
			// Assert
			assert.Equal(t, tt.want, cfg.OKFIndexStyle())
			if tt.wantErr {
				assert.ErrorContains(t, err, `unknown okf.index_style "yaml"`)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestDecodeConfigTOMLReadsTheOKFSection(t *testing.T) {
	// Arrange
	data := []byte("version = \"4.0\"\nname = \"x\"\n\n[okf]\ndir = \"docs/kb\"\ninclude = [\"rules\"]\nindex_style = \"frontmatter\"\n")
	// Act
	cfg, err := decodeConfigTOML(data, "config.toml")
	// Assert
	assert.NoError(t, err)
	assert.Equal(t, "docs/kb", cfg.OKFDir())
	assert.Equal(t, []string{"rules"}, cfg.OKFInclude())
	assert.Equal(t, OKFIndexStyleFrontmatter, cfg.OKFIndexStyle())
}
