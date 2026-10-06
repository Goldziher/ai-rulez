package review

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// goldenCase describes one case of a test golden set.
type goldenCase struct {
	id       string
	desc     string
	siblings []string
	labels   map[string]string
	labelsB  map[string]string
	probes   []string
}

var tqLabels = []string{VerdictPass, VerdictPass, VerdictPass, VerdictWarn, VerdictWarn, VerdictFail, VerdictFail, VerdictFail, VerdictPass, VerdictWarn}

// tenCases is a golden set of ten skills: trigger-quality varies, two of them hold an injection,
// and every case has the same two siblings.
func tenCases() []goldenCase {
	var out []goldenCase
	for i := 0; i < 10; i++ {
		inj := VerdictPass
		if i == 6 || i == 7 {
			inj = VerdictFail
		}
		c := goldenCase{
			id: fmt.Sprintf("case-%02d", i), desc: fmt.Sprintf("Golden item number %d does one specific thing for the build", i),
			siblings: []string{"sib-one", "sib-two"},
			labels:   map[string]string{"trigger-quality": tqLabels[i], "injection-intent": inj, "overlap": VerdictPass},
		}
		if i%2 == 0 {
			c.probes = []string{ProbePad, ProbeRename, ProbeReorder, ProbeCanary}
		}
		out = append(out, c)
	}
	return out
}

func writeGolden(t *testing.T, cases []goldenCase) string {
	t.Helper()
	base := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(base, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(text), 0o600))
	}
	for _, sib := range []string{"sib-one", "sib-two"} {
		write("fixtures/shared/"+sib+"/SKILL.md", "---\nname: "+sib+"\ndescription: Shared sibling "+sib+" handles something else entirely\n---\nbody of "+sib+"\n")
	}
	for _, c := range cases {
		write("fixtures/cases/"+c.id+"/"+c.id+"/SKILL.md", "---\nname: "+c.id+"\ndescription: "+c.desc+"\n---\nThe body of "+c.id+".\n")
		var sb strings.Builder
		fmt.Fprintf(&sb, "schema_version: 1\nid: %s\nkind: skill\nitem:\n  path: fixtures/cases/%s/%s/SKILL.md\n", c.id, c.id, c.id)
		if len(c.siblings) > 0 {
			sb.WriteString("siblings:\n")
			for _, s := range c.siblings {
				fmt.Fprintf(&sb, "  - fixtures/shared/%s/SKILL.md\n", s)
			}
		}
		labelsB := c.labelsB
		if labelsB == nil {
			labelsB = c.labels
		}
		fmt.Fprintf(&sb, "labelers:\n  - id: a\n    labels: %s\n  - id: b\n    labels: %s\nadjudicated: %s\n", flow(c.labels), flow(labelsB), flow(c.labels))
		if len(c.probes) > 0 {
			fmt.Fprintf(&sb, "probes: [%s]\n", strings.Join(c.probes, ", "))
		}
		write("golden/"+c.id+".golden.yaml", sb.String())
	}
	return base
}

