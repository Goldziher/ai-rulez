package commands

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/stretchr/testify/assert"
)

func TestEvalPrice(t *testing.T) {
	haiku, _ := evals.PriceFor("haiku")
	withEstimate := func(in, out float64) *config.Config {
		return &config.Config{Lint: &config.LintConfig{Evals: &config.LintEvals{Estimate: &config.LintEvalsEstimate{PriceInPerMTok: in, PriceOutPerMTok: out}}}}
	}
	tests := []struct {
		name     string
		cfg      *config.Config
		flagIn   float64
		flagOut  float64
		model    string
		want     evals.Price
		wantZero bool
	}{
		{name: "nothing set keeps the table", cfg: &config.Config{}, model: "haiku", wantZero: true},
		{name: "no config at all", cfg: nil, model: "haiku", wantZero: true},
		{name: "config sets both", cfg: withEstimate(0.4, 5), model: "haiku", want: evals.Price{InPerMTok: 0.4, OutPerMTok: 5}},
		{name: "config sets only the input price, the output comes from the table", cfg: withEstimate(0.4, 0), model: "haiku", want: evals.Price{InPerMTok: 0.4, OutPerMTok: haiku.OutPerMTok}},
		{name: "a flag wins over the config", cfg: withEstimate(0.4, 5), flagIn: 2, model: "haiku", want: evals.Price{InPerMTok: 2, OutPerMTok: 5}},
		{name: "a flag alone fills the other side from the table", cfg: &config.Config{}, flagOut: 9, model: "haiku", want: evals.Price{InPerMTok: haiku.InPerMTok, OutPerMTok: 9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetEvalFlags(t)
			evalFlags.priceIn, evalFlags.priceOut = tt.flagIn, tt.flagOut

			got := evalPrice(tt.cfg, tt.model)

			if tt.wantZero {
				assert.Equal(t, evals.Price{}, got)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
