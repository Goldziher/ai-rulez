package lint

import (
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Output formats of `validate --strict`.
const (
	FormatText     = "text"
	FormatJSON     = "json"
	FormatSARIF    = "sarif"
	FormatGitHub   = "github"
	FormatJUnit    = "junit"
	FormatMarkdown = "markdown"
)

// Formats lists the accepted --format values.
func Formats() []string {
	return []string{FormatText, FormatJSON, FormatSARIF, FormatGitHub, FormatJUnit, FormatMarkdown}
}

// IsFormat reports whether name is an accepted format ("" means text).
func IsFormat(name string) bool {
	if name == "" {
		return true
	}
	for _, f := range Formats() {
		if f == name {
			return true
		}
	}
	return false
}

// WriteOptions carries what a formatter needs beyond the findings.
type WriteOptions struct {
	// Version is the ai-rulez release, recorded as the SARIF tool version.
	Version string
	// FailOn is the threshold JUnit uses to tell a failure from a skipped note.
	FailOn string
}

// Write prints c in the named format.
func Write(w io.Writer, format string, c Combined, o WriteOptions) error {
	switch format {
	case "", FormatText:
		return WriteText(w, c)
	case FormatJSON:
		return WriteJSON(w, c)
	case FormatSARIF:
		return WriteSARIF(w, c, o.Version)
	case FormatGitHub:
		return WriteGitHub(w, c)
	case FormatJUnit:
		return WriteJUnit(w, c, o.FailOn)
	case FormatMarkdown:
		return WriteMarkdown(w, c)
	}
	return fmt.Errorf("unknown format %q", format)
}

// runProperties and resultProperties are the advisory extras (risk, baseline
// state) that structured formats attach next to the findings.
func runProperties(c Combined) map[string]any {
	props := map[string]any{}
	if c.Risk != nil {
		props["risk"] = c.Risk
	}
	return props
}

func resultProperties(f *Finding) map[string]any {
	props := map[string]any{}
	if f.Meta != nil && f.Meta.Analyzer != "" {
		props["analyzer"], props["scope"] = f.Meta.Analyzer, f.Meta.Scope
	}
	return props
}

// ghEscape escapes a GitHub workflow command value; property values also escape
// the characters that delimit properties.
func ghEscape(s string, property bool) string {
	s = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
	if property {
		s = strings.NewReplacer(":", "%3A", ",", "%2C").Replace(s)
	}
	return s
}

func ghLevel(s Severity) string {
	switch s {
	case SeverityError:
		return string(SeverityError)
	case SeverityWarning:
		return string(SeverityWarning)
	default:
		return "notice"
	}
}

// WriteGitHub prints one GitHub Actions workflow-command annotation per finding.
func WriteGitHub(w io.Writer, c Combined) error {
	var sb strings.Builder
	for i := range c.Findings {
		f := &c.Findings[i]
		if f.IsAccepted() {
			continue
		}
		path := f.RepoPath()
		fmt.Fprintf(&sb, "::%s file=%s,line=%d,title=%s::%s\n",
			ghLevel(f.Severity), ghEscape(path, true), max(f.Line, 1), ghEscape(f.Code+" "+f.Name, true), ghEscape(f.Message, false))
	}
	_, err := io.WriteString(w, sb.String())
	return err //nolint:wrapcheck // writer error
}

type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Skipped  int          `xml:"skipped,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	File      string        `xml:"file,attr,omitempty"`
	Line      int           `xml:"line,attr,omitempty"`
	Failure   *junitProblem `xml:"failure,omitempty"`
	Skipped   *junitProblem `xml:"skipped,omitempty"`
}

type junitProblem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr,omitempty"`
	Body    string `xml:",chardata"`
}

