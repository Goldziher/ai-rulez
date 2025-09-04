package crud

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// TestMain sets up a dummy config for all tests
var testConfig *config.Config

func setup() {
	testConfig = &config.Config{
		Metadata: config.Metadata{
			Name:    "Test Project",
			Version: "1.0.0",
		},
		Rules: []config.Rule{
			{Name: "Rule 1", Content: "Content 1"},
		},
		MCPServers: []config.MCPServer{
			{Name: "Server 1", Command: "cmd1"},
		},
		Includes: []string{"./common.yaml"},
	}
}

func TestHandleAdd(t *testing.T) {
	setup()
	newRule := &config.Rule{Name: "Rule 2", Content: "Content 2"}

	_, err := HandleAdd(context.Background(), "rules", newRule, testConfig)
	assert.NoError(t, err)
	assert.Len(t, testConfig.Rules, 2)
	assert.Equal(t, "Rule 2", testConfig.Rules[1].Name)
}

func TestHandleGet(t *testing.T) {
	setup()
	result, err := HandleGet(context.Background(), "rules", "Rule 1", testConfig)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "Rule 1")
}

func TestHandleGet_NotFound(t *testing.T) {
	setup()
	_, err := HandleGet(context.Background(), "rules", "Non-existent", testConfig)
	assert.Error(t, err)
}

func TestHandleList(t *testing.T) {
	setup()
	result, err := HandleList(context.Background(), "rules", testConfig)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result.Content[0].(mcp.TextContent).Text, "Rule 1")
}

func TestHandleUpdate(t *testing.T) {
	setup()
	updates := map[string]interface{}{"Content": "Updated Content"}

	_, err := HandleUpdate(context.Background(), "rules", "Rule 1", updates, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Content", testConfig.Rules[0].Content)
}

func TestHandleDelete(t *testing.T) {
	setup()
	_, err := HandleDelete(context.Background(), "rules", "Rule 1", testConfig)
	assert.NoError(t, err)
	assert.Len(t, testConfig.Rules, 0)
}

func TestHandleAddToList(t *testing.T) {
	setup()
	_, err := HandleAddToList(context.Background(), "includes", "./new.yaml", testConfig)
	assert.NoError(t, err)
	assert.Len(t, testConfig.Includes, 2)
	assert.Equal(t, "./new.yaml", testConfig.Includes[1])
}

func TestHandleDeleteFromList(t *testing.T) {
	setup()
	_, err := HandleDeleteFromList(context.Background(), "includes", "./common.yaml", testConfig)
	assert.NoError(t, err)
	assert.Len(t, testConfig.Includes, 0)
}

func TestSingletonGetSetDelete(t *testing.T) {
	setup()
	// Test Get
	getResult, err := HandleGet(context.Background(), "metadata", "", testConfig)
	assert.NoError(t, err)
	assert.Contains(t, getResult.Content[0].(mcp.TextContent).Text, "Test Project")

	// Test Update
	updates := map[string]interface{}{"Name": "New Project Name"}
	_, err = HandleUpdate(context.Background(), "metadata", "", updates, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, "New Project Name", testConfig.Metadata.Name)

	// Test Extends
	_, err = HandleUpdate(context.Background(), "extends", "", map[string]interface{}{"Extends": "./base.yaml"}, testConfig)
	assert.NoError(t, err)
	assert.Equal(t, "./base.yaml", testConfig.Extends)

	_, err = HandleDelete(context.Background(), "extends", "", testConfig)
	assert.NoError(t, err)
	assert.Empty(t, testConfig.Extends)
}
