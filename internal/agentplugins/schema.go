package agentplugins

import (
	"cmp"
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
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

// specFromID returns the specification version whose identifier, as built by
// idFor (PluginSchemaID or MCPSchemaID), is id.
func specFromID(id string, idFor func(string) string) (string, bool) {
	for _, spec := range Specs {
		if idFor(spec) == id {
			return spec, true
		}
	}
	return "", false
}

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
	// variants are the mcp.json server variants ($defs) keyed by their type
	// const. The server oneOf is a closed union discriminated by type, so an
	// entry is valid exactly when it matches the variant its type names; the
	// variant alone yields readable errors.
	variants map[string]*jsonschema.Schema
}

var variantDefs = map[string]string{
	typeStdio:          "stdioServer",
	typeStreamableHTTP: "streamableHttpServer",
	typeSSE:            "sseServer",
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
	compiler := jsonschema.NewCompiler()
	plugin, err := compileVendored(compiler, spec, "plugin")
	if err != nil {
		return nil, err
	}
	mcp, err := compileVendored(compiler, spec, "mcp")
	if err != nil {
		return nil, err
	}
	set := &schemaSet{plugin: plugin, mcp: mcp, variants: map[string]*jsonschema.Schema{}}
	for typ, def := range variantDefs {
		ref := fmt.Sprintf(`{"$ref":%q}`, MCPSchemaID(spec)+"#/$defs/"+def)
		if set.variants[typ], err = compiler.Compile([]byte(ref)); err != nil {
			return nil, oops.With("spec", spec, "def", def).Wrapf(err, "compile MCP server variant")
		}
	}
	schemaCache[spec] = set
	return set, nil
}

func compileVendored(compiler *jsonschema.Compiler, spec, kind string) (*jsonschema.Schema, error) {
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
	compiled, err := compiler.Compile(data)
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

// wrapperKeywords report that a subschema failed without saying why; the
// leaf errors below them carry the reason.
var wrapperKeywords = []string{"$ref", "properties", "oneOf", "anyOf", "allOf", "not", "schema", "items"}

var quotedList = regexp.MustCompile(`'[^']*'(?:, '[^']*')+`)

// schemaErrors validates doc (decoded JSON) and returns the leaf violations as
// sorted "location: message" strings; nil means valid. The validator lists
// properties in map order, so quoted lists are sorted to keep messages
// deterministic.
func schemaErrors(s *jsonschema.Schema, doc any) []string {
	res := s.Validate(doc)
	if res.IsValid() {
		return nil
	}
	seen := map[string]bool{}
	collectErrors(res, "", seen, true)
	if len(seen) == 0 {
		collectErrors(res, "", seen, false)
	}
	out := slices.Sorted(maps.Keys(seen))
	if len(out) == 0 {
		out = []string{"/: does not match the schema"}
	}
	return out
}

func collectErrors(r *jsonschema.EvaluationResult, base string, into map[string]bool, leavesOnly bool) {
	loc := base + r.InstanceLocation
	for kw, e := range r.Errors {
		if leavesOnly && slices.Contains(wrapperKeywords, kw) {
			continue
		}
		msg := quotedList.ReplaceAllStringFunc(e.Error(), func(list string) string {
			items := strings.Split(list, ", ")
			slices.Sort(items)
			return strings.Join(items, ", ")
		})
		into[cmp.Or(loc, "/")+": "+msg] = true
	}
	for _, d := range r.Details {
		collectErrors(d, loc, into, leavesOnly)
	}
}
