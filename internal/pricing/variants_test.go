package pricing

import "testing"

func TestLookupVariantsNeverUndercharge(t *testing.T) {
	full, _ := Lookup("gpt-4o")
	for _, name := range []string{"gpt-4o-realtime-preview", "openai/gpt-4o-audio-preview", "gpt-4o-mini-tts", "gpt-4o-mini-realtime-preview"} {
		p, known := Lookup(name)
		if !known {
			t.Fatalf("%s should be priced", name)
		}
		if p.InPerMTok < full.InPerMTok || p.OutPerMTok < full.OutPerMTok {
			t.Errorf("%s priced %+v, below gpt-4o %+v", name, p, full)
		}
		if c, _ := Cost(name, Tokens{Prompt: 1_000_000, Completion: 1_000_000}); c < full.InPerMTok+full.OutPerMTok {
			t.Errorf("%s costs %g for 1M+1M tokens, below gpt-4o", name, c)
		}
	}
	if p, _ := Lookup("gemini-2.5-flash-image"); p.OutPerMTok < 2.50 {
		t.Errorf("gemini flash image variant priced %+v, below flash", p)
	}
	if _, known := Lookup("totally-unknown"); known {
		t.Error("unknown model must stay unknown")
	}
}
