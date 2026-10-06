package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
)

var (
	reqItemRe = regexp.MustCompile(`<<<DATA-[0-9a-f]+ item=skill:([^>]+)>>>`)
	reqDimRe  = regexp.MustCompile(`(?m)^dimension ([a-z0-9-]+):`)
	reqDescRe = regexp.MustCompile(`(?m)^description: (.*)$`)
)

// fakeModel is a model that judges by a table and records every request.
type fakeModel struct {
	mu       sync.Mutex
	verdicts map[string]string // "item/dimension" -> verdict
	calls    int
	users    []string
	fixer    func(req llm.ChatRequest) string
	// decide, when set, picks a verdict from the item, its description and the dimension.
	decide func(item, desc, dim string) string
}

func (f *fakeModel) chat(req llm.ChatRequest) (string, error) {
	user := req.Messages[len(req.Messages)-1].Content
	f.mu.Lock()
	f.calls++
	f.users = append(f.users, user)
	f.mu.Unlock()
	if req.ResponseFormat != nil && req.ResponseFormat.Name == "review_fix" && f.fixer != nil {
		return f.fixer(req), nil
	}
	m := reqItemRe.FindStringSubmatch(user)
	if m == nil {
		return "{}", nil
	}
	desc := ""
	if d := reqDescRe.FindStringSubmatch(user); d != nil {
		desc = strings.TrimSpace(d[1])
	}
	var dims []map[string]any
	for _, d := range reqDimRe.FindAllStringSubmatch(user, -1) {
		v := f.verdicts[m[1]+"/"+d[1]]
		if f.decide != nil {
			v = f.decide(m[1], desc, d[1])
		}
		if v == "" {
			v = "pass"
		}
		ev := []map[string]string{}
		if v != "pass" && len(desc) >= 10 {
			ev = append(ev, map[string]string{"quote": desc[:10], "where": "description"})
		}
		dims = append(dims, map[string]any{"id": d[1], "verdict": v, "evidence": ev, "rationale": "r " + d[1], "suggestion": "s"})
	}
	b, err := json.Marshal(map[string]any{"dimensions": dims})
	return string(b), err
}

// useFakeModel replaces the client factory with the real middleware around a fake backend.
func useFakeModel(t *testing.T, f *fakeModel) {
	t.Helper()
	old := reviewClientFactory
	reviewClientFactory = func(lc llm.Config, opts llm.Options) (llm.Client, error) {
		fake := llm.NewFake()
		fake.ChatFunc = f.chat
		return llm.Wrap(fake, lc, opts), nil
	}
	t.Cleanup(func() { reviewClientFactory = old })
}

// judgedProject is a review project with network opt-in and a model, and the flags a judged run needs.
func judgedProject(t *testing.T, extraConfig string) {
	t.Helper()
	reviewProject(t, extraConfig)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "1")
	t.Setenv("AI_RULEZ_LLM_MODEL", "model")
	reviewFlags.semantic, reviewFlags.format, reviewFlags.content = true, "json", "full"
	require.NoError(t, ReviewCmd.Flags().Set("max-cost", "0"))
	reviewNow = func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { reviewNow = time.Now })
}

type judgedReport struct {
	Run struct {
		Calls      int
		Cached     int
		Incomplete bool
	}
	Items []struct {
		ID       string
		Redacted bool
		Semantic *struct {
			Score      *int
			Dimensions []struct{ ID, Status, Verdict string }
		}
	}
	Findings []struct{ Code, Item, Origin, Fingerprint, Dimension string }
	Gate     *struct {
		Requested, Passed bool
		Refused           string
		Failures          []struct{ Item, Dimension string }
	}
	Calibration *struct {
		Status  string
		Reasons []string
	}
	Models *struct {
		Models        []string
		Disagreements []struct{ Item, Dimension string }
	}
}

func runJudged(t *testing.T, args ...string) (judgedReport, int, error) {
	t.Helper()
	var out bytes.Buffer
	exit, err := runReview(ReviewCmd, args, &out)
	var rep judgedReport
	if err == nil && reviewFlags.format == "json" {
		require.NoError(t, json.Unmarshal(out.Bytes(), &rep), out.String())
		validateAgainst(t, "../../schema/review-report.schema.json", out.Bytes())
	}
	return rep, exit, err
}

