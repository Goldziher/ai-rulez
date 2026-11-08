package scanner

import (
	"context"
	"crypto/sha256"
	_ "embed" // adapter sources are part of the cache key
	"encoding/hex"
	"fmt"
	"time"
)

// This file names the tools and builds them. It is not part of any cache key: adding a tool or a row
// must not invalidate the verdicts of the others. What a key covers is scanner.go, the invocation and
// parsing every tool shares, and the tool's own adapter file.

// Supported reports whether the installed release can produce this row at all.
func (t *Tool) Supported(ctx context.Context) bool { return t.supported == nil || t.supported(ctx, t) }

//go:embed scanner.go
var sharedSource string

//go:embed airulez.go
var aiRulezSource string

//go:embed cisco.go
var ciscoSource string

//go:embed skillspector.go
var skillSpectorSource string

// AdapterDigest fingerprints how this tool is invoked and parsed. It is part of the verdict cache key,
// so changing an adapter invalidates that tool's cache and nothing else's.
func (t *Tool) AdapterDigest() string {
	source := map[string]string{
		toolAIRulez: aiRulezSource, toolAIRulezThreat: aiRulezSource, toolCisco: ciscoSource, toolSkillSpector: skillSpectorSource,
	}[t.Name]
	sum := sha256.Sum256([]byte(sharedSource + source))
	return hex.EncodeToString(sum[:])[:12]
}

// toolAIRulez is ai-rulez's own name, its binary and the warning-gate row.
const (
	toolAIRulez       = "ai-rulez"
	toolAIRulezThreat = "ai-rulez-threat"
	toolCisco         = "skill-scanner"
	toolSkillSpector  = "skillspector"
)

// Names lists the tools in report order. ai-rulez-threat is ai-rulez at --fail-on threat, the headline.
var Names = []string{toolAIRulezThreat, toolAIRulez, toolCisco, toolSkillSpector}

// New returns the adapter for a tool name.
func New(name, bin string, timeout time.Duration) (*Tool, error) {
	t := &Tool{Name: name, Bin: bin, Timeout: timeout, versionArgs: []string{"--version"}}
	switch name {
	case toolAIRulez:
		t.scan = scanAIRulez
	case toolAIRulezThreat:
		t.scan, t.supported, t.Bin = scanAIRulezThreat, supportsThreatGate, firstNonEmpty(bin, toolAIRulez)
	case toolCisco:
		t.scan = scanCisco
	case toolSkillSpector:
		t.scan = scanSkillSpector
	default:
		return nil, fmt.Errorf("unknown scanner %q", name)
	}
	if t.Bin == "" {
		t.Bin = name
	}
	return t, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
