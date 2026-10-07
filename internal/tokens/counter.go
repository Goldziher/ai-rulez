// Package tokens provides offline, deterministic token counting for generated
// AI-assistant artifacts.
//
// No counter here is exact for Claude: Anthropic does not publish its
// tokenizer, so every number this package produces is an approximation. The
// default counter uses the cl100k_base BPE vocabulary from
// github.com/tiktoken-go/tokenizer (MIT), which is compiled into the binary —
// counting needs no network access and no API key, so it works in CI and in
// pre-commit hooks.
//
// Calibration: one real 19,230-byte generated instruction file whose exact
// Claude Code prompt cost was measured at 4,879 tokens counts as 4,487 tokens
// under cl100k_base — 8% low. Use these counts to compare (profile against
// profile, before an edit against after), never as a prediction of a session
// total.
package tokens

import (
	"fmt"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/tiktoken-go/tokenizer/codec"
)

// Counter counts tokens in a rendered artifact.
type Counter interface {
	// Count returns the approximate number of tokens in text.
	Count(text string) int
	// Name identifies the counting method so reports can label their numbers.
	Name() string
	// IsEstimate reports whether Name refers to a byte-ratio estimate rather
	// than a real byte-pair-encoding tokenizer.
	IsEstimate() bool
}

// Counter names accepted by New.
const (
	// CounterCL100KBase is the default: the cl100k_base BPE vocabulary,
	// embedded in the binary.
	CounterCL100KBase = "cl100k_base"
	// CounterEstimate is the byte-ratio fallback. Only meaningful for a single
	// prose artifact; across a whole generated tree the real ratio has been
	// measured between 1.81 and 5.30 bytes per token, so the estimate can be
	// wrong by a factor of three.
	CounterEstimate = "estimate"
)

// EstimateBytesPerToken is the ratio used by the byte-ratio counter. It is the
// measured ratio of a single generated markdown instruction file (19,230 bytes,
// 4,879 real tokens). It is deliberately not applied to whole trees.
const EstimateBytesPerToken = 3.94

// New returns the counter registered under name. An empty name selects the
// default cl100k_base counter.
func New(name string) (Counter, error) {
	switch name {
	case "", CounterCL100KBase:
		return CL100KBase(), nil
	case CounterEstimate:
		return ByteRatio(EstimateBytesPerToken), nil
	default:
		return nil, fmt.Errorf("unknown tokenizer %q (want %q or %q)", name, CounterCL100KBase, CounterEstimate)
	}
}

// Names returns the accepted counter names, for flag help and error hints.
func Names() []string {
	return []string{CounterCL100KBase, CounterEstimate}
}

// cl100kOnce keeps the ~5 MB vocabulary map construction to at most once per
// process. Loading measures around 5 ms; commands that never count pay nothing.
var cl100kOnce = sync.OnceValue(codec.NewCl100kBase)

type cl100kCounter struct{}

// CL100KBase returns the embedded cl100k_base BPE counter. The vocabulary is
// built on first use.
func CL100KBase() Counter {
	return cl100kCounter{}
}

func (cl100kCounter) Name() string { return CounterCL100KBase }

func (cl100kCounter) IsEstimate() bool { return false }

// Count encodes text with cl100k_base. The underlying tokenizer can only fail
// on a regular-expression engine error, which no input encountered so far
// triggers; if it ever does, Count degrades to the byte-ratio estimate and logs
// the reason rather than reporting a silent zero.
func (c cl100kCounter) Count(text string) int {
	if text == "" {
		return 0
	}
	count, err := cl100kOnce().Count(text)
	if err != nil {
		logger.Std().Warn("cl100k_base tokenizer failed, falling back to a byte-ratio estimate",
			"error", err, "bytes", len(text))
		return ByteRatio(EstimateBytesPerToken).Count(text)
	}
	return count
}

type byteRatioCounter struct {
	bytesPerToken float64
}

// ByteRatio returns a counter that divides byte length by bytesPerToken. It is
// a labeled estimate, not a tokenizer: callers must surface IsEstimate in any
// output so a reader never mistakes the number for a measurement.
func ByteRatio(bytesPerToken float64) Counter {
	if bytesPerToken <= 0 {
		bytesPerToken = EstimateBytesPerToken
	}
	return byteRatioCounter{bytesPerToken: bytesPerToken}
}

func (byteRatioCounter) Name() string { return CounterEstimate }

func (byteRatioCounter) IsEstimate() bool { return true }

func (c byteRatioCounter) Count(text string) int {
	if text == "" {
		return 0
	}
	count := int(float64(len(text))/c.bytesPerToken + 0.5)
	if count < 1 {
		return 1
	}
	return count
}