func TestReviewSemanticNeedsTheNetworkOptInAndAModel(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"no network opt-in", map[string]string{"AI_RULEZ_LLM_MODEL": "model"}, "LLM access is disabled"},
		{"no model", map[string]string{"AI_RULEZ_LLM_ALLOW_NETWORK": "1"}, "no model configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			judgedProject(t, "")
			t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "")
			t.Setenv("AI_RULEZ_LLM_MODEL", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			f := &fakeModel{}
			useFakeModel(t, f)

			// Act
			_, exit, err := runJudged(t)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Zero(t, f.calls, "nothing is sent")
			assert.Equal(t, exitReviewRefused, exit)
		})
	}
}

func TestReviewSemanticRepositoryConfigCannotEnableTheNetwork(t *testing.T) {
	// Arrange: the repository config opts in; only user scope may
	judgedProject(t, "\n[llm]\nallow_network = true\nmodel = \"model\"\n")
	t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "")
	f := &fakeModel{}
	useFakeModel(t, f)

	// Act
	_, _, err := runJudged(t)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM access is disabled")
	assert.Zero(t, f.calls)
}

func TestReviewSemanticJudgesAndCaches(t *testing.T) {
	// Arrange
	judgedProject(t, "")
	f := &fakeModel{verdicts: map[string]string{"deploy/trigger-quality": "fail"}}
	useFakeModel(t, f)

	// Act
	first, exit, err := runJudged(t)
	callsAfterFirst := f.calls
	second, _, err2 := runJudged(t)

	// Assert
	require.NoError(t, err)
	require.NoError(t, err2)
	assert.Zero(t, exit)
	assert.Positive(t, first.Run.Calls)
	var judged []string
	for _, fi := range first.Findings {
		if fi.Origin == "llm-judge" {
			judged = append(judged, fi.Code+" "+fi.Item)
		}
	}
	assert.Equal(t, []string{"AR9G1 skill:deploy"}, judged)
	assert.Equal(t, callsAfterFirst, f.calls, "an unchanged project costs nothing on re-run")
	assert.Zero(t, second.Run.Calls)
	assert.Equal(t, first.Run.Calls, second.Run.Cached)
	assert.Equal(t, "missing", first.Calibration.Status)
	for _, u := range f.users {
		assert.NotContains(t, u, "AKIAIOSFODNN7EXAMPLE", "the withheld item is never sent")
	}
}

func TestReviewSemanticHonoursTheSpendCap(t *testing.T) {
	// Arrange
	judgedProject(t, "")
	f := &fakeModel{verdicts: map[string]string{"deploy/trigger-quality": "fail", "release/trigger-quality": "fail"}}
	useFakeModel(t, f)
	require.NoError(t, ReviewCmd.Flags().Set("max-calls", "4"))

	// Act: the plan fits (one vote per call), the extra votes of the flagged dimensions do not
	rep, exit, err := runJudged(t)

	// Assert
	require.NoError(t, err)
	assert.Zero(t, exit)
	assert.True(t, rep.Run.Incomplete)
	assert.Equal(t, 4, f.calls)
}

func TestReviewSemanticAllowedHostsAreUserScopeOnly(t *testing.T) {
	t.Run("a user-scope allow-list refuses another host", func(t *testing.T) {
		judgedProject(t, "")
		userCfg := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "ai-rulez")
		require.NoError(t, os.MkdirAll(userCfg, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(userCfg, "config.toml"), []byte("[review]\nallowed_hosts = [\"gateway.internal\"]\n"), 0o600))
		t.Setenv("AI_RULEZ_LLM_BASE_URL", "https://elsewhere.example/v1")
		f := &fakeModel{}
		useFakeModel(t, f)

		_, _, err := runJudged(t)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "elsewhere.example is not in [review] allowed_hosts")
		assert.Zero(t, f.calls)
	})

	t.Run("a listed host is allowed", func(t *testing.T) {
		judgedProject(t, "")
		t.Setenv("AI_RULEZ_REVIEW_ALLOWED_HOSTS", "gateway.internal")
		t.Setenv("AI_RULEZ_LLM_BASE_URL", "https://gateway.internal/v1")
		f := &fakeModel{}
		useFakeModel(t, f)

		_, _, err := runJudged(t)

		require.NoError(t, err)
		assert.Positive(t, f.calls)
	})

	t.Run("a repository allow-list is ignored", func(t *testing.T) {
		judgedProject(t, "\n[review]\nallowed_hosts = [\"elsewhere.example\"]\n")
		t.Setenv("AI_RULEZ_REVIEW_ALLOWED_HOSTS", "gateway.internal")
		t.Setenv("AI_RULEZ_LLM_BASE_URL", "https://elsewhere.example/v1")
		f := &fakeModel{}
		useFakeModel(t, f)

		_, _, err := runJudged(t)

		require.Error(t, err, "the repository cannot widen the list the user set")
		assert.Contains(t, err.Error(), "not in [review] allowed_hosts")
	})
}

