package evals

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalibrate_ProposesTheBilledInputPriceWhenCachingDiscountsIt(t *testing.T) {
	// Arrange: haiku lists at $1 in and $5 out per million. 100 runs of 25,000 input
	// and 300 output tokens cost $0.40 in total: $1.5 of list output leaves $0.40 - ...
	// is worked out below, not assumed.
	price, known := PriceFor("haiku")
	require.True(t, known)
	in, out, runs := 25000, 300, 4
	billedPerRun := 0.0100 // dollars: far below 25,000 tokens at list price
	rec := estimateRecord(runs, runs*2000, runs*150, runs*in, runs*out, 1)
	rec.ActualUSD = billedPerRun * float64(runs)
	store := storeWith(SkillRecord{ID: "a", Harness: "claude", Model: "haiku", Estimate: rec})

	// Act
	cal := Calibrate(store, &CalibrateOptions{})

	// Assert
	g := cal.Groups[0]
	assert.Equal(t, price.InPerMTok, g.ListPriceIn)
	want := (rec.ActualUSD*1e6 - float64(runs*out)*price.OutPerMTok) / float64(runs*in)
	assert.InDelta(t, want, g.EffectivePriceIn, 1e-3)
	assert.Less(t, g.EffectivePriceIn, g.ListPriceIn/2)
	var text bytes.Buffer
	require.NoError(t, cal.WriteText(&text))
	assert.Regexp(t, `price_in_per_mtok = \d+(\.\d+)?  # list price \$1:`, text.String(), "a TOML number, not a dollar amount")
	assert.Contains(t, text.String(), "prompt caching")
}

func TestCalibrate_NoPriceProposalWithoutCostOrForAnUnlistedModel(t *testing.T) {
	noCost := estimateRecord(4, 8000, 2400, 100000, 1200, 1)
	unlisted := estimateRecord(4, 8000, 2400, 100000, 1200, 1)
	unlisted.ActualUSD = 0.04
	store := storeWith(
		SkillRecord{ID: "a", Harness: "claude", Model: "haiku", Estimate: noCost},
		SkillRecord{ID: "b", Harness: "other", Model: "mystery-9", Estimate: unlisted},
	)

	cal := Calibrate(store, &CalibrateOptions{})

	for _, g := range cal.Groups {
		assert.Zero(t, g.EffectivePriceIn, "%s/%s", g.Harness, g.Model)
	}
	var text bytes.Buffer
	require.NoError(t, cal.WriteText(&text))
	assert.NotContains(t, text.String(), "price_in_per_mtok")
}

func TestPriceDiffers(t *testing.T) {
	assert.True(t, priceDiffers(CalibrationGroup{ListPriceIn: 1, EffectivePriceIn: 0.4}))
	assert.False(t, priceDiffers(CalibrationGroup{ListPriceIn: 1, EffectivePriceIn: 1.05}))
	assert.False(t, priceDiffers(CalibrationGroup{ListPriceIn: 1}))
	assert.False(t, priceDiffers(CalibrationGroup{EffectivePriceIn: 1}))
}
