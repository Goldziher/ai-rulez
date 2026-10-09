package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctor_ExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		strict   bool
		generate bool
		json     bool
		want     int
	}{
		{name: "clean", config: validRootConfig, generate: true, want: 0},
		{name: "clean strict", config: validRootConfig, generate: true, strict: true, want: 0},
		{name: "drift warning passes", config: validRootConfig, want: 0},
		{name: "drift warning fails with strict", config: validRootConfig, strict: true, want: exitDoctorFindings},
		{name: "removed preset is an error", config: "version = \"5.0\"\nname = \"x\"\npresets = [\"windsurf\"]\n", want: exitDoctorFindings},
		{name: "unloadable config cannot run", config: brokenRootConfig, want: exitDoctorCannotRun},
		{name: "json unloadable config cannot run", config: brokenRootConfig, json: true, want: exitDoctorCannotRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDoctorFlags(t)
			root := t.TempDir()
			writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), tt.config)
			chdir(t, root)
			if tt.generate {
				// Start from generated output so drift adds no warnings.
				if code := runRecursiveGenerate(); code != 0 {
					t.Fatalf("setup generate exit code = %d", code)
				}
			}
			doctorStrict, doctorJSON = tt.strict, tt.json
			var out bytes.Buffer

			got := runDoctor(context.Background(), &out)

			if got != tt.want {
				t.Errorf("exit code = %d, want %d\n%s", got, tt.want, out.String())
			}
			if tt.json {
				var decoded map[string]any
				if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
					t.Errorf("--json output is not valid JSON: %v\n%s", err, out.String())
				}
			} else if !strings.Contains(out.String(), "No problems") && !strings.Contains(out.String(), "error") {
				t.Errorf("text output has no summary:\n%s", out.String())
			}
		})
	}
}

func resetDoctorFlags(t *testing.T) {
	t.Helper()
	reset := func() { doctorStrict, doctorJSON, doctorProfile, noLocal, configDir = false, false, "", false, "" }
	reset()
	t.Cleanup(reset)
}