func TestReviewSemanticRepositoryCapsOnlyTighten(t *testing.T) {
	// Arrange: the repository asks for a 1000 call cap; the default 300 stands
	judgedProject(t, "\n[review]\nmax_calls = 1000\n")
	require.NoError(t, ReviewCmd.Flags().Set("max-cost", "0"))
	ReviewCmd.Flags().Lookup("max-calls").Changed = false
	reviewFlags.estimate, reviewFlags.semantic, reviewFlags.format = true, false, "json"
	var out bytes.Buffer

	// Act
	_, err := runReview(ReviewCmd, nil, &out)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, out.String(), `"max_calls": 300`)
}

func TestReviewSemanticRedactModeSendsMaskedText(t *testing.T) {
	// Arrange
	judgedProject(t, "\n[review]\non_secret = \"redact\"\n")
	f := &fakeModel{}
	useFakeModel(t, f)

	// Act
	rep, _, err := runJudged(t)

	// Assert
	require.NoError(t, err)
	redacted := false
	for _, it := range rep.Items {
		if it.ID == "skill:leak" {
			redacted = it.Redacted && it.Semantic != nil
		}
	}
	assert.True(t, redacted)
	for _, u := range f.users {
		assert.NotContains(t, u, "AKIAIOSFODNN7EXAMPLE")
	}
}

func writeCalibrationFor(t *testing.T, model, status string, dims map[string]string) {
	t.Helper()
	rec := map[string]any{
		"schema_version": 1, "rubric": map[string]any{"id": "skill-quality", "version": 2, "digest": rubricCoreDigest(t)},
		"model": model, "prompt_digest": promptDigest(t), "golden_digest": "", "content": "full", "date": "2026-10-01", "k": 3, "n_items": 50,
		"status": status, "dimensions": map[string]any{},
	}
	for d, st := range dims {
		rec["dimensions"].(map[string]any)[d] = map[string]any{"status": st}
	}
	raw, err := json.Marshal(rec)
	require.NoError(t, err)
	dir := filepath.Join(".ai-rulez", "calibration")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skill-quality.builtin.json"), raw, 0o600))
}

