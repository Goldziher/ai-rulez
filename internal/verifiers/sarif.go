package verifiers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/sarifout"
)

const (
	sarifSchema  = sarifout.Schema
	sarifVersion = sarifout.Version
	// fingerprintKey names the partial fingerprint; bump the suffix when the
	// recipe changes.
	fingerprintKey = "aiRulezVerifier/v1"
)

type sarifDoc struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Invocations []sarifInvocation `json:"invocations"`
	Results     []sarifResult     `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	ShortDescription sarifText    `json:"shortDescription"`
	Help             *sarifText   `json:"help,omitempty"`
	Properties       sarifRuleExt `json:"properties"`
}

type sarifRuleExt struct {
	Rule string `json:"rule,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool                `json:"executionSuccessful"`
	Notifications       []sarifNotification `json:"toolExecutionNotifications,omitempty"`
}

type sarifNotification struct {
	Level   string    `json:"level"`
	Message sarifText `json:"message"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations,omitempty"`
	RelatedLocations    []sarifLocation   `json:"relatedLocations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties          sarifResultExt    `json:"properties"`
}

type sarifResultExt struct {
	Verifier string `json:"verifier"`
	Rule     string `json:"rule,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Code     string `json:"code"`
	Fix      string `json:"fix,omitempty"`
}

type sarifLocation struct {
	ID               int           `json:"id,omitempty"`
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
	Message          *sarifText    `json:"message,omitempty"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           *sarifRegion  `json:"region,omitempty"`
}

type sarifArtifact struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// WriteSARIF writes failures and invalid declarations as SARIF 2.1.0. Each
// result carries the verifier id and the declaring rule or skill (as a related
// location and in properties); the fingerprint is stable across line shifts.
func WriteSARIF(w io.Writer, r *Report, version string) error {
	var (
		rules   = []sarifRule{}
		index   = map[string]int{}
		results = []sarifResult{}
		notes   []sarifNotification
	)
	for _, res := range r.Results {
		if res.Status == StatusError && res.Code == "" {
			notes = append(notes, sarifNotification{Level: "error", Message: sarifText{Text: res.Name + ": " + res.Message}})
			continue
		}
		if res.Status != StatusFail && res.Code != CodeVerifierInvalid {
			continue
		}
		ruleID := res.Code + "/" + res.Name
		idx, ok := index[ruleID]
		if !ok {
			idx = len(rules)
			index[ruleID] = idx
			rules = append(rules, sarifRuleFor(ruleID, res))
		}
		results = append(results, sarifResultsFor(res, ruleID, idx)...)
	}
	if r.Err != nil {
		notes = append(notes, sarifNotification{Level: "error", Message: sarifText{Text: "run failed: " + sanitize(r.Err.Error())}})
	}
	doc := sarifDoc{Schema: sarifSchema, Version: sarifVersion, Runs: []sarifRun{{
		Tool:        sarifTool{Driver: sarifDriver{Name: "ai-rulez verifiers", Version: version, InformationURI: sarifout.InformURI, Rules: rules}},
		Invocations: []sarifInvocation{{ExecutionSuccessful: len(notes) == 0 && r.Err == nil, Notifications: notes}},
		Results:     results,
	}}}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return oops.Wrapf(err, "write verifiers SARIF")
	}
	return nil
}

func sarifRuleFor(ruleID string, res Result) sarifRule {
	desc := res.Description
	if desc == "" {
		desc = res.Message
	}
	rule := sarifRule{
		ID: ruleID, Name: res.Name, ShortDescription: sarifText{Text: sanitize(desc)},
		Properties: sarifRuleExt{},
	}
	if res.Target != nil {
		rule.Properties = sarifRuleExt{Rule: res.Target.ID, Kind: res.Target.Kind}
	}
	if res.Fix != "" {
		rule.Help = &sarifText{Text: sanitize(res.Fix)}
	}
	return rule
}

func sarifResultsFor(res Result, ruleID string, idx int) []sarifResult {
	level := sarifout.Level(res.Severity)
	if res.Status == StatusError {
		level = sarifout.LevelError
	}
	findings := res.Findings
	if len(findings) == 0 {
		findings = []Finding{{Message: res.Message}}
	}
	out := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		msg := f.Message
		if res.Message != "" && res.Message != summarize(res.Findings) && len(res.Findings) > 0 {
			msg = res.Message + " (" + f.Message + ")"
		}
		sr := sarifResult{
			RuleID: ruleID, RuleIndex: idx, Level: level, Message: sarifText{Text: sanitize(msg)},
			PartialFingerprints: map[string]string{fingerprintKey: fingerprint(res.Name, f)},
			Properties:          sarifResultExt{Verifier: res.Name, Code: res.Code, Fix: sanitize(res.Fix)},
		}
		if res.Target != nil {
			sr.Properties.Rule, sr.Properties.Kind = res.Target.ID, res.Target.Kind
			if res.Target.Path != "" {
				sr.RelatedLocations = []sarifLocation{{ID: 1, PhysicalLocation: physical(res.Target.Path, res.Target.Line),
					Message: &sarifText{Text: "declared by " + res.Target.Kind + " " + sanitize(res.Target.ID)}}}
			}
		}
		switch {
		case f.File != "":
			sr.Locations = []sarifLocation{{PhysicalLocation: physical(f.File, f.Line)}}
		case res.Target != nil && res.Target.Path != "":
			sr.Locations = []sarifLocation{{PhysicalLocation: physical(res.Target.Path, res.Target.Line)}}
		case res.Source != "":
			sr.Locations = []sarifLocation{{PhysicalLocation: physical(res.Source, 0)}}
		}
		out = append(out, sr)
	}
	return out
}

func physical(p string, line int) sarifPhysical {
	uri, base := sarifout.ArtifactURI(sanitize(p))
	ph := sarifPhysical{ArtifactLocation: sarifArtifact{URI: uri, URIBaseID: base}}
	if line > 0 {
		ph.Region = &sarifRegion{StartLine: line}
	}
	return ph
}

// fingerprint hashes the verifier id, the subject path and the normalized
// matched text, so moving a violation to another line keeps its identity.
func fingerprint(verifier string, f Finding) string {
	text := f.Match
	if text == "" {
		text = f.Message
	}
	sum := sha256.Sum256([]byte(verifier + "\x00" + f.File + "\x00" + strings.Join(strings.Fields(text), " ")))
	return hex.EncodeToString(sum[:])
}
