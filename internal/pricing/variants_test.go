package pricing

import "testing"

// liter-llm 2.2.3 resolves a model only when the exact name (or a real revision/version/latest
// suffix) matches, so an unlisted realtime/audio/tts variant is no longer silently priced as its
// base model. It must stay unknown rather than be guessed at, and a cataloged variant must not
// come out cheaper than its base model.
func TestLookupVariantRowsNeverUndercharge(t *testing.T) {
	full, _ := Lookup("gpt-4o")
	if full.InPerMTok <= 0 || full.OutPerMTok <= 0 {
		t.Fatalf("gpt-4o must be priced, got %+v", full)
	}
	for _, name := range []string{
		"gpt-4o-realtime-preview", "openai/gpt-4o-audio-preview",
		"gpt-4o-mini-tts", "gpt-4o-mini-realtime-preview",
	} {
		if _, known := Lookup(name); known {
			t.Errorf("%s must not be priced as its base model", name)
		}
		if _, known := Cost(name, Tokens{Prompt: 1_000_000, Completion: 1_000_000}); known {
			t.Errorf("%s must not be costed as its base model", name)
		}
	}

	// gemini-2.5-flash-image is a catalog row (not a fallback): it is priced, and above flash.
	flash, ok := Lookup("gemini-2.5-flash")
	if !ok {
		t.Fatal("gemini-2.5-flash must be priced")
	}
	image, ok := Lookup("gemini-2.5-flash-image")
	if !ok {
		t.Fatal("gemini-2.5-flash-image must be a catalog row")
	}
	if image.OutPerMTok < flash.OutPerMTok {
		t.Errorf("gemini flash image %+v is below flash %+v", image, flash)
	}
	if _, known := Lookup("totally-unknown"); known {
		t.Error("unknown model must stay unknown")
	}
}