func TestReviewGateIsEarnedByACalibrationRecord(t *testing.T) {
	failing := map[string]string{"deploy/trigger-quality": "fail"}

	t.Run("refused without a record, and nothing is spent", func(t *testing.T) {
		judgedProject(t, "")
		reviewFlags.gate = true
		f := &fakeModel{verdicts: failing}
		useFakeModel(t, f)

		_, exit, err := runJudged(t)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--gate is refused: calibration missing")
		assert.Equal(t, exitReviewRefused, exit)
		assert.Zero(t, f.calls)
	})

	t.Run("refused for a floating alias", func(t *testing.T) {
		judgedProject(t, "")
		t.Setenv("AI_RULEZ_LLM_MODEL", "model-latest")
		reviewFlags.gate = true
		f := &fakeModel{verdicts: failing}
		useFakeModel(t, f)

		_, _, err := runJudged(t)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "floating alias")
		assert.Zero(t, f.calls)
	})

	t.Run("a matching record lets a stable fail fail the gate (exit 2)", func(t *testing.T) {
		judgedProject(t, "")
		writeCalibrationFor(t, "model", "pass", map[string]string{"trigger-quality": "pass"})
		reviewFlags.gate = true
		f := &fakeModel{verdicts: failing}
		useFakeModel(t, f)

		rep, exit, err := runJudged(t)

		require.NoError(t, err)
		assert.Equal(t, exitReviewGate, exit)
		require.NotNil(t, rep.Gate)
		assert.False(t, rep.Gate.Passed)
		assert.Equal(t, "matched", rep.Calibration.Status)
		assert.Equal(t, "skill:deploy", rep.Gate.Failures[0].Item)
	})

	t.Run("a dimension the record did not calibrate never gates", func(t *testing.T) {
		judgedProject(t, "")
		writeCalibrationFor(t, "model", "pass", map[string]string{"overlap": "pass"})
		reviewFlags.gate = true
		f := &fakeModel{verdicts: failing}
		useFakeModel(t, f)

		rep, exit, err := runJudged(t)

		require.NoError(t, err)
		assert.Zero(t, exit)
		assert.True(t, rep.Gate.Passed)
	})

	t.Run("an item cut before the judge saw it refuses the gate", func(t *testing.T) {
		judgedProject(t, "")
		writeCalibrationFor(t, "model", "pass", map[string]string{"trigger-quality": "pass"})
		body := "---\nname: release\ndescription: Use when asked to cut a release of the service; not for deployments.\n---\n" + strings.Repeat("Run the release script.\n", 3000)
		require.NoError(t, os.WriteFile(filepath.Join(".ai-rulez", "skills", "release", "SKILL.md"), []byte(body), 0o600))
		reviewFlags.gate = true
		useFakeModel(t, &fakeModel{})

		rep, exit, err := runJudged(t)

		require.NoError(t, err)
		assert.Equal(t, exitReviewRefused, exit)
		require.NotNil(t, rep.Gate)
		assert.Contains(t, rep.Gate.Refused, "skill:release")
	})

	t.Run("a record for another model is stale", func(t *testing.T) {
		judgedProject(t, "")
		writeCalibrationFor(t, "another-model", "pass", map[string]string{"trigger-quality": "pass"})
		reviewFlags.gate = true
		useFakeModel(t, &fakeModel{verdicts: failing})

		_, _, err := runJudged(t)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "calibrated for model another-model")
	})

	t.Run("require_calibration = false lets the gate run uncalibrated", func(t *testing.T) {
		judgedProject(t, "\n[review.gate]\nrequire_calibration = false\n")
		reviewFlags.gate = true
		useFakeModel(t, &fakeModel{verdicts: failing})

		rep, exit, err := runJudged(t)

		require.NoError(t, err)
		assert.Equal(t, exitReviewGate, exit)
		assert.Equal(t, "missing", rep.Calibration.Status, "AR9G9 still reports it")
	})
}

func TestReviewBaselineSuppressesKnownFindingsFromTheGate(t *testing.T) {
	// Arrange
	judgedProject(t, "\n[review.gate]\nrequire_calibration = false\n")
	f := &fakeModel{verdicts: map[string]string{"deploy/trigger-quality": "fail"}}
	useFakeModel(t, f)
	base := filepath.Join(t.TempDir(), "baseline.json")
	reviewFlags.writeBaseline = base
	_, _, err := runJudged(t)
	require.NoError(t, err)

	// Act
	reviewFlags.writeBaseline, reviewFlags.baseline, reviewFlags.gate = "", base, true
	rep, exit, err := runJudged(t)

	// Assert
	require.NoError(t, err)
	assert.Zero(t, exit, "an accepted finding does not gate")
	hidden := 0
	for _, fi := range rep.Findings {
		_ = fi
		hidden++
	}
	assert.Positive(t, hidden)
	assert.True(t, rep.Gate.Passed)
}

func TestReviewSemanticModelsComparison(t *testing.T) {
	// Arrange: both models are the same fake, so there is no disagreement and a pairwise row per dimension
	judgedProject(t, "")
	f := &fakeModel{verdicts: map[string]string{"deploy/trigger-quality": "warn"}}
	useFakeModel(t, f)
	reviewFlags.models = "model,other-model"
	require.NoError(t, ReviewCmd.Flags().Set("models", "model,other-model"))

	// Act
	rep, _, err := runJudged(t)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, rep.Models)
	assert.Equal(t, []string{"model", "other-model"}, rep.Models.Models)
}

func TestReviewSelectionBySince(t *testing.T) {
	// Arrange
	judgedProject(t, "")
	gitRun := func(args ...string) {
		cmd := gitutil.CommandNoContext("", append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "user.email=t@example.test", "-c", "user.name=t"}, args...)...) //nolint:gosec // test
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	gitRun("init", "-q")
	gitRun("add", "-A")
	gitRun("commit", "-q", "-m", "one")
	require.NoError(t, os.WriteFile(".ai-rulez/skills/release/SKILL.md", []byte("---\nname: release\ndescription: Use when asked to cut a release; not for deployments or rollbacks.\n---\nChanged.\n"), 0o600))
	useFakeModel(t, &fakeModel{})
	reviewFlags.since = "HEAD"

	// Act
	rep, _, err := runJudged(t)

	// Assert
	require.NoError(t, err)
	var ids []string
	for _, it := range rep.Items {
		ids = append(ids, it.ID)
	}
	assert.Equal(t, []string{"skill:release"}, ids)
}

