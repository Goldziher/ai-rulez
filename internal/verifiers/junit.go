package verifiers

import (
	"encoding/xml"
	"io"
	"sort"

	"github.com/samber/oops"
)

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Name    string       `xml:"name,attr"`
	Tests   int          `xml:"tests,attr"`
	Failed  int          `xml:"failures,attr"`
	Errors  int          `xml:"errors,attr"`
	Skipped int          `xml:"skipped,attr"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name    string      `xml:"name,attr"`
	Tests   int         `xml:"tests,attr"`
	Failed  int         `xml:"failures,attr"`
	Errors  int         `xml:"errors,attr"`
	Skipped int         `xml:"skipped,attr"`
	Cases   []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Failure   *junitProblem `xml:"failure,omitempty"`
	Error     *junitProblem `xml:"error,omitempty"`
	Skipped   *struct{}     `xml:"skipped,omitempty"`
	// SystemOut carries a finding that is below the failing severity: the case
	// passes, the text stays visible.
	SystemOut string `xml:"system-out,omitempty"`
}

type junitProblem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

// WriteJUnit writes the report as JUnit XML: one suite per enforced rule (or
// skill, agent, command), one case per verifier and, for a failure, per subject.
// A failure below the failOn severity ("error", "warning", "info" or "none") is a
// passing case with its text in system-out, matching the exit code.
func WriteJUnit(w io.Writer, r *Report, failOn string) error {
	suites := map[string]*junitSuite{}
	var order []string
	suiteFor := func(res Result) *junitSuite {
		name := "config"
		if res.Target != nil {
			name = res.Target.Kind + ":" + res.Target.ID
		}
		if s, ok := suites[name]; ok {
			return s
		}
		suites[name] = &junitSuite{Name: sanitize(name)}
		order = append(order, name)
		return suites[name]
	}
	for _, res := range r.Results {
		s := suiteFor(res)
		s.Cases = append(s.Cases, casesFor(res, failOn)...)
	}
	sort.Strings(order)
	doc := junitSuites{Name: "ai-rulez verifiers"}
	for _, name := range order {
		s := suites[name]
		for _, c := range s.Cases {
			s.Tests++
			switch {
			case c.Failure != nil:
				s.Failed++
			case c.Error != nil:
				s.Errors++
			case c.Skipped != nil:
				s.Skipped++
			}
		}
		doc.Tests, doc.Failed, doc.Errors, doc.Skipped = doc.Tests+s.Tests, doc.Failed+s.Failed, doc.Errors+s.Errors, doc.Skipped+s.Skipped
		doc.Suites = append(doc.Suites, *s)
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return oops.Wrapf(err, "write verifiers JUnit")
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return oops.Wrapf(err, "write verifiers JUnit")
	}
	_, err := io.WriteString(w, "\n")
	return wrapWrite(err)
}

func casesFor(res Result, failOn string) []junitCase {
	class := sanitize(res.Name)
	switch res.Status {
	case StatusPass:
		return []junitCase{{Name: class, ClassName: class}}
	case StatusNotApplicable:
		return []junitCase{{Name: class, ClassName: class, Skipped: &struct{}{}}}
	case StatusError:
		return []junitCase{{Name: class, ClassName: class, Error: &junitProblem{Message: res.Message, Type: nonEmpty(res.Code, "error"), Text: res.Message}}}
	}
	findings := res.Findings
	if len(findings) == 0 {
		findings = []Finding{{Message: res.Message}}
	}
	counts := severityRank(res.Severity) >= severityRank(failOn) && failOn != "none" && severityRank(failOn) > 0
	cases := make([]junitCase, 0, len(findings))
	for _, f := range findings {
		name := class
		if f.File != "" {
			name = class + " " + sanitize(f.File)
		}
		text := findingLine(f)
		if res.Fix != "" {
			text += "\nfix: " + sanitize(res.Fix)
		}
		if !counts {
			cases = append(cases, junitCase{Name: name, ClassName: class, SystemOut: res.Severity + ": " + text})
			continue
		}
		cases = append(cases, junitCase{Name: name, ClassName: class,
			Failure: &junitProblem{Message: sanitize(f.Message), Type: res.Code, Text: text}})
	}
	return cases
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func severityRank(s string) int {
	switch s {
	case "info":
		return 1
	case severityWarning:
		return 2
	case severityError:
		return 3
	}
	return 0
}
