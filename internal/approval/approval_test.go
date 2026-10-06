package approval

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

const (
	digestA = "sha256:aaaa"
	digestB = "sha256:bbbb"
)

func rec(kind, id, digest, reviewer string, mods ...func(*lockfile.Approval)) lockfile.Approval {
	a := lockfile.Approval{Kind: kind, ID: id, Digest: digest, Reviewer: reviewer, Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-10-01T00:00:00Z"}
	for _, m := range mods {
		m(&a)
	}
	return a
}

func expires(date string) func(*lockfile.Approval) {
	return func(a *lockfile.Approval) { a.Expires = date }
}

func TestEvaluate(t *testing.T) {
	hook := Subject{Kind: "hook", ID: "PreToolUse:*:0", Digest: digestB, Class: ClassLocal}
	include := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}
	tests := []struct {
		name    string
		policy  Policy
		subject Subject
		recs    []lockfile.Approval
		want    string
		code    string
	}{
		{"not selected", Policy{Selectors: []string{"remote"}}, hook, nil, StatusNotRequired, ""},
		{"no policy", Policy{}, include, nil, StatusNotRequired, ""},
		{"missing", Policy{Selectors: []string{"remote"}}, include, nil, StatusMissing, "AR710"},
		{"ok", Policy{Selectors: []string{"remote"}}, include, []lockfile.Approval{rec("include", "shared", digestB, "alice")}, StatusOK, ""},
		{"stale when only an older digest was approved", Policy{Selectors: []string{"remote"}}, include,
			[]lockfile.Approval{rec("include", "shared", digestA, "alice")}, StatusStale, "AR711"},
		{"a record of another item does not count", Policy{Selectors: []string{"remote"}}, include,
			[]lockfile.Approval{rec("include", "other", digestB, "alice")}, StatusMissing, "AR710"},
		{"expired", Policy{Selectors: []string{"remote"}}, include,
			[]lockfile.Approval{rec("include", "shared", digestB, "alice", expires("2026-10-04"))}, StatusExpired, "AR712"},
		{"holds through the expiry date", Policy{Selectors: []string{"remote"}}, include,
			[]lockfile.Approval{rec("include", "shared", digestB, "alice", expires("2026-10-05"))}, StatusOK, ""},
		{"a malformed expiry fails closed", Policy{Selectors: []string{"remote"}}, include,
			[]lockfile.Approval{rec("include", "shared", digestB, "alice", expires("someday"))}, StatusExpired, "AR712"},
		{"reviewer outside the allowlist", Policy{Selectors: []string{"remote"}, Approvers: []string{"carol"}}, include,
			[]lockfile.Approval{rec("include", "shared", digestB, "alice")}, StatusUnauthorized, "AR713"},
		{"allowlisted reviewer", Policy{Selectors: []string{"remote"}, Approvers: []string{"alice"}}, include,
			[]lockfile.Approval{rec("include", "shared", digestB, "alice")}, StatusOK, ""},
		{"one reviewer twice counts once", Policy{Selectors: []string{"remote"}, MinApprovers: 2}, include,
			[]lockfile.Approval{rec("include", "shared", digestB, "alice"), rec("include", "shared", digestB, "alice")}, StatusInsufficient, "AR714"},
		{"two reviewers", Policy{Selectors: []string{"remote"}, MinApprovers: 2}, include,
			[]lockfile.Approval{rec("include", "shared", digestB, "alice"), rec("include", "shared", digestB, "bob")}, StatusOK, ""},
		{"exempt wins", Policy{Selectors: []string{"remote"}, Exempt: []string{"include:sh*"}}, include, nil, StatusNotRequired, ""},
		{"kind selector", Policy{Selectors: []string{"kind:hook"}}, hook, nil, StatusMissing, "AR710"},
		{"all selects local items", Policy{Selectors: []string{"all"}}, hook, nil, StatusMissing, "AR710"},
		{"local selects authored items only", Policy{Selectors: []string{"local"}}, include, nil, StatusNotRequired, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange / Act
			got := tt.policy.Evaluate(tt.recs, tt.subject, testNow)

			// Assert
			assert.Equal(t, tt.want, got.Status)
			assert.Equal(t, tt.code, CodeOf(got.Status))
			assert.Equal(t, tt.want != StatusNotRequired, got.Required)
		})
	}
}

