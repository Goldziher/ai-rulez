package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRepairWorkflowTemplate_IsAScheduledGitHubWorkflow(t *testing.T) {
	t.Parallel()
	// Arrange
	text, ok := Template(RepairWorkflow)
	require.True(t, ok)

	// Act
	var wf struct {
		Name string `yaml:"name"`
		On   struct {
			Schedule         []map[string]string `yaml:"schedule"`
			WorkflowDispatch any                 `yaml:"workflow_dispatch"`
			PullRequest      any                 `yaml:"pull_request"`
		} `yaml:"on"`
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Steps []map[string]any `yaml:"steps"`
		} `yaml:"jobs"`
	}
	err := yaml.Unmarshal([]byte(text), &wf)

	// Assert
	require.NoError(t, err)
	require.Len(t, wf.On.Schedule, 1)
	assert.NotEmpty(t, wf.On.Schedule[0]["cron"])
	assert.NotNil(t, wf.On.WorkflowDispatch)
	assert.Nil(t, wf.On.PullRequest, "a fork's pull request must never run a job that holds the secrets")
	assert.Equal(t, map[string]string{"contents": "write", "pull-requests": "write"}, wf.Permissions)
	require.Contains(t, wf.Jobs, "repair")
	assert.GreaterOrEqual(t, len(wf.Jobs["repair"].Steps), 4)
}

func TestRepairWorkflowTemplate_IsListedAsATemplateNotAnAdapter(t *testing.T) {
	t.Parallel()
	found := false
	for _, info := range List() {
		if info.Name == RepairWorkflow {
			found = true
			assert.False(t, info.Runnable)
			assert.Contains(t, info.Summary, "workflow")
		}
	}
	assert.True(t, found)
	assert.False(t, IsRunnable(RepairWorkflow))
}
