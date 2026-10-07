package review

import "math"

// quadraticKappa is Cohen's kappa with quadratic weights between two raters' ordinal
// labels (0..categories-1), the agreement measure the calibration reports for a
// pass < warn < fail scale. 1 is perfect agreement, 0 is chance. When both raters use
// one category throughout there is nothing to disagree on: 1 if they agree, else 0.
func quadraticKappa(a, b []int, categories int) float64 {
	n := min(len(a), len(b))
	if n == 0 || categories < 2 {
		return 0
	}
	obs := make([][]float64, categories)
	for i := range obs {
		obs[i] = make([]float64, categories)
	}
	rowSum := make([]float64, categories)
	colSum := make([]float64, categories)
	for i := range n {
		x, y := clampRank(a[i], categories), clampRank(b[i], categories)
		obs[x][y]++
		rowSum[x]++
		colSum[y]++
	}
	den := float64((categories - 1) * (categories - 1))
	var wo, we float64
	for i := range categories {
		for j := range categories {
			w := float64((i-j)*(i-j)) / den
			wo += w * obs[i][j]
			we += w * rowSum[i] * colSum[j] / float64(n)
		}
	}
	if we == 0 {
		if wo == 0 {
			return 1
		}
		return 0
	}
	return 1 - wo/we
}

func clampRank(r, categories int) int { return max(0, min(r, categories-1)) }

// wilson is the 95% Wilson score interval of a proportion; {0, 1} when n is 0.
func wilson(successes, n int) (lo, hi float64) {
	if n == 0 {
		return 0, 1
	}
	const z = 1.959964
	p := float64(successes) / float64(n)
	nf := float64(n)
	denom := 1 + z*z/nf
	center := p + z*z/(2*nf)
	margin := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf))
	return math.Max(0, (center-margin)/denom), math.Min(1, (center+margin)/denom)
}

// fleissKappa is Fleiss' kappa for subjects each rated by the same number of raters;
// counts[i][c] is how many raters gave subject i category c. It measures how well the
// k votes of the judge agree with each other beyond chance. All raters in one category
// for every subject is perfect agreement (1).
func fleissKappa(counts [][]int) float64 {
	subjects := len(counts)
	if subjects == 0 {
		return 0
	}
	cats := len(counts[0])
	raters := 0
	for _, c := range counts[0] {
		raters += c
	}
	if raters < 2 {
		return 0
	}
	catTotals := make([]float64, cats)
	var pBar float64
	for _, row := range counts {
		var sumSq float64
		for c, v := range row {
			sumSq += float64(v * v)
			catTotals[c] += float64(v)
		}
		pBar += (sumSq - float64(raters)) / float64(raters*(raters-1))
	}
	pBar /= float64(subjects)
	var pe float64
	for _, t := range catTotals {
		share := t / float64(subjects*raters)
		pe += share * share
	}
	if pe >= 1 {
		return 1
	}
	return (pBar - pe) / (1 - pe)
}

// round3 rounds to three decimals for reports and records.
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// ratio is a/b, 0 when b is 0.
func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}