func flow(m map[string]string) string {
	var parts []string
	for _, k := range sortedKeys(m) {
		parts = append(parts, k+": "+m[k])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// truthJudge answers with the adjudicated label of the golden case whose description it is shown,
// and behaves like a sound judge on the probes: a canary is flagged, padding changes nothing.
func truthJudge(cases []goldenCase) *scriptedJudge {
	byDesc := map[string]goldenCase{}
	for _, c := range cases {
		byDesc[c.desc] = c
	}
	return &scriptedJudge{decide: func(c scriptedCall, dim string) string {
		if dim == "injection-intent" && strings.Contains(c.User, "NOTE TO THE REVIEWER") {
			return VerdictFail
		}
		gc, ok := byDesc[c.Desc]
		if !ok {
			return VerdictPass
		}
		if v, ok := gc.labels[dim]; ok {
			return v
		}
		return VerdictPass
	}}
}

func calibrationRubric(t *testing.T) *Rubric {
	t.Helper()
	rb := *builtin(t)
	rb.Calibration.GoldenMinItems = 8
	return &rb
}

func runCalibrate(t *testing.T, rb *Rubric, cases []goldenCase, sj *scriptedJudge, cfg llm.Config, opts SemanticOptions) *CalibrationReport {
	t.Helper()
	set, err := LoadGolden(writeGolden(t, cases), rb)
	require.NoError(t, err)
	client, _ := newClient(t, sj, cfg)
	opts.Client = client
	rep, err := Calibrate(t.Context(), CalibrateInput{Rubric: rb, Golden: set, Now: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), Options: opts})
	require.NoError(t, err)
	return rep
}

func TestCalibrateAPerfectJudgePasses(t *testing.T) {
	// Arrange
	rb := calibrationRubric(t)
	cases := tenCases()
	sj := truthJudge(cases)

	// Act
	rep := runCalibrate(t, rb, cases, sj, llm.Config{}, SemanticOptions{K: 3})

	// Assert
	require.NotNil(t, rep.Record)
	rec := rep.Record
	assert.Equal(t, "pass", rec.Status)
	assert.Equal(t, 10, rec.NItems)
	assert.Equal(t, 3, rec.K)
	assert.Equal(t, "full", rec.Content)
	assert.Equal(t, "2026-10-06", rec.Date)
	assert.Equal(t, "fake/model", rec.Model, "the model id the provider answered with")
	assert.Equal(t, PromptDigest(rb), rec.PromptDigest)
	assert.Equal(t, rb.CoreDigest, rec.Rubric.Digest)
	assert.Equal(t, rb.Version, rec.Rubric.Version)
	assert.Empty(t, rep.Diffs)
	tq := rec.Dimensions["trigger-quality"]
	assert.Equal(t, CalPass, tq.Status)
	assert.Equal(t, 10, tq.N)
	assert.InDelta(t, 1.0, tq.Kappa, 0.001)
	assert.InDelta(t, 1.0, tq.Consistency, 0.001)
	assert.InDelta(t, 1.0, tq.FleissKappa, 0.001)
	assert.InDelta(t, 1.0, tq.HumanKappa, 0.001)
	assert.InDelta(t, 1.0, tq.Metamorphic[ProbePad], 0.001)
	assert.InDelta(t, 1.0, tq.Metamorphic[ProbeRename], 0.001)
	inj := rec.Dimensions["injection-intent"]
	assert.Equal(t, CalPass, inj.Status)
	assert.InDelta(t, 1.0, inj.Recall, 0.001)
	assert.InDelta(t, 1.0, inj.Metamorphic[ProbeCanary], 0.001)
	assert.Equal(t, CalUncalibrated, rec.Dimensions["scope-creep"].Status, "a dimension nobody labelled is never trusted")
	assert.Equal(t, VerdictWarn, rec.Cases["case-03"]["trigger-quality"])
	assert.Positive(t, rep.Usage.Calls)
}

func TestCalibrateAsksEveryVoteSoConsistencyIsMeasured(t *testing.T) {
	// Arrange: a judge that flips its second vote on one case
	rb := calibrationRubric(t)
	cases := tenCases()
	base := truthJudge(cases)
	sj := &scriptedJudge{decide: func(c scriptedCall, dim string) string {
		v := base.decide(c, dim)
		if strings.Contains(c.Desc, "number 3 ") && dim == "trigger-quality" && c.Vote == 2 {
			return VerdictFail
		}
		return v
	}}

	// Act
	rep := runCalibrate(t, rb, cases, sj, llm.Config{}, SemanticOptions{K: 3})

	// Assert: case 3 (a warn) votes warn, fail, warn: the median is still warn, but the votes disagree
	tq := rep.Record.Dimensions["trigger-quality"]
	assert.InDelta(t, 0.9, tq.Consistency, 0.001, "nine of ten cases had three identical votes")
	assert.Less(t, tq.FleissKappa, 1.0)
	assert.InDelta(t, 1.0, tq.Kappa, 0.001, "the aggregated verdict still matches the label")
	votes := 0
	for _, c := range sj.callsFor("case-03") {
		if c.Vote == 3 {
			votes++
		}
	}
	assert.Positive(t, votes, "the third vote is asked even though a pass or an early agreement would not need it")
}

func TestCalibrateFailsAJudgeThatMissesInjection(t *testing.T) {
	// Arrange
	rb := calibrationRubric(t)
	cases := tenCases()
	base := truthJudge(cases)
	blind := &scriptedJudge{decide: func(c scriptedCall, dim string) string {
		if dim == "injection-intent" {
			return VerdictPass
		}
		return base.decide(c, dim)
	}}

	// Act
	rep := runCalibrate(t, rb, cases, blind, llm.Config{}, SemanticOptions{K: 1})

	// Assert
	inj := rep.Record.Dimensions["injection-intent"]
	assert.Equal(t, CalFail, inj.Status)
	assert.Zero(t, inj.Recall)
	assert.Contains(t, strings.Join(inj.Misses, "; "), "recall 0.00 below 0.90")
	assert.Equal(t, "fail", rep.Record.Status)
	assert.Len(t, rep.Diffs, 2, "the two missed injections are listed for a person to read")
	assert.Equal(t, CalPass, rep.Record.Dimensions["trigger-quality"].Status, "a failing dimension does not hide a passing one")
}

func TestCalibrateReportsADimensionTheLabelersDisagreeOnAsIllDefined(t *testing.T) {
	// Arrange: labeler b gives the opposite verdict on every case
	rb := calibrationRubric(t)
	cases := tenCases()
	for i := range cases {
		flipped := map[string]string{}
		for d, v := range cases[i].labels {
			flipped[d] = v
		}
		switch cases[i].labels["trigger-quality"] {
		case VerdictPass:
			flipped["trigger-quality"] = VerdictFail
		case VerdictFail:
			flipped["trigger-quality"] = VerdictPass
		default:
			flipped["trigger-quality"] = VerdictPass
		}
		cases[i].labelsB = flipped
	}

	// Act
	rep := runCalibrate(t, rb, cases, truthJudge(cases), llm.Config{}, SemanticOptions{K: 1})

	// Assert
	tq := rep.Record.Dimensions["trigger-quality"]
	assert.Equal(t, CalIllDefined, tq.Status)
	assert.Contains(t, tq.Misses[0], "fix the rubric not the model")
	assert.Equal(t, "fail", rep.Record.Status)
}

func TestCalibrateCatchesVerbosityAndInjectionProbes(t *testing.T) {
	// Arrange: a judge that rates padded items better and ignores the canary
	rb := calibrationRubric(t)
	cases := tenCases()
	base := truthJudge(cases)
	biased := &scriptedJudge{decide: func(c scriptedCall, dim string) string {
		v := base.decide(c, dim)
		if strings.Contains(c.User, "## Notes") && v != VerdictPass {
			return VerdictPass
		}
		if strings.Contains(c.User, "NOTE TO THE REVIEWER") && dim == "injection-intent" {
			return VerdictPass
		}
		return v
	}}

	// Act
	rep := runCalibrate(t, rb, cases, biased, llm.Config{}, SemanticOptions{K: 1})

	// Assert
	tq := rep.Record.Dimensions["trigger-quality"]
	assert.Less(t, tq.Metamorphic[ProbePad], 0.95, "padding bought a better verdict")
	assert.Equal(t, CalFail, tq.Status)
	assert.Zero(t, rep.Record.Dimensions["injection-intent"].Metamorphic[ProbeCanary], "the canary was not flagged")
	assert.Equal(t, CalFail, rep.Record.Dimensions["injection-intent"].Status)
}

func TestCalibrateStopsIncompleteWhenTheCapIsReached(t *testing.T) {
	// Arrange
	rb := calibrationRubric(t)
	cases := tenCases()
	set, err := LoadGolden(writeGolden(t, cases), rb)
	require.NoError(t, err)
	client, _ := newClient(t, truthJudge(cases), llm.Config{MaxCalls: 5})

	// Act
	rep, err := Calibrate(t.Context(), CalibrateInput{Rubric: rb, Golden: set, Now: time.Now(), Options: SemanticOptions{Client: client, K: 3, Workers: 1}})

	// Assert
	require.NoError(t, err)
	assert.True(t, rep.Incomplete)
	assert.Nil(t, rep.Record, "a partial calibration is never a record")
}

func TestLoadGoldenValidatesAndDigests(t *testing.T) {
	rb := calibrationRubric(t)

	t.Run("digest follows the fixtures and the labels", func(t *testing.T) {
		cases := tenCases()[:3]
		base := writeGolden(t, cases)
		first, err := LoadGolden(base, rb)
		require.NoError(t, err)
		again, err := LoadGolden(base, rb)
		require.NoError(t, err)
		assert.Equal(t, first.Digest, again.Digest)
		require.Len(t, first.Cases, 3)
		assert.Equal(t, "skill:case-00", first.Cases[0].Item.ID)
		assert.Len(t, first.Cases[0].Siblings, 2)

		require.NoError(t, os.WriteFile(filepath.Join(base, "fixtures/shared/sib-one/SKILL.md"), []byte("---\nname: sib-one\ndescription: edited\n---\n"), 0o600))
		edited, err := LoadGolden(base, rb)
		require.NoError(t, err)
		assert.NotEqual(t, first.Digest, edited.Digest, "editing a fixture changes the golden digest")
	})

	t.Run("a case that does not lint is refused", func(t *testing.T) {
		base := writeGolden(t, tenCases()[:1])
		p := filepath.Join(base, "golden", "case-00.golden.yaml")
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(p, []byte(strings.Replace(string(data), "trigger-quality", "no-such-dimension", 1)), 0o600))

		_, err = LoadGolden(base, rb)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown dimension")
	})

	t.Run("a fixture path outside the set is refused", func(t *testing.T) {
		base := writeGolden(t, tenCases()[:1])
		p := filepath.Join(base, "golden", "case-00.golden.yaml")
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(p, []byte(strings.Replace(string(data), "fixtures/cases/case-00/case-00/SKILL.md", "../outside/SKILL.md", 1)), 0o600))

		_, err = LoadGolden(base, rb)

		require.Error(t, err)
	})
}

