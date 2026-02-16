package config

import "testing"

func TestIsValidTriggerMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		expected bool
	}{
		{
			name:     "valid manual mode",
			mode:     TriggerManual,
			expected: true,
		},
		{
			name:     "valid always_on mode",
			mode:     TriggerAlwaysOn,
			expected: true,
		},
		{
			name:     "valid model_decision mode",
			mode:     TriggerModelDecision,
			expected: true,
		},
		{
			name:     "valid glob mode",
			mode:     TriggerGlob,
			expected: true,
		},
		{
			name:     "invalid mode",
			mode:     "unknown",
			expected: false,
		},
		{
			name:     "empty mode",
			mode:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidTriggerMode(tt.mode)
			if result != tt.expected {
				t.Errorf("IsValidTriggerMode(%q) = %v, want %v", tt.mode, result, tt.expected)
			}
		})
	}
}

func TestGetTriggerMode(t *testing.T) {
	tests := []struct {
		name     string
		metadata *MetadataV3
		expected string
	}{
		{
			name:     "nil metadata returns manual",
			metadata: nil,
			expected: TriggerManual,
		},
		{
			name:     "nil Extra returns manual",
			metadata: &MetadataV3{},
			expected: TriggerManual,
		},
		{
			name: "empty Extra returns manual",
			metadata: &MetadataV3{
				Extra: map[string]string{},
			},
			expected: TriggerManual,
		},
		{
			name: "valid manual mode",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "manual",
				},
			},
			expected: TriggerManual,
		},
		{
			name: "valid always_on mode",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "always_on",
				},
			},
			expected: TriggerAlwaysOn,
		},
		{
			name: "valid model_decision mode",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "model_decision",
				},
			},
			expected: TriggerModelDecision,
		},
		{
			name: "invalid mode returns manual (graceful degradation)",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "invalid_mode",
				},
			},
			expected: TriggerManual, // degrades to default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetTriggerMode()
			if result != tt.expected {
				t.Errorf("GetTriggerMode() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetTriggerDescription(t *testing.T) {
	tests := []struct {
		name     string
		metadata *MetadataV3
		expected string
	}{
		{
			name:     "nil metadata returns empty",
			metadata: nil,
			expected: "",
		},
		{
			name: "with description",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"description": "Apply when working with APIs",
				},
			},
			expected: "Apply when working with APIs",
		},
		{
			name: "without description",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "manual",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetTriggerDescription()
			if result != tt.expected {
				t.Errorf("GetTriggerDescription() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetTriggerGlob(t *testing.T) {
	tests := []struct {
		name     string
		metadata *MetadataV3
		expected string
	}{
		{
			name:     "nil metadata returns empty",
			metadata: nil,
			expected: "",
		},
		{
			name: "with glob pattern",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"glob": "**/*.ts",
				},
			},
			expected: "**/*.ts",
		},
		{
			name: "without glob",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "manual",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.GetTriggerGlob()
			if result != tt.expected {
				t.Errorf("GetTriggerGlob() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestShouldRenderTriggerFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		metadata *MetadataV3
		expected bool
	}{
		{
			name:     "nil metadata returns false",
			metadata: nil,
			expected: false,
		},
		{
			name:     "default manual mode with no extra returns false",
			metadata: &MetadataV3{},
			expected: false,
		},
		{
			name: "explicit manual mode with no extra returns false",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "manual",
				},
			},
			expected: false,
		},
		{
			name: "model_decision mode returns true",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "model_decision",
				},
			},
			expected: true,
		},
		{
			name: "always_on mode returns true",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "always_on",
				},
			},
			expected: true,
		},
		{
			name: "manual mode with description returns true",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger":     "manual",
					"description": "Apply for API files",
				},
			},
			expected: true,
		},
		{
			name: "manual mode with glob returns true",
			metadata: &MetadataV3{
				Extra: map[string]string{
					"trigger": "manual",
					"glob":    "**/*.ts",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.metadata.ShouldRenderTriggerFrontmatter()
			if result != tt.expected {
				t.Errorf("ShouldRenderTriggerFrontmatter() = %v, want %v", result, tt.expected)
			}
		})
	}
}
