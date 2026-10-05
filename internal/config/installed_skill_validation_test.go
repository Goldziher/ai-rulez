package config

import "testing"

func TestValidateInstalledSkills(t *testing.T) {
	tests := []struct {
		name    string
		skill   InstalledSkillConfig
		wantErr bool
	}{
		{"https source", InstalledSkillConfig{Name: "a", Source: "https://github.com/o/r", Ref: "v1.0", Path: "skills/a"}, false},
		{"ssh scp source", InstalledSkillConfig{Name: "a", Source: "git@github.com:o/r.git"}, false},
		{"local source", InstalledSkillConfig{Name: "a", Source: "./vendor/skills"}, false},
		{"option source", InstalledSkillConfig{Name: "a", Source: "--upload-pack=touch x"}, true},
		{"ext helper", InstalledSkillConfig{Name: "a", Source: "ext::sh -c touch% x"}, true},
		{"ref option", InstalledSkillConfig{Name: "a", Source: "https://h/r", Ref: "--output=x"}, true},
		{"ref dotdot", InstalledSkillConfig{Name: "a", Source: "https://h/r", Ref: "a..b"}, true},
		{"absolute path", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: "/etc"}, true},
		{"dotdot path", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: "skills/../../x"}, true},
		{"missing name", InstalledSkillConfig{Source: "https://h/r"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInstalledSkills([]InstalledSkillConfig{tt.skill})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
