package llm

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// TOML accepts nan and inf, and every comparison with NaN is false: a NaN cap passed `v < 0`,
// survived the min-merge as min(user, NaN) = NaN and then admitted every call (RV-LLM-2).
func TestValidateShouldRejectNonFiniteNumbers(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"nan cost cap", Config{MaxCostUSD: math.NaN()}, "max_cost_usd must be a finite number"},
		{"infinite cost cap", Config{MaxCostUSD: math.Inf(1)}, "max_cost_usd must be a finite number"},
		{"nan input price", Config{PriceInputPerMTok: math.NaN()}, "price_input_per_mtok must be a finite number"},
		{"negative infinite output price", Config{PriceOutputPerMTok: math.Inf(-1)}, "price_output_per_mtok must be a finite number"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			err := tc.cfg.Err()

			// Assert
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Err() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestResolveShouldIgnoreANonFiniteRepositoryCap(t *testing.T) {
	tests := []struct {
		name     string
		repoTOML string
		user     *Config
		want     float64
	}{
		{"nan repo cap keeps the user cap", "max_cost_usd = nan", &Config{MaxCostUSD: 0.01}, 0.01},
		{"inf repo cap keeps the user cap", "max_cost_usd = inf", &Config{MaxCostUSD: 0.01}, 0.01},
		{"nan repo cap alone is unset", "max_cost_usd = nan", nil, 0},
		{"finite repo cap still tightens", "max_cost_usd = 0.005", &Config{MaxCostUSD: 0.01}, 0.005},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var doc struct {
				LLM *Config `toml:"llm"`
			}
			if err := toml.Unmarshal([]byte("[llm]\n"+tc.repoTOML+"\n"), &doc); err != nil {
				t.Fatalf("toml: %v", err)
			}

			// Act
			cfg, _ := Resolve(doc.LLM, tc.user)

			// Assert
			if cfg.MaxCostUSD != tc.want {
				t.Errorf("max_cost_usd = %v, want %v", cfg.MaxCostUSD, tc.want)
			}
		})
	}
}

// The reviewer's repro: a repository NaN under a $0.01 user cap admitted five ~$0.29 calls.
func TestRepositoryNaNCostShouldNotLiftTheUserCap(t *testing.T) {
	// Arrange
	var doc struct {
		LLM *Config `toml:"llm"`
	}
	if err := toml.Unmarshal([]byte("[llm]\nmax_cost_usd = nan\nmodel = \"gpt-4o\"\n"), &doc); err != nil {
		t.Fatalf("toml: %v", err)
	}
	cfg, _ := Resolve(doc.LLM, &Config{AllowNetwork: true, MaxCostUSD: 0.01, Model: "gpt-4o", Provider: "openai"})
	m := Wrap(NewFake(), cfg, Options{NoCache: true, Retry: &RetryPolicy{}})

	// Act
	_, err := m.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: string(make([]byte, 300_000))}}, MaxTokens: 4000})

	// Assert
	if !errors.Is(err, ErrBudget) {
		t.Fatalf("a $0.29 call under a $0.01 cap: err = %v, want a budget refusal", err)
	}
}

// A budget handed a NaN cap directly refuses rather than treating it as unlimited.
func TestBudgetShouldRefuseANaNCap(t *testing.T) {
	// Arrange
	b := NewBudget(Limits{MaxCostUSD: math.NaN()}, NewPricing(Config{PriceInputPerMTok: 1, PriceOutputPerMTok: 1}))
	c := WithBudget(NewFake(), b, "m", "e")

	// Act
	_, err := c.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}})

	// Assert
	if !errors.Is(err, ErrBudget) {
		t.Fatalf("err = %v, want a budget refusal", err)
	}
}

// A NaN temperature cannot be encoded; it used to collapse every such request onto one cache key
// (RV-LLM-22). It is refused before the cache and the provider are reached.
func TestChatShouldRefuseANonFiniteTemperature(t *testing.T) {
	for _, temp := range []float64{math.NaN(), math.Inf(1)} {
		// Arrange
		f := NewFake()
		m := Wrap(f, Config{AllowNetwork: true, Model: "m"}, Options{CacheDir: t.TempDir(), SecretPath: t.TempDir() + "/k"})

		// Act
		_, err := m.Chat(context.Background(), ChatRequest{Temperature: temp, Messages: []Message{{Role: RoleUser, Content: "A"}}})

		// Assert
		if !errors.Is(err, ErrConfig) || len(f.ChatCalls()) != 0 {
			t.Errorf("temperature %v: err = %v, calls = %d; want a config error and no call", temp, err, len(f.ChatCalls()))
		}
	}
}

func TestCacheKeyShouldNeverCollapseUnencodableRequests(t *testing.T) {
	// Arrange
	c := NewCache(t.TempDir(), "id", []byte("0123456789abcdef0123456789abcdef"))

	// Act
	a, okA := c.key("chat", "m", ChatRequest{Temperature: math.NaN(), Messages: []Message{{Role: RoleUser, Content: "A"}}})
	b, okB := c.key("chat", "m", ChatRequest{Temperature: math.NaN(), Messages: []Message{{Role: RoleUser, Content: "B"}}})

	// Assert
	if okA || okB || a != "" || b != "" {
		t.Errorf("keys = %q (%v), %q (%v); want no key for a request that cannot be encoded", a, okA, b, okB)
	}
}