func TestEvaluate_StaleNamesTheApprovedDigest(t *testing.T) {
	p := Policy{Selectors: []string{"remote"}}
	s := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}
	got := p.Evaluate([]lockfile.Approval{rec("include", "shared", digestA, "alice")}, s, testNow)
	assert.Equal(t, digestA, got.ApprovedDigest)
	assert.Contains(t, got.Message(), "include:shared")
}

func TestPolicy_McpServerSelectorMatchesTheSettingsItem(t *testing.T) {
	p := Policy{Selectors: []string{"kind:mcp_server"}}
	assert.True(t, p.Requires(Subject{Kind: "settings", ID: "mcp-servers", Class: ClassLocal}))
	assert.False(t, p.Requires(Subject{Kind: "settings", ID: "permissions", Class: ClassLocal}))
}

func TestPolicy_ServedLocalSkillsAreOnlySelectedByKindServed(t *testing.T) {
	local := Subject{Kind: KindServed, ID: "mine", Class: ClassServedLocal}
	assert.False(t, Policy{Selectors: []string{"all"}}.Requires(local))
	assert.False(t, Policy{Selectors: []string{"remote"}}.Requires(local))
	assert.True(t, Policy{Selectors: []string{"kind:served"}}.Requires(local))
	assert.True(t, Policy{Selectors: []string{"remote"}}.Requires(Subject{Kind: KindServed, ID: "theirs", Class: ClassRemote}))
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"skill:acme-internal/*", "skill:acme-internal/deploy", true},
		{"skill:acme-internal/*", "skill:other/deploy", false},
		{"hook:*", "hook:PreToolUse:*:0", true},
		{"include:?hared", "include:shared", true},
		{"*", "", true},
		{"a*b*c", "aXXbYYc", true},
		{"a*b*c", "aXXbYY", false},
		{"", "x", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, globMatch(tt.pattern, tt.name), "%q ~ %q", tt.pattern, tt.name)
	}
}

func TestOrphans(t *testing.T) {
	subs := []Subject{{Kind: "rule", ID: "style", Digest: digestA}}
	recs := []lockfile.Approval{rec("rule", "style", digestA, "a"), rec("rule", "gone", digestA, "a")}
	got := Orphans(recs, subs)
	require.Len(t, got, 1)
	assert.Equal(t, "gone", got[0].ID)
}

func TestResolve(t *testing.T) {
	subs := []Subject{
		{Kind: "skill", Domain: "backend", ID: "deploy"},
		{Kind: "skill", Domain: "frontend", ID: "deploy"},
		{Kind: "rule", ID: "style"},
		{Kind: KindInclude, ID: "shared"},
	}
	tests := []struct {
		ref, want, wantErr string
	}{
		{"skill:backend/deploy", "skill:backend/deploy", ""},
		{"skill:deploy", "", "ambiguous"},
		{"deploy", "", "ambiguous"},
		{"style", "rule:style", ""},
		{"rule:style", "rule:style", ""},
		{"include:shared", "include:shared", ""},
		{"nope", "", "not pinned"},
	}
	for _, tt := range tests {
		got, err := Resolve(subs, tt.ref)
		if tt.wantErr != "" {
			require.Error(t, err, tt.ref)
			assert.Contains(t, err.Error(), tt.wantErr)
			continue
		}
		require.NoError(t, err, tt.ref)
		assert.Equal(t, tt.want, got.Ref())
	}
}

