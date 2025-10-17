package integration

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReviewWorkflow tests the iterative review system
func TestReviewWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !agentTestsEnabled() {
		t.Skipf("Skipping agent integration tests (set %s=1 to enable)", agentTestEnvVar)
	}

	agent := getFirstAvailableAgent(t)
	if agent == "" {
		t.Skip("No agents available for testing")
	}

	t.Run("BasicReview", func(t *testing.T) {
		testBasicReview(t, agent)
	})

	t.Run("IterativeReview", func(t *testing.T) {
		testIterativeReview(t, agent)
	})

	t.Run("ReviewThreshold", func(t *testing.T) {
		testReviewThreshold(t, agent)
	})

	t.Run("AutoApproval", func(t *testing.T) {
		testAutoApproval(t, agent)
	})

	t.Run("RequireImprovement", func(t *testing.T) {
		testRequireImprovement(t, agent)
	})
}

// TestMultiAgentReview tests using different agents for enforcement and review
func TestMultiAgentReview(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !agentTestsEnabled() {
		t.Skipf("Skipping agent integration tests (set %s=1 to enable)", agentTestEnvVar)
	}

	// Find at least two available agents
	var agent1, agent2 string
	for _, agent := range testAgents {
		if isAgentAvailable(agent) {
			if agent1 == "" {
				agent1 = agent
			} else if agent2 == "" {
				agent2 = agent
				break
			}
		}
	}

	if agent1 == "" || agent2 == "" {
		t.Skip("Need at least two agents for multi-agent testing")
	}

	t.Run("DifferentAgents", func(t *testing.T) {
		output := runEnforcementRaw(t,
			"--agent", agent1,
			"--review",
			"--review-agent", agent2,
			"--review-iterations", "2",
			"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
			"--include-files", filepath.Join(fixturesPath, "violations/javascript/console_log.js"),
			"--format", "json",
			"--timeout", "30s",
			"--review-timeout", "30s",
		)

		var result struct {
			EnforcementResult
			Metadata map[string]interface{} `json:"metadata"`
		}
		err := json.Unmarshal([]byte(output), &result)
		assert.NoError(t, err, "Should parse JSON output")

		// Check that both agents were used
		assert.Equal(t, agent1, result.AgentUsed, "Primary agent should be %s", agent1)

		if result.Metadata != nil && result.Metadata["review_sessions"] != nil {
			t.Logf("Review completed with agents: %s (enforcement) and %s (review)", agent1, agent2)
		}
	})
}

func testBasicReview(t *testing.T, agent string) {
	output := runEnforcementRaw(t,
		"--agent", agent,
		"--review",
		"--review-iterations", "1",
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(fixturesPath, "violations/javascript/console_log.js"),
		"--format", "json",
		"--timeout", "20s",
		"--review-timeout", "20s",
	)

	var result struct {
		EnforcementResult
		Metadata map[string]interface{} `json:"metadata"`
	}
	err := json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err, "Should parse JSON output")

	// Check if review was performed
	if result.Metadata != nil {
		if reviewSummary, ok := result.Metadata["review_summary"].(map[string]interface{}); ok {
			t.Logf("Review summary: %+v", reviewSummary)
			assert.NotNil(t, reviewSummary["total_sessions"], "Should have review sessions")
		}
	}
}

func testIterativeReview(t *testing.T, agent string) {
	iterations := 3

	output := runEnforcementRaw(t,
		"--agent", agent,
		"--review",
		"--review-iterations", fmt.Sprintf("%d", iterations),
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(fixturesPath, "violations/javascript/console_log.js"),
		"--format", "json",
		"--timeout", "20s",
		"--review-timeout", "30s",
	)

	var result struct {
		EnforcementResult
		Metadata map[string]interface{} `json:"metadata"`
	}
	err := json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err, "Should parse JSON output")

	// Check iterations
	if result.Metadata != nil && result.Metadata["review_summary"] != nil {
		if summary, ok := result.Metadata["review_summary"].(map[string]interface{}); ok {
			if avgIterations, ok := summary["average_iterations"].(float64); ok {
				t.Logf("Average iterations: %.1f", avgIterations)
				assert.True(t, avgIterations >= 1 && avgIterations <= float64(iterations),
					"Iterations should be between 1 and %d", iterations)
			}
		}
	}
}

