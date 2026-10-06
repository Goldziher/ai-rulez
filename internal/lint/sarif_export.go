package lint

import (
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/sarifout"
)

// SARIF 2.1.0 output. Rule ids are the AR codes, artifact locations are
// relative to the repository root (uriBaseId %SRCROOT%), and every result
// carries a partialFingerprint that does not depend on the line number, so code
// scanning keeps tracking an alert after the file is edited above it.

const (
	sarifSchema      = sarifout.Schema
	sarifFingerprint = "aiRulezFingerprint/v1"
	toolName         = "ai-rulez"
	toolInfoURI      = sarifout.InformURI
)

type sarifDocument struct {
	Schema  string        `json:"$schema"`
	Version string        `json:"version"`
	Runs    []sarifOutRun `json:"runs"`
}

type sarifOutRun struct {
	Tool       sarifTool         `json:"tool"`
	Results    []sarifOutResult  `json:"results"`
	Properties map[string]any    `json:"properties,omitempty"`
	Invocation []sarifInvocation `json:"invocations,omitempty"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool `json:"executionSuccessful"`
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

type sarifOutText struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown,omitempty"`
}

type sarifRule struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	ShortDescription     sarifOutText    `json:"shortDescription"`
	FullDescription      sarifOutText    `json:"fullDescription"`
	Help                 sarifOutText    `json:"help"`
	HelpURI              string          `json:"helpUri"`
	DefaultConfiguration sarifRuleConfig `json:"defaultConfiguration"`
	Properties           map[string]any  `json:"properties"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifOutResult struct {
	RuleID              string             `json:"ruleId"`
	RuleIndex           int                `json:"ruleIndex"`
	Level               string             `json:"level"`
	Message             sarifOutText       `json:"message"`
	Locations           []sarifLocation    `json:"locations"`
	PartialFingerprints map[string]string  `json:"partialFingerprints,omitempty"`
	Suppressions        []sarifSuppression `json:"suppressions,omitempty"`
	BaselineState       string             `json:"baselineState,omitempty"`
	Properties          map[string]any     `json:"properties,omitempty"`
}

type sarifSuppression struct {
	Kind          string `json:"kind"`
	Status        string `json:"status,omitempty"`
	Justification string `json:"justification,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// sarifLevel maps a severity to a SARIF result level.
func sarifLevel(s Severity) string { return sarifout.Level(string(s)) }

// securitySeverity is the CVSS-like score GitHub code scanning reads from the
// security-severity property to rank an alert (>=7 high, 4-6.9 medium, <4 low).
func securitySeverity(s Severity) string {
	switch s {
	case SeverityError:
		return "8.0"
	case SeverityWarning:
		return "5.0"
	default:
		return "2.0"
	}
}

// isSecurityCode reports whether code belongs to the AR0xx security family.
func isSecurityCode(code string) bool {
	return len(code) == 5 && strings.HasPrefix(code, "AR0")
}

func sarifRuleFor(code string, def Severity) sarifRule {
	info, known := lookupRule(code)
	e, _ := Explain(code) //nolint:errcheck // unknown codes (external) fall through below
	if !known {
		info = RuleInfo{Code: code, Name: strings.ToLower(code), Default: def, Describe: code}
		e = Explanation{Summary: code, Anchor: "", DocsURL: docsBase}
	}
	help := e.Why
	md := ""
	if known {
		help = e.Summary + ".\n" + e.Why + "\nBad: " + e.Bad + "\nGood: " + e.Good
		md = "**" + e.Summary + "**\n\n" + e.Why + "\n\n- Bad: " + e.Bad + "\n- Good: " + e.Good + "\n\n[Documentation](" + e.DocsURL + ")"
	}
	if help == "" {
		help = info.Describe
	}
	props := map[string]any{"tags": ruleTags(code), "analyzer": AnalyzerFor(code).Name, "scope": AnalyzerFor(code).Scope}
	if isSecurityCode(code) {
		props["security-severity"] = securitySeverity(info.Default)
	}
	return sarifRule{
		ID: info.Code, Name: info.Name,
		ShortDescription:     sarifOutText{Text: info.Name},
		FullDescription:      sarifOutText{Text: info.Describe},
		Help:                 sarifOutText{Text: help, Markdown: md},
		HelpURI:              e.DocsURL,
		DefaultConfiguration: sarifRuleConfig{Level: sarifLevel(info.Default)},
		Properties:           props,
	}
}

// ruleTags labels a rule for code scanning filters.
func ruleTags(code string) []string {
	family := "quality"
	if isSecurityCode(code) {
		family = AnalyzerSecurity
	}
	tags := []string{family, "ai-instructions"}
	if name := AnalyzerFor(code).Name; name != family { // SARIF requires unique tags
		tags = append(tags, name)
	}
	return tags
}

// buildSARIF converts a combined report to a SARIF log. version is the ai-rulez
// release; it may be empty.
func buildSARIF(c Combined, version string) sarifDocument {
	codes := map[string]Severity{}
	for _, f := range c.Findings {
		if _, ok := codes[f.Code]; !ok {
			codes[f.Code] = f.Severity
		}
	}
	ordered := make([]string, 0, len(codes))
	for code := range codes {
		ordered = append(ordered, code)
	}
	sort.Strings(ordered)
	index := map[string]int{}
	rules := make([]sarifRule, 0, len(ordered))
	for i, code := range ordered {
		index[code] = i
		rules = append(rules, sarifRuleFor(code, codes[code]))
	}
	results := make([]sarifOutResult, 0, len(c.Findings))
	for i := range c.Findings {
		results = append(results, sarifResultFor(&c.Findings[i], index[c.Findings[i].Code], c.Baseline != nil))
	}
	run := sarifOutRun{
		Tool: sarifTool{Driver: sarifDriver{
			Name: toolName, Version: version, InformationURI: toolInfoURI, Rules: rules,
		}},
		Results:    results,
		Invocation: []sarifInvocation{{ExecutionSuccessful: true}},
	}
	if props := runProperties(c); len(props) > 0 {
		run.Properties = props
	}
	return sarifDocument{Schema: sarifSchema, Version: sarifVersion, Runs: []sarifOutRun{run}}
}

func sarifResultFor(f *Finding, ruleIndex int, baselined bool) sarifOutResult {
	path := f.RepoPath()
	uri, base := sarifout.ArtifactURI(path)
	res := sarifOutResult{
		RuleID: f.Code, RuleIndex: ruleIndex, Level: sarifLevel(f.Severity),
		Message: sarifOutText{Text: f.Message},
		Locations: []sarifLocation{{PhysicalLocation: sarifPhysical{
			ArtifactLocation: sarifArtifact{URI: uri, URIBaseID: base},
			Region:           sarifRegion{StartLine: max(f.Line, 1)},
		}}},
	}
	if fp := f.Fingerprint(); fp != "" {
		res.PartialFingerprints = map[string]string{sarifFingerprint: fp}
	}
	if f.IsAccepted() {
		res.BaselineState = "unchanged"
		res.Suppressions = []sarifSuppression{{Kind: "external", Status: "accepted", Justification: f.Meta.AcceptReason}}
	} else if baselined {
		res.BaselineState = "new"
	}
	props := resultProperties(f)
	if isSecurityCode(f.Code) {
		props["security-severity"] = securitySeverity(f.Severity)
	}
	if len(props) > 0 {
		res.Properties = props
	}
	return res
}

// WriteSARIF prints the combined report as SARIF 2.1.0.
func WriteSARIF(w io.Writer, c Combined, version string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(buildSARIF(c, version)) //nolint:wrapcheck // writer error
}
