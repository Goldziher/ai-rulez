package schema_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/kaptinlin/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func validateEvalCase(t *testing.T, doc string) bool {
	t.Helper()
	raw, err := os.ReadFile("eval-case.schema.json")
	require.NoError(t, err)
	compiled, err := jsonschema.NewCompiler().Compile(raw)
	require.NoError(t, err)
	var data any
	require.NoError(t, yaml.Unmarshal([]byte(doc), &data))
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	return compiled.Validate(encoded).IsValid()
}

func TestEvalCaseSchema(t *testing.T) {
	valid := []string{
		"cases:\n  - id: a\n    prompt: hi\n    expect_trigger: true\n",
		"prompt: hi\nexpect_trigger: false\n",
		"prompt_file: p.md\nexpect_trigger: true\nnear_miss: [x]\nassertions:\n  - {type: contains, value: ok}\n  - {type: command_exit, command: 'true', exit_code: 1}\nfiles:\n  - {path: a.txt, content: x}\nrubric: good\nrubric_min_score: 0.5\ntags: [smoke]\n",
	}
	for _, doc := range valid {
		assert.True(t, validateEvalCase(t, doc), doc)
	}
	invalid := []string{
		"cases:\n  - prompt: hi\n    expect_trigger: true\n", // id required in a list
		"cases:\n  - id: a\n    prompt: hi\n",                // expect_trigger required
		"prompt: hi\n",                                       // single case needs expect_trigger
		"cases:\n  - id: a\n    prompt: hi\n    expect_trigger: true\n    nope: 1\n",
		"prompt: hi\nexpect_trigger: true\nassertions:\n  - {type: equals, value: x}\n",
		"prompt: hi\nexpect_trigger: true\nfiles:\n  - {path: /etc/passwd}\n",
		"prompt: hi\nexpect_trigger: true\nrubric_min_score: 2\n",
		"prompt: hi\nexpect_trigger: true\ncases:\n  - {id: a, prompt: p, expect_trigger: true}\n",
		"schema_version: 2\ncases:\n  - {id: a, prompt: p, expect_trigger: true}\n",
	}
	for _, doc := range invalid {
		assert.False(t, validateEvalCase(t, doc), doc)
	}
}
