package schema_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/schema"
)

func TestHermesDependenciesSchemas(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "requirements", value: `["example-tool[cli]>=0.1,<1", "httpx; python_version >= '3.11'"]`},
		{name: "empty list", value: "[]"},
		{name: "not array", value: `"example-tool"`, wantErr: true},
		{name: "non-string item", value: "[1]", wantErr: true},
		{name: "empty item", value: `[""]`, wantErr: true},
		{name: "blank item", value: `["   "]`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "[plugin]\nname = \"example\"\nversion = \"1.2.3\"\ndescription = \"Example.\"\n" +
				"[plugin.hermes]\ndependencies = " + tt.value + "\n"
			for _, local := range []bool{false, true} {
				var err error
				if local {
					err = schema.ValidateLocalFile(writeTOML(t, body))
				} else {
					err = schema.ValidateFile(writeTOML(t, "version = \"5.0\"\nname = \"example\"\n"+body))
				}
				if tt.wantErr {
					require.Error(t, err, "local=%v", local)
				} else {
					require.NoError(t, err, "local=%v", local)
				}
			}
		})
	}
}