func TestSubjectsOf_CoversRemoteEntriesAndItems(t *testing.T) {
	lock := &lockfile.File{
		Include: []lockfile.Entry{{Name: "shared", Digest: digestA}},
		Skill:   []lockfile.Entry{{Name: "pdf", Digest: digestA}},
		Source:  []lockfile.Entry{{Name: "vendor", Digest: digestA}},
		Served: []lockfile.Entry{{Name: "v-pdf", Digest: digestA, Source: "https://example.org/r"}, {Name: "pinned", Digest: digestA, Ref: "v1"},
			{Name: "mine", Digest: digestA, Source: ".ai-rulez/skills/mine/SKILL.md"}},
	}
	got := SubjectsOf(lock, []lockfile.Item{{Kind: "rule", ID: "style", Digest: digestB}})
	refs := map[string]string{}
	for _, s := range got {
		refs[s.Ref()] = s.Class
	}
	assert.Equal(t, map[string]string{
		"include:shared": ClassRemote, "installed-skill:pdf": ClassRemote, "source:vendor": ClassRemote,
		"served:v-pdf": ClassRemote, "served:pinned": ClassRemote, "served:mine": ClassServedLocal, "rule:style": ClassLocal,
	}, refs)
}

func TestPolicyOf_ReadsGovernance(t *testing.T) {
	cfg := &config.Config{Governance: &config.GovernanceConfig{RequireApproval: []string{"remote"}, MinApprovers: 2, MaxAge: "30d", Enforce: true}}
	p := PolicyOf(cfg)
	assert.True(t, p.Active())
	assert.Equal(t, 2, p.minApprovers())
	assert.Equal(t, 30*24*time.Hour, p.MaxAge)
	assert.True(t, p.Enforce)
	assert.False(t, PolicyOf(&config.Config{}).Active())
	assert.Equal(t, 1, Policy{}.minApprovers())
}

// TestApprovalFollowsTheContentDigest is the property the design rests on: an
// approval stays valid exactly while the digest stays the same. A CRLF-only edit
// keeps the digest (the lock normalises line endings), any other edit changes it,
// and so does the executable bit.
func TestApprovalFollowsTheContentDigest(t *testing.T) {
	digestOf := func(data, mode string) string {
		d, err := contentlock.TreeDigest("skill", []contentlock.Leaf{{Path: "SKILL.md", Mode: mode, Data: []byte(data)}})
		require.NoError(t, err)
		return d
	}
	p := Policy{Selectors: []string{"local"}}
	base := digestOf("line one\nline two\n", contentlock.ModeRegular)
	approved := []lockfile.Approval{rec("skill", "deploy", base, "alice")}
	statusAt := func(digest string) string {
		return p.Evaluate(approved, Subject{Kind: "skill", ID: "deploy", Digest: digest, Class: ClassLocal}, testNow).Status
	}
	tests := []struct {
		name   string
		digest string
		want   string
	}{
		{"unchanged", base, StatusOK},
		{"CRLF only", digestOf("line one\r\nline two\r\n", contentlock.ModeRegular), StatusOK},
		{"whitespace edit", digestOf("line one \nline two\n", contentlock.ModeRegular), StatusStale},
		{"content edit", digestOf("line one\nline three\n", contentlock.ModeRegular), StatusStale},
		{"executable bit flipped", digestOf("line one\nline two\n", contentlock.ModeExecutable), StatusStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, statusAt(tt.digest))
		})
	}
}

func TestExpiredAt(t *testing.T) {
	assert.False(t, ExpiredAt("", testNow))
	assert.False(t, ExpiredAt("2026-10-05", testNow))
	assert.True(t, ExpiredAt("2026-10-04", testNow))
	assert.True(t, ExpiredAt("not a date", testNow))
	assert.False(t, ExpiredAt(strings.Repeat("9", 4)+"-01-01", testNow), "a far future year is a valid date")
}

