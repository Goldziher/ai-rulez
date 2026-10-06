package improve

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
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
		case r.Unstable:
			// shown, never a win or a loss: the interval must agree with the gate
		case r.Cand && !r.Base:
			delta[i] = 1
		case r.Base && !r.Cand:
			delta[i] = -1
		}
		h.Write([]byte{byte(delta[i] + 1)})
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
	tail := (1 - bootstrapConfidence) / 2
	lo := gains[int(math.Floor(tail*float64(resamples)))]
	hi := gains[min(int(math.Ceil((1-tail)*float64(resamples)))-1, resamples-1)]
	return &CI{Low: roundRate(lo), High: roundRate(hi), Confidence: bootstrapConfidence, Resamples: resamples}
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
