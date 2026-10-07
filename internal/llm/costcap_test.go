package llm

import (
	"math"
	"testing"
)

func TestRepoNonFiniteOrNegativeCostCapCannotLoosenTheUsers(t *testing.T) {
	user := &Config{MaxCostUSD: 5}
	for name, bad := range map[string]float64{"nan": math.NaN(), "inf": math.Inf(1), "neg-inf": math.Inf(-1), "negative": -1} {
		got, _ := Resolve(&Config{MaxCostUSD: bad}, user)
		if got.MaxCostUSD != 5 {
			t.Errorf("%s: repo value changed the user's cap to %v", name, got.MaxCostUSD)
		}
		// no user cap: a bad repo value must not become the effective cap either
		got, _ = Resolve(&Config{MaxCostUSD: bad}, &Config{})
		if got.MaxCostUSD != 0 {
			t.Errorf("%s: repo value became the effective cap %v", name, got.MaxCostUSD)
		}
	}
	for _, c := range []Config{{MaxCostUSD: math.NaN()}, {MaxCostUSD: math.Inf(1)}, {PriceInputPerMTok: math.NaN()}, {PriceOutputPerMTok: math.Inf(1)}} {
		if len(c.Validate()) == 0 {
			t.Errorf("Validate accepted a non-finite number: %+v", c)
		}
	}
}

func TestEnvRejectsNonFiniteCost(t *testing.T) {
	for _, v := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf"} {
		_, err := Config{}.WithEnv(func(k string) string {
			if k == "AI_RULEZ_LLM_MAX_COST_USD" {
				return v
			}
			return ""
		})
		if err == nil {
			t.Errorf("AI_RULEZ_LLM_MAX_COST_USD=%s was accepted", v)
		}
	}
}