func TestReviewerIdentityIsCaseAndSpaceInsensitive(t *testing.T) {
	// Arrange
	include := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}
	recs := []lockfile.Approval{
		rec("include", "shared", digestB, "Alice@Example.org"),
		rec("include", "shared", digestB, " alice@example.org "),
	}
	tests := []struct {
		name   string
		policy Policy
		want   string
	}{
		{"allowlist matches any casing", Policy{Selectors: []string{"remote"}, Approvers: []string{"ALICE@example.org"}}, StatusOK},
		{"two casings of one reviewer count once", Policy{Selectors: []string{"remote"}, MinApprovers: 2}, StatusInsufficient},
		{"an outsider is still refused", Policy{Selectors: []string{"remote"}, Approvers: []string{"bob@example.org"}}, StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := tt.policy.Evaluate(recs, include, testNow)

			// Assert
			assert.Equal(t, tt.want, got.Status)
		})
	}
	assert.Equal(t, "alice@example.org", NormalizeReviewer("  Alice@Example.ORG "))
}

func TestServedClass_IsOneRuleForTheLockAndTheServer(t *testing.T) {
	tests := []struct {
		name                string
		source, ref, commit string
		want                string
	}{
		{"authored in the project", "skills/mine", "", "", ClassServedLocal},
		{"names a ref", "skills/mine", "v1", "", ClassRemote},
		{"names a commit", "skills/mine", "", "abc", ClassRemote},
		{"git source", "https://github.com/o/r.git", "", "", ClassRemote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := ServedClass(tt.source, tt.ref, tt.commit)
			subs := SubjectsOf(&lockfile.File{Served: []lockfile.Entry{{Name: "s", Source: tt.source, Ref: tt.ref, Commit: tt.commit, Digest: digestA}}}, nil)

			// Assert
			assert.Equal(t, tt.want, got)
			require.Len(t, subs, 1)
			assert.Equal(t, got, subs[0].Class)
		})
	}
}

func TestGovernanceLockProblem_FailsClosedUnderEnforce(t *testing.T) {
	pinned := &lockfile.File{Item: []lockfile.Item{{Kind: "rule", ID: "a", Digest: digestA}}}
	tests := []struct {
		name   string
		policy Policy
		lock   *lockfile.File
		want   bool
	}{
		{"enforced, no lock", Policy{Selectors: []string{"remote"}, Enforce: true}, nil, true},
		{"enforced, lock without pins", Policy{Selectors: []string{"remote"}, Enforce: true}, &lockfile.File{}, true},
		{"enforced, pinned lock", Policy{Selectors: []string{"remote"}, Enforce: true}, pinned, false},
		{"not enforced, no lock", Policy{Selectors: []string{"remote"}}, nil, false},
		{"enforce without selectors requires nothing", Policy{Enforce: true}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			msg := tt.policy.LockProblem(tt.lock)

			// Assert
			assert.Equal(t, tt.want, msg != "", msg)
			if tt.want {
				assert.Contains(t, msg, CodeMissing)
			}
		})
	}
}

func TestEvaluate_UnknownAssuranceNeverCounts(t *testing.T) {
	// Arrange
	include := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}
	forged := rec("include", "shared", digestB, "alice", func(a *lockfile.Approval) { a.Assurance = "signed" })
	policy := Policy{Selectors: []string{"remote"}}

	// Act
	got := policy.Evaluate([]lockfile.Approval{forged}, include, testNow)

	// Assert
	assert.Equal(t, StatusMissing, got.Status, "nothing here can verify a signature, so it is no approval")
}

func TestEvaluate_RecordedNamesReviewersOfFailingRecords(t *testing.T) {
	// Arrange
	include := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}
	policy := Policy{Selectors: []string{"remote"}}

	// Act
	expired := policy.Evaluate([]lockfile.Approval{rec("include", "shared", digestB, "Alice", expires("2020-01-01"))}, include, testNow)

	// Assert
	assert.Equal(t, StatusExpired, expired.Status)
	assert.Empty(t, expired.Reviewers)
	assert.Equal(t, []string{"alice"}, expired.Recorded)
}