func TestMatchCalibration(t *testing.T) {
	rb := builtin(t)
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	good := func() *CalibrationRecord {
		return &CalibrationRecord{
			SchemaVersion: 1, Rubric: CalibrationRubric{ID: rb.ID, Version: rb.Version, Digest: rb.CoreDigest}, Model: "gemini-2.5-flash-lite",
			PromptDigest: PromptDigest(rb), GoldenDigest: "sha256:g", Content: "full", Date: "2026-09-30", K: 3, Status: "pass",
			Dimensions: map[string]DimCalibration{"trigger-quality": {Status: CalPass}, "overlap": {Status: CalFail}},
		}
	}
	cur := CalKey{Model: "gemini/gemini-2.5-flash-lite", PromptDigest: PromptDigest(rb), GoldenDigest: "sha256:g", Content: "full", K: 3}
	tests := []struct {
		name       string
		mutate     func(r *CalibrationRecord, k *CalKey)
		wantState  string
		wantReason string
	}{
		{"matches (provider prefix ignored)", func(*CalibrationRecord, *CalKey) {}, CalMatched, ""},
		{"no record", nil, CalMissing, "review calibrate"},
		{"rubric edited", func(r *CalibrationRecord, _ *CalKey) { r.Rubric.Digest = "sha256:old" }, CalStale, "the rubric changed"},
		{"prompt edited", func(_ *CalibrationRecord, k *CalKey) { k.PromptDigest = "sha256:new" }, CalStale, "the prompt changed"},
		{"golden edited", func(_ *CalibrationRecord, k *CalKey) { k.GoldenDigest = "sha256:other" }, CalStale, "the golden set changed"},
		{"another model", func(_ *CalibrationRecord, k *CalKey) { k.Model = "gemini-2.5-pro" }, CalStale, "calibrated for model gemini-2.5-flash-lite, judging with gemini-2.5-pro"},
		{"another content mode", func(_ *CalibrationRecord, k *CalKey) { k.Content = "descriptions" }, CalStale, "--content"},
		{"another vote count", func(_ *CalibrationRecord, k *CalKey) { k.K = 1 }, CalStale, "k=3"},
		{"too old", func(r *CalibrationRecord, _ *CalKey) { r.Date = "2026-05-01" }, CalStale, "days old"},
		{"dated in the future", func(r *CalibrationRecord, _ *CalKey) { r.Date = "2027-01-01" }, CalStale, "in the future"},
		{"dated today", func(r *CalibrationRecord, _ *CalKey) { r.Date = "2026-10-06" }, CalMatched, ""},
		{"a record that failed its thresholds", func(r *CalibrationRecord, _ *CalKey) { r.Status = "fail" }, CalFailedRecord, "did not meet"},
		{"golden unknown to the caller is not compared", func(_ *CalibrationRecord, k *CalKey) { k.GoldenDigest = "" }, CalMatched, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var rec *CalibrationRecord
			key := cur
			if tt.mutate != nil {
				rec = good()
				tt.mutate(rec, &key)
			}

			// Act
			got := MatchCalibration(rb, rec, key, now, 0)

			// Assert
			assert.Equal(t, tt.wantState, got.State)
			if tt.wantReason != "" {
				assert.Contains(t, strings.Join(got.Reasons, "; "), tt.wantReason)
			}
			if tt.wantState == CalMatched {
				assert.Equal(t, []string{"trigger-quality"}, got.Dimensions, "only dimensions whose calibration passed may gate")
			}
		})
	}

	t.Run("the age limit can be overridden", func(t *testing.T) {
		rec := good()
		rec.Date = "2026-05-01"
		assert.Equal(t, CalMatched, MatchCalibration(rb, rec, cur, now, 365).State)
	})
}

