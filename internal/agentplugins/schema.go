package agentplugins

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/kaptinlin/jsonschema"
	"github.com/samber/oops"
)

// Supported Agent Plugins specification versions. 1.0.0 is published; 1.1.0
// is the working draft. Both share one schema contract apart from the
// identifiers.
const (
	Spec100     = "1.0.0"
	Spec110     = "1.1.0"
	DefaultSpec = Spec100
)

// Specs lists every supported specification version, oldest first.
var Specs = []string{Spec100, Spec110}

// The vendored schemas under schemas/<version>/ are byte-identical copies of
// the official schemas at this commit of the specification repository. The
// schema tests pin their SHA-256 digests.
const (
	SchemaSourceRepo   = "https://github.com/agentplugins/agent-plugins-spec"
	SchemaSourceCommit = "ff8ab5e392cc87bd88d87c060815a87490e51003"
)

const schemaBase = "https://agent-plugins.org/schemas/"

//go:embed schemas
var schemaFS embed.FS

// PluginSchemaID is the canonical plugin.json $schema identifier for spec.
func PluginSchemaID(spec string) string { return schemaBase + spec + "/plugin.schema.json" }

// MCPSchemaID is the canonical mcp.json $schema identifier for spec.
func MCPSchemaID(spec string) string { return schemaBase + spec + "/mcp.schema.json" }

// SupportedSpec reports whether spec is a supported specification version.
func SupportedSpec(spec string) bool { return slices.Contains(Specs, spec) }

// upstreamNamePattern is the official manifest name pattern. Its lookahead is
// not valid RE2, so the schema validator cannot compile it. re2NamePattern is
// the equivalent regular expression without a lookahead: alphanumeric runs
// separated by alternating runs of '-' and '.', which forbids "--" and "..".
// TestRE2NamePatternAgreesWithTheSpecRules proves the two agree.
const (
	upstreamNamePattern = `^(?!.*(?:--|\.\.))[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`
	re2NamePattern      = `^[a-z0-9]+(?:(?:-(?:\.-)*\.?|\.(?:-\.)*-?)[a-z0-9]+)*$`
)

type schemaSet struct {
	plugin *jsonschema.Schema
	mcp    *jsonschema.Schema
}

var (
	schemaMu    sync.Mutex
	schemaCache = map[string]*schemaSet{}
)

// schemasFor compiles (once) the vendored schemas of spec.
func schemasFor(spec string) (*schemaSet, error) {
	if !SupportedSpec(spec) {
		return nil, oops.With("spec", spec).Errorf("unsupported Agent Plugins version %q (supported: %s)",
			spec, strings.Join(Specs, ", "))
	}
	schemaMu.Lock()
	defer schemaMu.Unlock()
	if set, ok := schemaCache[spec]; ok {
		return set, nil
	}
	plugin, err := compileVendored(spec, "plugin")
	if err != nil {
		return nil, err
	}
	mcp, err := compileVendored(spec, "mcp")
	if err != nil {
		return nil, err
	}
	set := &schemaSet{plugin: plugin, mcp: mcp}
	schemaCache[spec] = set
	return set, nil
}

func compileVendored(spec, kind string) (*jsonschema.Schema, error) {
	file := "schemas/" + spec + "/" + kind + ".schema.json"
	data, err := schemaFS.ReadFile(file)
	if err != nil {
		return nil, oops.With("file", file).Wrapf(err, "read vendored schema")
	}
	if kind == "plugin" {
		if data, err = substituteNamePattern(data); err != nil {
			return nil, oops.With("file", file).Wrap(err)
		}
	}
	compiled, err := jsonschema.NewCompiler().Compile(data)
	if err != nil {
		return nil, oops.With("file", file).Wrapf(err, "compile vendored schema")
	}
	return compiled, nil
}

// substituteNamePattern replaces the official name pattern with its RE2
// equivalent. It fails when the vendored pattern is not the one the
// substitute was proven against, so a re-vendored schema forces a review.
func substituteNamePattern(data []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, oops.Wrapf(err, "decode plugin schema")
	}
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		return nil, oops.Errorf("plugin schema has no properties")
	}
	name, ok := props["name"].(map[string]any)
	if !ok || name["pattern"] != upstreamNamePattern {
		return nil, oops.Errorf("plugin schema name pattern changed upstream; re-derive re2NamePattern")
	}
	name["pattern"] = re2NamePattern
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, oops.Wrapf(err, "encode plugin schema")
	}
	return out, nil
}

// schemaErrors validates doc (decoded JSON) and returns the violations as
// sorted "location: message" strings; nil means valid.
func schemaErrors(s *jsonschema.Schema, doc any) []string {
	res := s.Validate(doc)
	if res.IsValid() {
		return nil
	}
	detailed := res.DetailedErrors()
	out := make([]string, 0, len(detailed))
	for loc, msg := range detailed {
		if loc == "" {
			loc = "/"
		}
		out = append(out, fmt.Sprintf("%s: %s", loc, msg))
	}
	if len(out) == 0 {
		out = append(out, "/: does not match the schema")
	}
	sort.Strings(out)
	return out
}