func testReviewThreshold(t *testing.T, agent string) {
	threshold := 80.0

	output := runEnforcementRaw(t,
		"--agent", agent,
		"--review",
		"--review-threshold", fmt.Sprintf("%.1f", threshold),
		"--review-iterations", "3",
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(fixturesPath, "violations/javascript/console_log.js"),
		"--format", "json",
		"--timeout", "20s",
		"--review-timeout", "20s",
	)

	var result struct {
		EnforcementResult
		Metadata map[string]interface{} `json:"metadata"`
	}
	err := json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err, "Should parse JSON output")

	// Check if threshold was considered
	if result.Metadata != nil && result.Metadata["review_summary"] != nil {
		if summary, ok := result.Metadata["review_summary"].(map[string]interface{}); ok {
			if avgScore, ok := summary["average_score"].(float64); ok {
				t.Logf("Average score: %.1f%% (threshold: %.1f%%)", avgScore, threshold)

				// If approved, score should be above threshold (or auto-approved)
				if approved, ok := summary["approved_sessions"].(float64); ok && approved > 0 {
					t.Logf("Sessions approved with average score: %.1f%%", avgScore)
				}
			}
		}
	}
}

func testAutoApproval(t *testing.T, agent string) {
	output := runEnforcementRaw(t,
		"--agent", agent,
		"--review",
		"--review-auto-approve",
		"--review-threshold", "95", // High threshold
		"--review-iterations", "1",
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(fixturesPath, "violations/javascript/console_log.js"),
		"--format", "json",
		"--timeout", "20s",
		"--review-timeout", "20s",
	)

	var result struct {
		EnforcementResult
		Metadata map[string]interface{} `json:"metadata"`
	}
	err := json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err, "Should parse JSON output")

	// With auto-approve, all sessions should be approved
	if result.Metadata != nil && result.Metadata["review_summary"] != nil {
		if summary, ok := result.Metadata["review_summary"].(map[string]interface{}); ok {
			totalSessions, _ := summary["total_sessions"].(float64)
			approvedSessions, _ := summary["approved_sessions"].(float64)

			t.Logf("Auto-approval: %v/%v sessions approved", approvedSessions, totalSessions)

			// With auto-approve flag, all sessions should be approved
			if totalSessions > 0 {
				assert.Equal(t, totalSessions, approvedSessions,
					"All sessions should be approved with auto-approve flag")
			}
		}
	}
}

func testRequireImprovement(t *testing.T, agent string) {
	output := runEnforcementRaw(t,
		"--agent", agent,
		"--review",
		"--require-improvement",
		"--review-iterations", "3",
		"--config", filepath.Join(fixturesPath, "configs/test_enforcement.yaml"),
		"--include-files", filepath.Join(fixturesPath, "violations/javascript/console_log.js"),
		"--format", "json",
		"--timeout", "20s",
		"--review-timeout", "30s",
	)

	var result struct {
		EnforcementResult
		Metadata map[string]interface{} `json:"metadata"`
	}
	err := json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err, "Should parse JSON output")

	// Check if improvement was required and tracked
	if result.Metadata != nil && result.Metadata["review_sessions"] != nil {
		sessions, ok := result.Metadata["review_sessions"].([]interface{})
		if ok && len(sessions) > 0 {
			t.Logf("Review sessions with require-improvement: %d sessions", len(sessions))

			// Each session should show improvement attempts
			for i, session := range sessions {
				if s, ok := session.(map[string]interface{}); ok {
					iterations, _ := s["iterations"].(float64)
					finalScore, _ := s["final_score"].(float64)
					t.Logf("Session %d: %v iterations, final score: %.1f%%", i+1, iterations, finalScore)
				}
			}
		}
	}
}
