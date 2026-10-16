package judgebench

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/judge"
)

// corpus is the corpus as the judge sees a project: every file, with a reader.
type corpus struct {
	root  string
	files []string
}

func loadCorpus(t *testing.T, m Manifest) corpus {
	t.Helper()
	c := corpus{root: m.CorpusDir()}
	require.NoError(t, filepath.WalkDir(c.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(c.root, p)
		c.files = append(c.files, filepath.ToSlash(rel))
		return nil
	}))
	sort.Strings(c.files)
	return c
}

func (c corpus) read(rel string) judge.SourceFile {
	data, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(rel))) //nolint:gosec // the repository's own corpus
	if err != nil {
		return judge.SourceFile{Path: rel, Skip: judge.SkipUnreadable, SkipReason: err.Error()}
	}
	return judge.SourceFile{Path: rel, Data: data, Bytes: len(data)}
}

func (c corpus) repo() judge.Repo {
	return judge.Repo{Files: c.files, Read: func(rel string) (string, bool) {
		sf := c.read(rel)
		return string(sf.Data), sf.Skip == ""
	}}
}

// unit builds the judge unit that holds path (a skill directory, or the file itself).
func (c corpus) unit(t *testing.T, path string) judge.Unit {
	t.Helper()
	for _, ref := range judge.Discover(c.files, true) {
		if ref.Path == path {
			return judge.BuildUnit(ref, c.read, judge.UnitOptions{})
		}
	}
	for _, ref := range judge.Discover(c.files, true) {
		for _, member := range ref.Members {
			if member == path {
				return judge.BuildUnit(ref, c.read, judge.UnitOptions{})
			}
		}
	}
	for _, ref := range judge.Discover(c.files, true) {
		if strings.HasPrefix(ref.Path, path+"/") {
			return judge.BuildUnit(ref, c.read, judge.UnitOptions{})
		}
	}
	t.Fatalf("no judge unit holds %s", path)
	return judge.Unit{}
}

func loadManifest(t *testing.T) Manifest {
	t.Helper()
	m, err := Load(".")
	require.NoError(t, err)
	return m
}

