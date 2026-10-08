package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

const roleLockConfig = `
[[roles]]
name = "dev"
pin = true
[roles.skill_mode]
deploy = "%s"

[[roles]]
name = "ops"
[roles.skill_mode]
deploy = "name-only"
`

func roleLockProject(t *testing.T, mode string) string {
	t.Helper()
	root := lockProject(t, strings.Replace(roleLockConfig, "%s", mode, 1))
	t.Cleanup(func() {
		lockRoles, lockServeRole, generateRole = false, "", ""
	})
	return root
}

func setRoleMode(t *testing.T, root, from, to string) {
	t.Helper()
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), `deploy = "`+from+`"`)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), `deploy = "`+from+`"`, `deploy = "`+to+`"`, 1)), 0o644))
}

func pinnedRoles(t *testing.T, root string) []string {
	t.Helper()
	lock, err := lockfile.Load(filepath.Join(root, ".ai-rulez"))
	require.NoError(t, err)
	var roles []string
	for _, o := range lock.RoleOutputs() {
		assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, o.Digest)
		assert.Empty(t, o.Path, "a role pin is one aggregate digest, not per file")
		roles = append(roles, o.Role)
	}
	return roles
}

func TestLockRoles_SkillModeChangeIsReportedAsRoleOutputDrift(t *testing.T) {
	// Arrange
	root := roleLockProject(t, "off")
	require.Equal(t, 0, writeLockAt("", "", nil))
	require.Equal(t, []string{"dev"}, pinnedRoles(t, root), "only the role with pin = true is pinned")
	var code int
	report, _ := capture(t, func() { code = checkLockAt("") })
	require.Equal(t, 0, code, report)

	// Act: the skill_mode changes, no source file does
	setRoleMode(t, root, "off", "name-only")
	report, _ = capture(t, func() { code = checkLockAt("") })

	// Assert
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, report, "outputs of role dev")
	assert.NotContains(t, report, "ops", "an unpinned role is not compared")
}

func TestLockRoles_UnpinnedRolesLeaveTheLockUnchanged(t *testing.T) {
	// Arrange: the same project without any pin
	root := lockProject(t, `
[[roles]]
name = "ops"
[roles.skill_mode]
deploy = "name-only"
`)
	t.Cleanup(func() { lockRoles, lockServeRole = false, "" })

	// Act
	require.Equal(t, 0, writeLockAt("", "", nil))

	// Assert
	data, err := os.ReadFile(filepath.Join(root, ".ai-rulez", lockfile.FileName))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "role =")
	assert.Empty(t, pinnedRoles(t, root))
}

func TestLockRoles_RolesFlagPinsEveryRoleAndRoleLimitsTheCheck(t *testing.T) {
	// Arrange
	root := roleLockProject(t, "off")
	lockRoles = true
	require.Equal(t, 0, writeLockAt("", "", nil))
	lockRoles = false
	require.Equal(t, []string{"dev", "ops"}, pinnedRoles(t, root))

	// Act: only ops drifts, and the check is limited to dev, then to ops
	setRoleMode(t, root, "name-only", "on")
	var code int
	lockServeRole = "dev"
	report, _ := capture(t, func() { code = checkLockAt("") })
	assert.NotContains(t, report, "outputs of role", "dev did not change (ops' own source did, which is always compared)")
	lockServeRole = "ops"
	report, _ = capture(t, func() { code = checkLockAt("") })

	// Assert
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, report, "outputs of role ops")
}

func TestLockRoles_CheckingAnUnpinnedRoleIsAnError(t *testing.T) {
	// Arrange
	roleLockProject(t, "off")
	require.Equal(t, 0, writeLockAt("", "", nil))
	lockServeRole = "ops"

	// Act
	var code int
	_, report := capture(t, func() { code = checkLockAt("") })

	// Assert
	assert.Equal(t, 1, code)
	assert.Contains(t, report, "not pinned")
}

func TestLockRoles_DiffListsPerFileDigestsAndWritesNothing(t *testing.T) {
	// Arrange
	root := roleLockProject(t, "off")
	require.Equal(t, 0, writeLockAt("", "", nil))
	setRoleMode(t, root, "off", "name-only")
	before, err := os.ReadFile(filepath.Join(root, ".ai-rulez", lockfile.FileName))
	require.NoError(t, err)

	// Act
	lockDiffFlag, lockFormat = true, formatJSON
	t.Cleanup(func() { lockDiffFlag, lockFormat = false, "" })
	var code int
	stdout, _ := capture(t, func() { code = diffLockAt("") })

	// Assert
	require.Equal(t, 0, code)
	validateAgainst(t, "../../schema/lock-diff.schema.json", []byte(stdout))
	assert.Contains(t, stdout, `"kind": "role"`)
	assert.Contains(t, stdout, `"files"`)
	assert.Contains(t, stdout, ".claude/settings.json")
	after, err := os.ReadFile(filepath.Join(root, ".ai-rulez", lockfile.FileName))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	_, statErr := os.Stat(filepath.Join(root, "CLAUDE.md"))
	assert.True(t, os.IsNotExist(statErr), "rendering a role never touches the disk")
}

