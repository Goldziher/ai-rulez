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
		{"github shorthand", InstalledSkillConfig{Name: "a", Source: "owner/repo"}, false},
		{"github prefix", InstalledSkillConfig{Name: "a", Source: "github:owner/repo"}, false},
		{"github prefix without repo", InstalledSkillConfig{Name: "a", Source: "github:owner"}, true},
		{"home source", InstalledSkillConfig{Name: "a", Source: "~/skills"}, false},
		{"bare home source", InstalledSkillConfig{Name: "a", Source: "~"}, false},
		{"other user's home", InstalledSkillConfig{Name: "a", Source: "~root/skills"}, true},
		{"file url", InstalledSkillConfig{Name: "a", Source: "file:///srv/skills"}, false},
		{"option source", InstalledSkillConfig{Name: "a", Source: "--upload-pack=touch x"}, true},
		{"ext helper", InstalledSkillConfig{Name: "a", Source: "ext::sh -c touch% x"}, true},
		{"ref option", InstalledSkillConfig{Name: "a", Source: "https://h/r", Ref: "--output=x"}, true},
		{"ref dotdot", InstalledSkillConfig{Name: "a", Source: "https://h/r", Ref: "a..b"}, true},
		{"absolute path", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: "/etc"}, true},
		{"dotdot path", InstalledSkillConfig{Name: "a", Source: "https://h/r", Path: "skills/../../x"}, true},
		{"missing name", InstalledSkillConfig{Source: "https://h/r"}, true},
		{"trust error", InstalledSkillConfig{Name: "a", Source: "https://h/r", Trust: TrustError}, false},
		{"trust warn", InstalledSkillConfig{Name: "a", Source: "https://h/r", Trust: TrustWarn}, false},
		{"invalid trust", InstalledSkillConfig{Name: "a", Source: "https://h/r", Trust: "loud"}, true},
		{"include globs", InstalledSkillConfig{Name: "a", Source: "https://h/r", Include: []string{"a*", "b?"}}, false},
		{"exclude globs", InstalledSkillConfig{Name: "a", Source: "https://h/r", Exclude: []string{"*-wip"}}, false},
		{"invalid include glob", InstalledSkillConfig{Name: "a", Source: "https://h/r", Include: []string{"[bad"}}, true},
		{"invalid exclude glob", InstalledSkillConfig{Name: "a", Source: "https://h/r", Exclude: []string{"[bad"}}, true},
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