func TestTheCorpusCoversEverySecurityCodeAndTrap(t *testing.T) {
	// Arrange
	m := loadManifest(t)

	// Act
	malicious, benign := map[string]int{}, map[string]int{}
	for _, u := range m.Units {
		for _, e := range u.Expect {
			if u.Group == GroupMalicious {
				malicious[e.Code]++
			}
		}
		if u.Group == GroupBenign {
			benign[strings.TrimPrefix(strings.ToUpper(u.ID), "BEN-")[:3]]++
		}
	}
	traps, twins := map[string]bool{}, map[string]bool{}
	for _, u := range m.Units {
		switch u.Group {
		case GroupTrap:
			traps[u.Trap] = true
		case GroupTwin:
			twins[u.Trap] = true
		}
	}

	// Assert
	for _, code := range judge.CodesOf(judge.CategorySecurity) {
		assert.GreaterOrEqual(t, malicious[code], 1, "%s needs a malicious unit", code)
		assert.GreaterOrEqual(t, benign[strings.ReplaceAll(strings.TrimPrefix(code, "JR-"), "-", "")], 1, "%s needs a benign look-alike", code)
	}
	for i := 1; i <= 14; i++ {
		id := "T" + pad2(i)
		assert.True(t, traps[id], "trap %s needs a unit", id)
		assert.True(t, twins[id], "trap %s needs a malicious twin", id)
	}
	for _, code := range judge.CodesOf(judge.CategoryQuality) {
		found := false
		for _, u := range m.ByGroup(GroupQuality) {
			for _, e := range u.Expect {
				found = found || e.Code == code
			}
		}
		assert.True(t, found, "%s needs a defect unit", code)
	}
	assert.GreaterOrEqual(t, len(m.ByGroup(GroupGovern)), 14)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func TestEveryUnitIsFoundByDefaultDiscoveryOrByName(t *testing.T) {
	// Arrange
	m := loadManifest(t)
	c := loadCorpus(t, m)
	conventional := map[string]bool{}
	for _, ref := range judge.Discover(c.files, false) {
		conventional[ref.Path] = true
	}

	// Act and Assert
	for _, u := range m.Units {
		t.Run(u.ID, func(t *testing.T) {
			unit := c.unit(t, u.Path())
			assert.NotEmpty(t, unit.Files)
			found := false
			for p := range conventional {
				found = found || p == u.Path() || strings.HasPrefix(p, u.Path()+"/")
			}
			assert.True(t, found, "default discovery finds a unit below %s (nested .claude directories at any depth)", u.Path())
		})
	}
}

// quoteAt finds the first line of the unit's file that holds want (case-insensitively) and returns the
// line number and the text of the line as the reviewer would copy it.
func quoteAt(u judge.Unit, path, want string) (line int, quote string, ok bool) {
	for _, f := range u.Files {
		if f.Path != path {
			continue
		}
		low := strings.ToLower(strings.Join(strings.Fields(want), " "))
		for i, l := range f.Lines {
			norm := strings.Join(strings.Fields(l), " ")
			if at := strings.Index(strings.ToLower(norm), low); at >= 0 {
				return i + 1, norm[at : at+len(low)], true
			}
		}
	}
	return 0, "", false
}

func TestTheVerifierGroundsEveryExpectedFindingAndRejectsAnInventedQuote(t *testing.T) {
	// Arrange
	m := loadManifest(t)
	c := loadCorpus(t, m)

	// Act and Assert
	for _, u := range m.Units {
		for _, e := range u.Expect {
			if u.ExpectStatus != "" || e.Contains == "" {
				continue
			}
			t.Run(u.ID+"/"+e.Code, func(t *testing.T) {
				unit := c.unit(t, u.Path())
				line, quote, ok := quoteAt(unit, e.Path, e.Contains)
				require.True(t, ok, "%q is in %s", e.Contains, e.Path)
				cat, _ := judge.CategoryOfCode(e.Code)
				cand := judge.Candidate{
					ID: "X1", Code: e.Code, Verdict: judge.VerdictConfirmed, Severity: judge.SeverityLow, Confidence: judge.ConfidenceMedium,
					Title: "t", Location: judge.Location{Path: e.Path, StartLine: line, EndLine: line}, Quote: quote,
				}
				if cat == judge.CategoryGovernance && e.Code != "JR-G06" {
					return // governance findings also need a convention quote; the attachment test covers them
				}
				ver := judge.NewVerifier(unit, cat, nil, nil, []string{"rubric:acme-skill-standards/C1"})
				if e.Code == "JR-G06" {
					id := "rubric:acme-skill-standards/C1"
					cand.RubricID = &id
				}

				good, probs := ver.Check(cand)
				cand.Quote = "invented " + quote
				_, bad := ver.Check(cand)

				assert.Empty(t, probs)
				assert.Equal(t, quote, good.Candidate.Quote)
				assert.NotEmpty(t, bad, "a quote that is not in the file is rejected")
			})
		}
	}
}

func TestTheAttachmentAlgorithmMatchesTheLabelsOfTheGovernanceRepository(t *testing.T) {
	// Arrange
	m := loadManifest(t)
	c := loadCorpus(t, m)
	checked := 0

	// Act and Assert
	for _, u := range m.Units {
		if len(u.Attach) == 0 {
			continue
		}
		checked++
		t.Run(u.ID, func(t *testing.T) {
			unit := c.unit(t, u.Path())
			att := judge.Attach(c.repo(), unit, judge.AttachOptions{Rules: true, ClaudeMD: true})
			var got []string
			for _, conv := range att.Conventions {
				if conv.Why != judge.WhyOverlap {
					got = append(got, conv.ID)
				}
			}
			assert.ElementsMatch(t, u.Attach, got, "attached to %s", u.Path())
		})
	}
	assert.GreaterOrEqual(t, checked, 21, "the governance units carry labels")
}

func TestARulePathMentionedInAUnitAttachesTheRuleTaggedAsReferenced(t *testing.T) {
	m := loadManifest(t)
	c := loadCorpus(t, m)
	for _, u := range m.Units {
		if !u.AttachReference {
			continue
		}
		att := judge.Attach(c.repo(), c.unit(t, u.Path()), judge.AttachOptions{Rules: true, ClaudeMD: true})
		referenced := 0
		for _, conv := range att.Conventions {
			if conv.Why == judge.WhyMatchesReference {
				referenced++
			}
		}
		assert.Positive(t, referenced, "%s mentions a path that a rule is scoped to", u.ID)
	}
}

func TestSecretUnitsAreWithheldAndRedactedOnRequest(t *testing.T) {
	m := loadManifest(t)
	c := loadCorpus(t, m)
	n := 0
	for _, u := range m.Units {
		if u.ExpectStatus == "" {
			continue
		}
		n++
		var ref judge.Ref
		for _, r := range judge.Discover(c.files, true) {
			if strings.HasPrefix(r.Path, u.Dir+"/") || r.Path == u.Dir {
				ref = r
			}
		}
		require.NotEmpty(t, ref.Path, u.ID)

		withheld := judge.BuildUnit(ref, c.read, judge.UnitOptions{})
		redacted := judge.BuildUnit(ref, c.read, judge.UnitOptions{Redact: true})

		assert.Equal(t, judge.StatusWithheld, withheld.Status, u.ID)
		assert.False(t, withheld.Sent(), u.ID)
		assert.Empty(t, redacted.Status, u.ID)
	}
	assert.GreaterOrEqual(t, n, 3)
}

func TestScoringAPerfectAndAnEmptyReport(t *testing.T) {
	// Arrange
	m := loadManifest(t)
	perfect, empty := &Report{}, &Report{}
	for _, u := range m.Units {
		ru := ReportUnit{Path: u.Path(), Status: "pass", Complete: true}
		if u.ExpectStatus != "" {
			ru.Status = u.ExpectStatus
		}
		for _, e := range u.Expect {
			if u.ExpectStatus != "" {
				continue
			}
			var f Finding
			f.Code, f.Category, f.Severity = e.Code, category(e.Code), "critical"
			f.Location.Path, f.Quote = e.Path, e.Contains
			ru.Findings = append(ru.Findings, f)
		}
		perfect.Units = append(perfect.Units, ru)
		empty.Units = append(empty.Units, ReportUnit{Path: u.Path(), Status: "pass", Complete: true})
	}

	// Act
	p, e := Score(m, perfect, nil), Score(m, empty, nil)

	// Assert
	assert.InDelta(t, 1.0, p.ByCategory["security"].Recall(), 1e-9)
	assert.InDelta(t, 1.0, p.ByCategory["security"].Precision(), 1e-9)
	assert.InDelta(t, 1.0, p.ByCategory["quality"].F1(), 1e-9)
	assert.Zero(t, p.TrapFalsePositives)
	assert.Equal(t, p.TwinUnits, p.TwinsFound)
	assert.Zero(t, e.ByCategory["security"].Recall())
	assert.Zero(t, e.TwinsFound)
	assert.Equal(t, 1.0, e.ByCategory["security"].Precision(), "nothing reported, nothing wrong")
	assert.Empty(t, p.Missing)
}

func TestScoringCountsAFindingOnATrapUnitAsAFalsePositive(t *testing.T) {
	m := loadManifest(t)
	rep := &Report{}
	for _, u := range m.Units {
		ru := ReportUnit{Path: u.Path(), Status: "pass", Complete: true}
		if u.Group == GroupTrap {
			f := Finding{Code: "JR-S05", Category: "security", Severity: "high", Quote: "x"}
			f.Location.Path = "x"
			ru.Findings = []Finding{f}
		}
		rep.Units = append(rep.Units, ru)
	}

	mt := Score(m, rep, nil)

	assert.Equal(t, mt.TrapUnits, mt.TrapFalsePositives)
	assert.Equal(t, mt.TrapUnits, mt.ByCode["JR-S05"].FP)
	assert.Greater(t, mt.FalsePositivesPer100, 0.0)
}