func TestLockRoles_GenerateLockedRoleComparesThePinnedRole(t *testing.T) {
	tests := []struct {
		name      string
		role      string
		mutate    bool
		wantLines int
	}{
		{name: "pinned role, unchanged", role: "dev"},
		{name: "pinned role, skill_mode changed", role: "dev", mutate: true, wantLines: 1},
		{name: "unpinned role is not compared", role: "ops", mutate: true},
		{name: "no role is not compared", role: "", mutate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := roleLockProject(t, "off")
			require.Equal(t, 0, writeLockAt("", "", nil))
			if tt.mutate {
				setRoleMode(t, root, "off", "name-only")
			}
			cfg, err := loadForLock("")
			require.NoError(t, err)
			generateRole = tt.role

			// Act
			lines, err := verifyLockedSources(cfg)

			// Assert: the role's own source changed too, so count the output lines only
			require.NoError(t, err)
			var outputLines []string
			for _, l := range lines {
				if strings.Contains(l, "outputs of role") {
					outputLines = append(outputLines, l)
				}
			}
			require.Len(t, outputLines, tt.wantLines, "%v", lines)
			if tt.wantLines > 0 {
				assert.Contains(t, outputLines[0], "outputs of role dev")
			}
		})
	}
}

func TestValidateLockFlags_Roles(t *testing.T) {
	tests := []struct {
		name    string
		set     func()
		args    []string
		wantErr string
	}{
		{"roles on write", func() { lockRoles = true }, nil, ""},
		{"role on check", func() { lockServeRole, lockCheck = "dev", true }, nil, ""},
		{"roles with check", func() { lockRoles, lockCheck = true, true }, nil, "--roles pins every role"},
		{"roles with names", func() { lockRoles = true }, []string{"shared"}, "cannot be combined"},
		{"roles with kind", func() { lockRoles, lockKind = true, "include" }, nil, "cannot be combined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { lockRoles, lockServeRole, lockCheck, lockKind = false, "", false, "" })
			tt.set()

			err := validateLockFlags(tt.args)

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLockRoles_PlainLockKeepsRolePinnedWithRolesFlag(t *testing.T) {
	// Arrange: ops is pinned only through --roles
	root := roleLockProject(t, "off")
	lockRoles = true
	require.Equal(t, 0, writeLockAt("", "", nil))
	lockRoles = false
	require.Equal(t, []string{"dev", "ops"}, pinnedRoles(t, root))

	// Act: a plain lock re-pins it
	require.Equal(t, 0, writeLockAt("", "", nil))

	// Assert
	assert.Equal(t, []string{"dev", "ops"}, pinnedRoles(t, root))
}

func TestLockRoles_PlainLockDropsAPinnedRoleRemovedFromConfig(t *testing.T) {
	// Arrange
	root := roleLockProject(t, "off")
	lockRoles = true
	require.Equal(t, 0, writeLockAt("", "", nil))
	lockRoles = false
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	i := strings.Index(string(data), "[[roles]]\nname = \"ops\"")
	require.Positive(t, i)
	require.NoError(t, os.WriteFile(path, data[:i], 0o644))

	// Act
	require.Equal(t, 0, writeLockAt("", "", nil))

	// Assert
	assert.Equal(t, []string{"dev"}, pinnedRoles(t, root))
}

func TestLockRoles_UnpinningARoleInTheConfigIsDriftAndLockDropsThePin(t *testing.T) {
	// Arrange: dev is pinned by its own pin = true
	root := roleLockProject(t, "off")
	require.Equal(t, 0, writeLockAt("", "", nil))
	require.Equal(t, []string{"dev"}, pinnedRoles(t, root))
	path := filepath.Join(root, ".ai-rulez", "config.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "pin = true")
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), "pin = true", "pin = false", 1)), 0o644))

	// Act
	var code int
	report, _ := capture(t, func() { code = checkLockAt("") })

	// Assert: the check names the pin that is no longer wanted, not only the changed role source
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, report, "outputs of role dev")
	assert.Contains(t, report, "no longer pinned")

	// Act: lock drops it
	require.Equal(t, 0, writeLockAt("", "", nil))

	// Assert
	assert.Empty(t, pinnedRoles(t, root))
	report, _ = capture(t, func() { code = checkLockAt("") })
	assert.Equal(t, 0, code, report)
}

func TestLockRoles_RolePinnedWithRolesFlagIsNotReportedRemovedByAPlainCheck(t *testing.T) {
	// Arrange: ops is pinned only through --roles
	root := roleLockProject(t, "off")
	lockRoles = true
	require.Equal(t, 0, writeLockAt("", "", nil))
	lockRoles = false
	require.Equal(t, []string{"dev", "ops"}, pinnedRoles(t, root))

	// Act
	var code int
	report, _ := capture(t, func() { code = checkLockAt("") })

	// Assert
	assert.Equal(t, 0, code, report)
}