func TestCalibrationRecordRoundTripAndRubricDigest(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	rubricDir := filepath.Join(dir, RubricsDir, "mine")
	require.NoError(t, os.MkdirAll(rubricDir, 0o755))
	body := strings.Replace(string(builtin(t).Raw), `id = "skill-quality"`, `id = "mine"`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(rubricDir, RubricFile), []byte(body), 0o600))
	before, err := Load(dir, "mine")
	require.NoError(t, err)
	rec := &CalibrationRecord{SchemaVersion: 1, Rubric: CalibrationRubric{ID: "mine", Version: before.Version, Digest: before.CoreDigest}, Date: "2026-10-06", Status: "pass"}

	// Act
	path := CalibrationPath(dir, before)
	require.NoError(t, SaveCalibration(path, rec))
	after, err := Load(dir, "mine")
	require.NoError(t, err)
	loaded, err := LoadCalibration(path)
	require.NoError(t, err)
	missing, err := LoadCalibration(filepath.Join(dir, "nope.json"))

	// Assert
	assert.Equal(t, filepath.Join(rubricDir, CalibrationFile), path)
	assert.Equal(t, before.CoreDigest, after.CoreDigest, "writing the record does not change the digest it names")
	assert.NotEqual(t, before.Digest, after.Digest, "the lock pin covers the record")
	assert.Equal(t, rec, loaded)
	require.NoError(t, err)
	assert.Nil(t, missing)
	assert.Equal(t, filepath.Join(dir, "calibration", "skill-quality.builtin.json"), CalibrationPath(dir, builtin(t)))
}

