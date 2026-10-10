package commands

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/stretchr/testify/assert"
)

func TestDetectRuntimesRecognizesNativeNPMPackages(t *testing.T) {
	for _, tt := range []struct {
		name  string
		files []publish.File
		want  []string
	}{
		{"Pi", []publish.File{{Path: "package.json", Data: []byte(`{"pi":{"skills":["./.pi/skills"]}}`)}}, []string{"pi"}},
		{"OpenCode", []publish.File{{Path: ".opencode/plugins/demo.js"}}, []string{"opencode"}},
		{"both", []publish.File{{Path: "package.json", Data: []byte(`{"pi":{"skills":[]}}`)},
			{Path: ".opencode/plugins/demo.js"}}, []string{"opencode", "pi"}},
		{"ordinary npm", []publish.File{{Path: "package.json", Data: []byte(`{"name":"demo"}`)}}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.want, detectRuntimes(tt.files)) })
	}
}
