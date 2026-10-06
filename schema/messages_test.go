package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRefineMessages(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "windsurf is renamed",
			in: []string{
				"- presets.0.oneOf: Value does not match the oneOf schema",
				"- presets.0.enum: Value windsurf should be one of the allowed values: claude, devin",
				"- presets.0.type: Value is string but should be object",
				"- presets.0.not: Value should not match the not schema",
			},
			want: []string{`- presets.0: preset "windsurf" was renamed to "devin"; use "devin" instead`},
		},
		{
			name: "continue-dev is removed",
			in:   []string{"- presets.1.enum: Value continue-dev should be one of the allowed values: claude"},
			want: []string{`- presets.1: preset "continue-dev" was removed and has no replacement; drop it from presets`},
		},
		{
			name: "other unknown preset is untouched",
			in:   []string{"- presets.0.enum: Value nope should be one of the allowed values: claude"},
			want: []string{"- presets.0.enum: Value nope should be one of the allowed values: claude"},
		},
		{
			name: "missing version is reported once",
			in: []string{
				"- 'version': required field is missing",
				"- version: required field is missing (expected string)",
				"- version.enum: Value <nil> should be one of the allowed values: 3.0, 4.0",
				"- name: required field is missing",
			},
			want: []string{
				`- version: the required key is missing; add version = "4.0" at the top of the file`,
				"- name: required field is missing",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			got := refineMessages(tt.in)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
