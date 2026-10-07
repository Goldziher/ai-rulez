package skillsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRanker_VectorMinSimAbstains(t *testing.T) {
	t.Parallel()
	const minSim = 0.5
	tests := []struct {
		name        string
		cfg         Config
		query       string
		wantNoHits  bool
		wantRanking string
	}{
		{"vector mode abstains on a query nothing resembles", Config{Mode: ModeVector, VectorMinSim: minSim}, "weather today", true, ModeVector},
		{"vector mode answers a query with a close skill", Config{Mode: ModeVector, VectorMinSim: minSim}, "money back", false, ModeVector},
		{"the default hybrid fusion abstains too", Config{Mode: ModeHybrid, VectorMinSim: minSim}, "weather today", true, ModeVector},
		{"rrf drops the weak vector candidates and abstains without a lexical match", Config{Mode: ModeHybrid, Fusion: FusionRRF, VectorMinSim: minSim}, "weather today", true, ModeHybrid},
		{"without a threshold the nearest skills are always listed", Config{Mode: ModeVector}, "weather today", false, ModeVector},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			r := autoRanker(t, tt.cfg)

			// Act
			got := r.Search(t.Context(), tt.query)

			// Assert
			assert.Equal(t, tt.wantRanking, got.Ranking)
			assert.Empty(t, got.Degraded)
			assert.Equal(t, tt.wantNoHits, len(got.Hits) == 0, "hits: %v", names(r, got.Hits))
			assert.Equal(t, tt.wantNoHits, got.Abstained)
			assert.Positive(t, got.TopSim, "the best cosine is reported with or without a threshold")
		})
	}
}

func TestRanker_RRFKeepsLexicalMatchesWhenTheVectorListAbstains(t *testing.T) {
	t.Parallel()
	// Arrange: "git" matches a skill by word although no vector clears the threshold
	r := autoRanker(t, Config{Mode: ModeHybrid, Fusion: FusionRRF, VectorMinSim: 0.99})

	// Act
	got := r.Search(t.Context(), "git")

	// Assert
	require.NotEmpty(t, got.Hits)
	assert.Equal(t, "git-workflow", names(r, got.Hits)[0])
	assert.False(t, got.Abstained)
}

func TestConfig_VectorMinSimIsValidated(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   float64
		invalid bool
	}{
		{"off", 0, false},
		{"a usual cosine", 0.62, false},
		{"one", 1, false},
		{"negative", -0.1, true},
		{"above one", 1.5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			problems := (&Config{VectorMinSim: tt.value}).Validate()

			// Assert
			assert.Equal(t, tt.invalid, len(problems) > 0, "%v", problems)
		})
	}
}

func TestRanker_VectorModeAbstainsDespiteASkillWithoutAVector(t *testing.T) {
	t.Parallel()
	// Arrange: one skill changed since the index was built, so it has no vector
	r := autoRanker(t, Config{Mode: ModeVector, VectorMinSim: 0.5})
	r.Items[1].Doc.Description = "Something else entirely"

	// Act
	got := r.Search(t.Context(), "entirely")

	// Assert: the lexical fill-in of the stale skill must not hide the abstention
	assert.True(t, got.Abstained)
	assert.Empty(t, got.Hits, "hits: %v", names(r, got.Hits))
}