func TestReviewFlagValidation(t *testing.T) {
	tests := []struct {
		name  string
		setup func()
		want  string
	}{
		{"gate without semantic", func() { reviewFlags.gate = true; ReviewCmd.Flags().Lookup("gate").Changed = true }, "--gate applies to --semantic only"},
		{"gate level without gate", func() {
			reviewFlags.semantic, reviewFlags.gateLevel = true, "warning"
			ReviewCmd.Flags().Lookup("gate-level").Changed = true
		}, "--gate-level needs --gate"},
		{"unknown gate level", func() { reviewFlags.semantic, reviewFlags.gate, reviewFlags.gateLevel = true, true, "fatal" }, "unknown --gate-level"},
		{"role and profile", func() { reviewFlags.role, reviewFlags.profile = "a", "b" }, "mutually exclusive"},
		{"gate with estimate", func() { reviewFlags.estimate, reviewFlags.gate, reviewFlags.semantic = true, true, true }, "--gate needs a judged run"},
		{"k without semantic", func() { reviewFlags.k = 2; ReviewCmd.Flags().Lookup("k").Changed = true }, "--k applies to --semantic only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reviewProject(t, "")
			tt.setup()

			_, err := runReview(ReviewCmd, nil, &bytes.Buffer{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestReviewExplainPrintsTheDimensionDefinitions(t *testing.T) {
	// Arrange
	reviewProject(t, "")
	var buf bytes.Buffer
	reviewExplainCmd.SetOut(&buf)
	t.Cleanup(func() { reviewExplainCmd.SetOut(nil) })

	// Act
	err := runReviewExplain(reviewExplainCmd, "ar9g2")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "AR9G2 trigger-overlap")
	assert.Contains(t, buf.String(), "Would a model confuse this skill with any listed sibling")
	assert.Contains(t, buf.String(), "A realistic request fits it and a sibling equally well")
	assert.Error(t, runReviewExplain(reviewExplainCmd, "AR001"))
}

func rubricCoreDigest(t *testing.T) string {
	t.Helper()
	rb, err := rv.Load("", "")
	require.NoError(t, err)
	return rb.CoreDigest
}

func promptDigest(t *testing.T) string {
	t.Helper()
	rb, err := rv.Load("", "")
	require.NoError(t, err)
	return rv.PromptDigest(rb)
}

func TestReviewSelectionByProfileAndRole(t *testing.T) {
	// Arrange: two domains, a profile and a role for each
	judgedProject(t, "\n[profiles]\nbackend = [\"backend\"]\nfrontend = [\"frontend\"]\n\n[[roles]]\nname = \"web\"\ndomains = [\"frontend\"]\n")
	for _, d := range []string{"backend", "frontend"} {
		dir := filepath.Join(".ai-rulez", "domains", d, "skills", d+"-skill")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+d+"-skill\ndescription: Use for "+d+" work only; not for anything else\n---\nbody\n"), 0o600))
	}
	reviewFlags.semantic, reviewFlags.format = false, "json"
	ReviewCmd.Flags().Lookup("max-cost").Changed = false
	ids := func() []string {
		var out bytes.Buffer
		_, err := runReview(ReviewCmd, nil, &out)
		require.NoError(t, err)
		var rep judgedReport
		require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
		var got []string
		for _, it := range rep.Items {
			if strings.HasSuffix(it.ID, "-skill") {
				got = append(got, it.ID)
			}
		}
		return got
	}

	// Act and Assert
	reviewFlags.profile = "backend"
	assert.Equal(t, []string{"skill:backend/backend-skill"}, ids())
	reviewFlags.profile = "frontend"
	assert.Equal(t, []string{"skill:frontend/frontend-skill"}, ids())
	reviewFlags.profile, reviewFlags.role = "", "web"
	assert.Equal(t, []string{"skill:frontend/frontend-skill"}, ids())
	reviewFlags.role = "nope"
	_, err := runReview(ReviewCmd, nil, &bytes.Buffer{})
	require.Error(t, err)
}
