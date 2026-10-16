// Package judgebench is the evaluation of `ai-rulez judge`: a synthetic corpus of agent configuration
// with known defects, benign lookalikes, the false-positive traps of the adjudicator and a fixture
// repository of written conventions, and the code that scores a judge report against the labels.
//
// Every file under corpus/ is synthetic: fake hosts, fake credentials, no customer data. Its
// malicious samples exist to be flagged and are never executed.
//
// The offline half (the tests in this package) runs the verifier and the convention attachment on the
// corpus without a model. The live half is cmd/judgebench (task bench:judge), which runs the model
// pipeline over the corpus and prints precision, recall, the trap false-positive rate, grounding and
// cost.
package judgebench

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The groups of a corpus unit.
const (
	GroupMalicious = "malicious"
	GroupBenign    = "benign"
	GroupTrap      = "trap"
	GroupTwin      = "twin"
	GroupQuality   = "quality"
	GroupQBenign   = "qbenign"
	GroupClean     = "clean"
	GroupGovern    = "governance"
	GroupGBenign   = "gbenign"
)

// Expect is a finding a correct review reports.
//
//nolint:tagliatelle // the label files are snake_case
type Expect struct {
	Code        string `yaml:"code"`
	Path        string `yaml:"path"`
	Contains    string `yaml:"contains"`
	MinSeverity string `yaml:"min_severity"`
}

// Unit is one labeled unit of the corpus.
//
//nolint:tagliatelle // the label files are snake_case
type Unit struct {
	ID              string   `yaml:"id"`
	Dir             string   `yaml:"dir"`
	JudgePath       string   `yaml:"judge_path"`
	Group           string   `yaml:"group"`
	Trap            string   `yaml:"trap"`
	TwinOf          string   `yaml:"twin_of"`
	Expect          []Expect `yaml:"expect"`
	ExpectStatus    string   `yaml:"expect_status"`
	CleanCategories []string `yaml:"clean_categories"`
	RubricSkill     string   `yaml:"rubric_skill"`
	Attach          []string `yaml:"attach"`
	AttachReference bool     `yaml:"attach_referenced"`
	Note            string   `yaml:"note"`
}

// Path is the path the judge is asked to review for the unit.
func (u Unit) Path() string {
	if u.JudgePath != "" {
		return u.JudgePath
	}
	return u.Dir
}

// Manifest is every labeled unit of the corpus.
type Manifest struct {
	Units []Unit
	// Root is bench/judge.
	Root string
}

// CorpusDir is the directory the judge is run in (after being copied to a temporary git repository).
func (m Manifest) CorpusDir() string { return filepath.Join(m.Root, "corpus") }

// Load reads manifest/*.yaml below root (the bench/judge directory) and checks the labels are well
// formed: unique ids, existing directories and paths, and an expected quote that is on one line of
// the file it names.
func Load(root string) (Manifest, error) {
	files, err := filepath.Glob(filepath.Join(root, "manifest", "*.yaml"))
	if err != nil || len(files) == 0 {
		return Manifest{}, fmt.Errorf("no manifest files below %s", root)
	}
	sort.Strings(files)
	m := Manifest{Root: root}
	seen := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // the repository's own corpus
		if err != nil {
			return m, fmt.Errorf("read %s: %w", f, err)
		}
		var doc struct {
			Units []Unit `yaml:"units"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return m, fmt.Errorf("parse %s: %w", f, err)
		}
		for i := range doc.Units {
			u := doc.Units[i]
			if seen[u.ID] {
				return m, fmt.Errorf("unit id %q is used twice", u.ID)
			}
			seen[u.ID] = true
			m.Units = append(m.Units, u)
		}
	}
	return m, m.check()
}

func (m Manifest) check() error {
	var problems []string
	corpus := m.CorpusDir()
	for i := range m.Units {
		u := &m.Units[i]
		if _, err := os.Stat(filepath.Join(corpus, filepath.FromSlash(u.Path()))); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %s does not exist", u.ID, u.Path()))
		}
		for _, e := range u.Expect {
			data, err := os.ReadFile(filepath.Join(corpus, filepath.FromSlash(e.Path))) //nolint:gosec // the repository's own corpus
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: expected file %s does not exist", u.ID, e.Path))
				continue
			}
			if e.Contains != "" && !lineContains(string(data), e.Contains) {
				problems = append(problems, fmt.Sprintf("%s: %q is not on one line of %s", u.ID, e.Contains, e.Path))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problems in the corpus labels:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

func lineContains(text, want string) bool {
	want = strings.ToLower(strings.Join(strings.Fields(want), " "))
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(strings.ToLower(strings.Join(strings.Fields(line), " ")), want) {
			return true
		}
	}
	return false
}

// ByGroup returns the units of the groups, in manifest order.
func (m Manifest) ByGroup(groups ...string) []Unit {
	var out []Unit
	for i := range m.Units {
		for _, g := range groups {
			if m.Units[i].Group == g {
				out = append(out, m.Units[i])
				break
			}
		}
	}
	return out
}
