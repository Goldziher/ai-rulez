package improve

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
)

// Bootstrap settings of the held-out gain interval.
const (
	// BootstrapResamples is the number of case resamples (design #227).
	BootstrapResamples = 10000
	// bootstrapConfidence is the two-sided level of the interval.
	bootstrapConfidence = 0.95
	// bootstrapTailPermille is each tail of that level in thousandths, so the percentile indexes are
	// integer arithmetic and no float rounding can move one by a resample.
	bootstrapTailPermille = 25
	permille              = 1000
)

// CI is a percentile bootstrap interval of the held-out pass-rate gain.
type CI struct {
	Low        float64 `json:"low"`
	High       float64 `json:"high"`
	Confidence float64 `json:"confidence"`
	Resamples  int     `json:"resamples"`
}

// IncludesZero reports whether the interval cannot tell the gain from no gain.
func (c *CI) IncludesZero() bool { return c.Low <= epsilon && c.High >= -epsilon }

// BootstrapGain resamples the paired held-out cases with replacement and returns
// the percentile interval of the pass-rate gain (candidate minus baseline). The
// resampling is seeded from the case ids and outcomes, so the same run always
// reports the same interval. It returns nil when there are no pairs.
func BootstrapGain(rows []PairRow, resamples int) *CI {
	n := len(rows)
	if n == 0 || resamples <= 0 {
		return nil
	}
	h := sha256.New()
	delta := make([]int, n) // per case: +1 win, -1 loss, 0 same
	for i, r := range rows {
		h.Write([]byte(r.Case))
		h.Write([]byte{0})
		switch {
		case r.Base && !r.Cand:
			delta[i] = -1 // a loss even when unstable, as the gate counts it
		case r.Unstable:
			// shown, never a win: the interval must agree with the gate
		case r.Cand && !r.Base:
			delta[i] = 1
		}
		h.Write([]byte{byte((delta[i] + 1) & 0xff)})
	}
	sum := h.Sum(nil)
	rng := rand.New(rand.NewPCG(binary.BigEndian.Uint64(sum[:8]), binary.BigEndian.Uint64(sum[8:16]))) //nolint:gosec // a statistical resample, not a secret
	gains := make([]float64, resamples)
	for b := range gains {
		total := 0
		for range n {
			total += delta[rng.IntN(n)]
		}
		gains[b] = float64(total) / float64(n)
	}
	sort.Float64s(gains)
	loIdx, hiIdx := tailIndexes(resamples)
	return &CI{Low: roundRate(gains[loIdx]), High: roundRate(gains[hiIdx]), Confidence: bootstrapConfidence, Resamples: resamples}
}

// tailIndexes are the indexes of the lower and upper percentile of n sorted resamples: the floor of the
// lower tail and the ceiling of the upper one, minus one.
func tailIndexes(n int) (lo, hi int) {
	lo = n * bootstrapTailPermille / permille
	hi = (n*(permille-bootstrapTailPermille)+permille-1)/permille - 1
	return min(lo, n-1), min(max(hi, 0), n-1)
}

// underpoweredWarning says why a comparison is weak evidence.
func underpoweredWarning(cmp *Comparison) string {
	var why []string
	if len(cmp.Table) < underpoweredBelow {
		why = append(why, fmt.Sprintf("%d held-out case(s), fewer than %d", len(cmp.Table), underpoweredBelow))
	}
	if cmp.CI != nil && cmp.CI.IncludesZero() {
		why = append(why, fmt.Sprintf("the %.0f%% bootstrap interval of the gain [%+.1f, %+.1f] points includes zero", cmp.CI.Confidence*100, cmp.CI.Low*100, cmp.CI.High*100))
	}
	return CodeUnderpowered + " underpowered: " + strings.Join(why, "; ")
}
