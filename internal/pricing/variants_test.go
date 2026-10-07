package pricing

import "testing"

func TestLookupVariantsNeverUndercharge(t *testing.T) {
	mini, _ := Lookup("gpt-4o-mini")
	full, _ := Lookup("gpt-4o")
	for _, name := range []string{"gpt-4o-realtime-preview", "openai/gpt-4o-audio-preview", "gpt-4o-mini-tts", "gpt-4o-mini-realtime-preview"} {
		p, known := Lookup(name)
		if !known {
			t.Fatalf("%s should fall back to a conservative price", name)
		}
		if p.InPerMTok < full.InPerMTok || p.OutPerMTok < full.OutPerMTok {
			t.Errorf("%s priced %+v, below the highest gpt sibling %+v", name, p, full)
		}
	}
	if p, _ := Lookup("gemini-2.5-flash-image"); p.OutPerMTok < 2.50 {
		t.Errorf("gemini flash image variant priced %+v, below flash", p)
	}
	// dated and versioned snapshots keep the base price
	for name, want := range map[string]Price{
		"gpt-4o-mini-2024-07-18":           mini,
		"gpt-4o-2024-08-06":                full,
		"claude-sonnet-4-5-20250929":       {3.00, 15.00},
		"gemini-2.5-flash-lite-preview-06": {0.10, 0.40},
		"gpt-4o-mini":                      mini,
	} {
		if p, known := Lookup(name); !known || p != want {
			t.Errorf("%s = %+v (%v), want %+v", name, p, known, want)
		}
	}
	if _, known := Lookup("totally-unknown"); known {
		t.Error("unknown model must stay unknown")
	}
}