// WriteJUnit prints one testsuite per root and one testcase per finding. A
// finding at or above failOn is a failure; a lower one, or one accepted by the
// baseline, is reported as skipped so it stays visible without failing the job.
func WriteJUnit(w io.Writer, c Combined, failOn string) error {
	threshold, ok := ParseSeverity(failOn)
	if !ok || threshold == SeverityOff {
		threshold = SeverityError
	}
	byRoot := map[string]*junitSuite{}
	var order []string
	for _, r := range c.Roots {
		if _, dup := byRoot[r]; !dup {
			byRoot[r] = &junitSuite{Name: r}
			order = append(order, r)
		}
	}
	doc := junitSuites{Name: "ai-rulez validate"}
	for i := range c.Findings {
		f := &c.Findings[i]
		root := f.Root
		if _, exists := byRoot[root]; !exists {
			byRoot[root] = &junitSuite{Name: root}
			order = append(order, root)
		}
		suite := byRoot[root]
		path := f.RepoPath()
		tc := junitCase{Name: fmt.Sprintf("%s %s (line %d)", f.Code, f.Name, f.Line), Classname: path, File: path, Line: f.Line}
		problem := &junitProblem{Message: f.Message, Type: f.Code, Body: fmt.Sprintf("%s:%d: %s %s: %s", path, f.Line, f.Severity, f.Code, f.Message)}
		switch {
		case f.IsAccepted():
			problem.Message = "accepted by baseline: " + f.Message
			tc.Skipped = problem
			suite.Skipped++
		case f.Severity.AtLeast(threshold):
			tc.Failure = problem
			suite.Failures++
		default:
			problem.Message = string(f.Severity) + ": " + f.Message
			tc.Skipped = problem
			suite.Skipped++
		}
		suite.Tests++
		suite.Cases = append(suite.Cases, tc)
	}
	for _, r := range order {
		s := byRoot[r]
		doc.Tests += s.Tests
		doc.Failures += s.Failures
		doc.Skipped += s.Skipped
		doc.Suites = append(doc.Suites, *s)
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err //nolint:wrapcheck // writer error
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err //nolint:wrapcheck // writer error
	}
	_, err := io.WriteString(w, "\n")
	return err //nolint:wrapcheck // writer error
}

func mdEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "|", `\|`, "\n", " ", "\r", " ", "<", "&lt;", ">", "&gt;",
		"[", `\[`, "]", `\]`, "`", "'").Replace(s)
}

// WriteMarkdown prints a summary meant for a pull-request comment, grouped by
// severity.
func WriteMarkdown(w io.Writer, c Combined) error {
	var sb strings.Builder
	sb.WriteString("## ai-rulez validate\n\n")
	if c.Summary.Total == 0 {
		fmt.Fprintf(&sb, "No findings in %d root(s).\n", len(c.Roots))
		writeBaselineMarkdown(&sb, c)
		_, err := io.WriteString(w, sb.String())
		return err //nolint:wrapcheck // writer error
	}
	fmt.Fprintf(&sb, "**%d error(s), %d warning(s), %d info** across %d root(s).\n\n",
		c.Summary.Errors, c.Summary.Warnings, c.Summary.Infos, len(c.Roots))
	for _, sev := range []Severity{SeverityError, SeverityWarning, SeverityInfo} {
		var group []Finding
		for i := range c.Findings {
			if c.Findings[i].Severity == sev && !c.Findings[i].IsAccepted() {
				group = append(group, c.Findings[i])
			}
		}
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "### %s (%d)\n\n| Rule | Location | Message |\n| --- | --- | --- |\n", strings.ToUpper(string(sev[:1]))+string(sev[1:])+"s", len(group))
		for _, f := range group {
			path := f.RepoPath()
			fmt.Fprintf(&sb, "| `%s` %s | `%s:%d` | %s |\n", f.Code, f.Name, strings.ReplaceAll(path, "`", "'"), f.Line, mdEscape(f.Message))
		}
		sb.WriteString("\n")
	}
	codes := make([]string, 0, len(c.Summary.ByCode))
	for code := range c.Summary.ByCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	sb.WriteString("<details><summary>By rule</summary>\n\n| Rule | Count |\n| --- | --- |\n")
	for _, code := range codes {
		rule, _ := lookupRule(code) //nolint:errcheck // codes come from findings
		fmt.Fprintf(&sb, "| `%s` %s | %d |\n", code, rule.Name, c.Summary.ByCode[code])
	}
	sb.WriteString("\n</details>\n")
	writeRiskMarkdown(&sb, c)
	writeBaselineMarkdown(&sb, c)
	_, err := io.WriteString(w, sb.String())
	return err //nolint:wrapcheck // writer error
}

func writeBaselineMarkdown(sb *strings.Builder, c Combined) {
	for _, e := range c.Ratchet {
		fmt.Fprintf(sb, "\n**Over ratchet:** `%s` has %d finding(s), ratchet %d.\n", e.Code, e.Count, e.Max)
	}
	if c.Baseline == nil {
		return
	}
	fmt.Fprintf(sb, "\nBaseline: %d accepted", c.Baseline.Accepted)
	if n := len(c.Baseline.Stale); n > 0 {
		fmt.Fprintf(sb, ", %d stale", n)
	}
	if n := len(c.Baseline.Expired); n > 0 {
		fmt.Fprintf(sb, ", %d expired", n)
	}
	sb.WriteString(".\n")
}