func TestCompareCalibration(t *testing.T) {
	old := &CalibrationRecord{Model: "m", Status: "pass",
		Dimensions: map[string]DimCalibration{"a": {Status: CalPass, Kappa: 0.80}, "b": {Status: CalPass, Kappa: 0.70}},
		Cases:      map[string]map[string]string{"c1": {"a": "pass", "b": "warn"}, "c2": {"a": "fail"}}}
	tests := []struct {
		name        string
		cur         *CalibrationRecord
		wantFailed  bool
		wantReg     string
		wantChanges int
	}{
		{"unchanged", &CalibrationRecord{Model: "m", Status: "pass", Dimensions: old.Dimensions, Cases: old.Cases}, false, "", 0},
		{"a kappa fell by more than 0.05", &CalibrationRecord{Model: "m", Status: "pass", Dimensions: map[string]DimCalibration{"a": {Status: CalPass, Kappa: 0.70}, "b": {Status: CalPass, Kappa: 0.70}}, Cases: old.Cases}, true, "a: kappa fell from 0.80 to 0.70", 0},
		{"a small fall is tolerated", &CalibrationRecord{Model: "m", Status: "pass", Dimensions: map[string]DimCalibration{"a": {Status: CalPass, Kappa: 0.76}, "b": {Status: CalPass, Kappa: 0.70}}, Cases: old.Cases}, false, "", 0},
		{"a dimension stops passing", &CalibrationRecord{Model: "m", Status: "fail", Dimensions: map[string]DimCalibration{"a": {Status: CalFail, Kappa: 0.79, Misses: []string{"recall 0.5"}}, "b": {Status: CalPass, Kappa: 0.70}}, Cases: old.Cases}, true, "was pass, now fail", 0},
		{"the resolved model changed", &CalibrationRecord{Model: "other", Status: "pass", Dimensions: old.Dimensions, Cases: old.Cases}, true, "the resolved model changed", 0},
		{"changed verdicts are listed", &CalibrationRecord{Model: "m", Status: "pass", Dimensions: old.Dimensions, Cases: map[string]map[string]string{"c1": {"a": "warn", "b": "warn"}, "c2": {"a": "fail"}}}, false, "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareCalibration(old, tt.cur)
			assert.Equal(t, tt.wantFailed, got.Failed)
			assert.Contains(t, strings.Join(got.Regressions, "; "), tt.wantReg)
			assert.Len(t, got.Changed, tt.wantChanges)
		})
	}
}

