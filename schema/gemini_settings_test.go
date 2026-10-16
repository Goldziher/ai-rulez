package schema_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

func TestGeminiSettingsSchemas(t *testing.T) {
	for _, test := range []struct {
		name, extra string
		invalid     bool
	}{
		{name: "credential allowlist", extra: `env_var = "OPENAI_API_KEY"
sensitive = true`},
		{name: "unknown secret default", extra: `env_var = "OPENAI_API_KEY"
default = "do-not-store-secrets"`, invalid: true},
		{name: "invalid variable", extra: `env_var = "INVALID-KEY"`, invalid: true},
		{name: "missing variable", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := "[plugin]\nname = \"example\"\nversion = \"1.0.0\"\ndescription = \"Example.\"\n" +
				"[[plugin.gemini.settings]]\nname = \"Provider key\"\ndescription = \"Provider credential.\"\n" + test.extra + "\n"
			for _, local := range []bool{false, true} {
				var err error
				if local {
					err = schema.ValidateLocalFile(writeTOML(t, body))
				} else {
					err = schema.ValidateFile(writeTOML(t, "version = \"5.0\"\nname = \"example\"\n"+body))
				}
				if test.invalid && (!local || test.name != "missing variable") {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}
