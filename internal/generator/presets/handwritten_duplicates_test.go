package presets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

func writeSettings(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestOpencodeInstructions_NeverClaimsAHandWrittenDuplicate: an instructions entry
// the user listed is theirs even when ai-rulez would write the same one.
func TestOpencodeInstructions_NeverClaimsAHandWrittenDuplicate(t *testing.T) {
	for _, spelled := range []string{"AGENTS.local.md", "./AGENTS.local.md"} {
		t.Run(spelled, func(t *testing.T) {
			// Arrange
			path := writeSettings(t, "opencode.json", `{"instructions":["`+spelled+`","docs.md"]}`)
			cfg := &config.Config{BaseDir: filepath.Dir(path), Run: config.NewRunState()}

			// Act
			entries, claimed, user, err := opencodeInstructions(cfg, path, "AGENTS.local.md", claimedInstructionsOwner(cfg, path))

			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 2 || len(claimed) != 0 || !user {
				t.Fatalf("entries=%v claimed=%v user=%v; the hand-written entry must stay unclaimed", entries, claimed, user)
			}
		})
	}
}

func TestGeminiSettingsKeys_NeverClaimsAHandWrittenDuplicate(t *testing.T) {
	// Arrange
	path := writeSettings(t, "settings.json", `{"context":{"fileName":["GEMINI.md","GEMINI.local.md","notes.md"]}}`)
	cfg := &config.Config{BaseDir: filepath.Dir(path), Run: config.NewRunState()}

	// Act
	owned, user, err := (&GeminiPresetGenerator{}).settingsKeys(path, cfg)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if !user {
		t.Fatal("the user's list must stay theirs")
	}
	for _, key := range owned {
		if len(key.Path) == 2 && key.Path[0] == "context" {
			if len(key.Elements) != 0 {
				t.Fatalf("claimed %v, but GEMINI.local.md was hand-written", key.Elements)
			}
			if got := jsonmerge.Digest(key.Value); got != jsonmerge.Digest([]string{"GEMINI.md", "GEMINI.local.md", "notes.md"}) {
				t.Fatalf("value changed: %v", key.Value)
			}
			return
		}
	}
	t.Fatal("no context.fileName key")
}