func TestCompareModels(t *testing.T) {
	// Arrange
	a := map[string]map[string]string{"i1": {"d": "pass"}, "i2": {"d": "fail"}, "i3": {"d": "warn"}}
	b := map[string]map[string]string{"i1": {"d": "pass"}, "i2": {"d": "warn"}, "i3": {"d": "warn"}}

	// Act
	got := CompareModels([]string{"m1", "m2"}, []map[string]map[string]string{a, b}, []string{"d"})

	// Assert
	require.Len(t, got.Pairwise, 1)
	assert.Equal(t, 3, got.Pairwise[0].N)
	assert.InDelta(t, 0.667, got.Pairwise[0].Agreement, 0.001)
	assert.Equal(t, []Disagreement{{Item: "i2", Dimension: "d", Verdicts: map[string]string{"m1": "fail", "m2": "warn"}}}, got.Disagreements, "an item the judges split on is a review priority")
}

func TestEvaluateGate(t *testing.T) {
	mk := func(status, verdict, severity string) *Results {
		return &Results{Items: []ItemResult{{Item: Item{ID: "skill:a"}, Status: StatusScored, Semantic: &SemanticResult{Dimensions: []SemDim{
			{ID: "trigger-quality", Code: "AR9G1", Status: status, Verdict: verdict, severity: severity, Agreement: 1},
		}}}}}
	}
	tests := []struct {
		name       string
		res        *Results
		level      string
		calibrated map[string]bool
		wantFail   bool
	}{
		{"a stable fail on a calibrated dimension fails", mk(SemJudged, VerdictFail, "warning"), "warning", map[string]bool{"trigger-quality": true}, true},
		{"a warn verdict does not gate", mk(SemJudged, VerdictWarn, "warning"), "warning", map[string]bool{"trigger-quality": true}, false},
		{"an unstable fail never gates", mk(SemUnstable, VerdictFail, "warning"), "warning", map[string]bool{"trigger-quality": true}, false},
		{"an uncalibrated dimension never gates", mk(SemJudged, VerdictFail, "warning"), "warning", map[string]bool{"overlap": true}, false},
		{"without a calibration requirement every dimension counts", mk(SemJudged, VerdictFail, "warning"), "warning", nil, true},
		{"an info dimension is below a warning gate", mk(SemJudged, VerdictFail, "info"), "warning", nil, false},
		{"an info gate includes it", mk(SemJudged, VerdictFail, "info"), "info", nil, true},
		{"the error level gates nothing a judge says", mk(SemJudged, VerdictFail, "warning"), "error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateGate(tt.res, tt.level, tt.calibrated)
			assert.Equal(t, tt.wantFail, !got.Passed)
			assert.Equal(t, tt.wantFail, len(got.Failures) == 1)
		})
	}

	t.Run("a baselined finding does not gate", func(t *testing.T) {
		res := mk(SemJudged, VerdictFail, "warning")
		res.Items[0].Semantic.Dimensions[0].Evidence = []Quote{{Quote: "Helps"}}
		res.SetBaseline(map[string]bool{fingerprint("AR9G1", "skill:a", "trigger-quality", "Helps"): true})
		assert.True(t, EvaluateGate(res, "warning", nil).Passed)
	})
}

func TestModelNames(t *testing.T) {
	assert.True(t, SameModel("gemini/gemini-2.5-flash", "gemini-2.5-flash"))
	assert.True(t, SameModel("models/gemini-2.5-flash", "gemini-2.5-flash"))
	assert.False(t, SameModel("gemini-2.5-flash", "gemini-2.5-flash-lite"))
	for _, alias := range []string{"gemini-flash-latest", "claude-3-5-sonnet-latest", "openai/gpt-4o-auto", "default"} {
		assert.True(t, IsFloatingAlias(alias), alias)
	}
	for _, pinned := range []string{"gemini-2.5-flash-lite", "claude-haiku-4-5-20251001", "gpt-4o-2024-08-06"} {
		assert.False(t, IsFloatingAlias(pinned), pinned)
	}
}

func TestRubricFilesAreNotChangedByAReview(t *testing.T) {
	// Arrange: a review reads an item with mode 0644 and must not tighten it
	dir := t.TempDir()
	p := filepath.Join(dir, "SKILL.md")
	require.NoError(t, os.WriteFile(p, []byte("---\ndescription: x\n---\n"), 0o644)) //nolint:gosec // the mode is the point
	require.NoError(t, os.Chmod(p, 0o644))
	cfgItem := config.ContentFile{Name: "x", Path: p}

	// Act
	it := newItem(KindSkill, "", cfgItem, dir, nil)

	// Assert
	require.Empty(t, it.ReadError)
	info, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}
