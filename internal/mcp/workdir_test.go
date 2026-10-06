package mcp

import "testing"

func TestServeSetupWorkDir(t *testing.T) {
	tests := []struct {
		name string
		set  ServeSetup
		want string
	}{
		{"an explicit directory is used as given", ServeSetup{WorkDir: "/srv/project"}, "/srv/project"},
		{"no directory means the current one, resolved by the caller", ServeSetup{}, "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange and Act
			got := tt.set.workDir()
			// Assert
			if got != tt.want {
				t.Fatalf("workDir = %q, want %q", got, tt.want)
			}
		})
	}
}
