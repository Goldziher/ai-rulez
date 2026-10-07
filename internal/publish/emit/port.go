package emit

import (
	"regexp"
	"strings"

	"github.com/samber/oops"
)

const (
	// portDefaultBlueprint is the blueprint identifier used when the operator names none.
	portDefaultBlueprint = "agent_skill"
	portIdentifierMax    = 100
)

var (
	portBlueprintPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
	portUnsafe           = regexp.MustCompile(`[^A-Za-z0-9_.@+:=/-]+`)
)

// portEntity is the body of POST /v1/blueprints/{blueprint}/entities
// (https://docs.port.io/api-reference/create-an-entity/, checked 2026-10-06):
// identifier, title, icon, team, properties and relations. The blueprint is the
// operator's; the property names below are a proposal the operator maps onto it.
type portEntity struct {
	Identifier string         `json:"identifier"`
	Title      string         `json:"title"`
	Properties map[string]any `json:"properties"`
	Relations  map[string]any `json:"relations"`
}

type portIndexEntry struct {
	File       string `json:"file"`
	Identifier string `json:"identifier"`
	// Request is the call that creates or updates the entity; the file is its body.
	Request string `json:"request"`
}

type portIndex struct {
	Blueprint string           `json:"blueprint"`
	Entities  []portIndexEntry `json:"entities"`
}

// port writes one catalog entity per plugin and per skill. It never calls Port:
// index.json lists the request each file is the body of.
type port struct{}

func init() { register(port{}) }

func (port) Name() string   { return "port" }
func (port) Status() string { return StatusExperimental }

func (port) Emit(in Input) ([]File, []Finding, error) {
	blueprint := in.Options["blueprint"]
	if blueprint == "" {
		blueprint = portDefaultBlueprint
	}
	if !portBlueprintPattern.MatchString(blueprint) {
		return nil, nil, oops.Errorf("port: %q is not a blueprint identifier", blueprint)
	}
	var (
		files    []File
		findings []Finding
		idx      = portIndex{Blueprint: blueprint}
	)
	add := func(e portEntity) error {
		e.Identifier = portIdentifier(e.Identifier)
		if e.Identifier == "" {
			return oops.Errorf("port: %q cannot be turned into an entity identifier", e.Title)
		}
		data, err := jsonBytes(e)
		if err != nil {
			return err
		}
		file := "entities/" + e.Identifier + ".json"
		files = append(files, File{Path: file, Data: data})
		idx.Entities = append(idx.Entities, portIndexEntry{
			File: file, Identifier: e.Identifier,
			Request: "POST /v1/blueprints/" + blueprint + "/entities?upsert=true",
		})
		return nil
	}
	for i := range in.Plugins {
		p := &in.Plugins[i]
		props := map[string]any{
			"kind": "plugin", keyVersion: p.Version, keyDescription: p.Description,
			"runtimes": nonNil(p.Runtimes), "bundle": p.BundleFile, "bundle_digest": p.BundleDigest,
			"repository": in.Repo, "commit": in.Commit, "lock_tree": in.LockTree,
		}
		if err := add(portEntity{Identifier: p.Name, Title: p.Name, Properties: props, Relations: map[string]any{}}); err != nil {
			return nil, nil, err
		}
		for _, s := range Skills(p.Files) {
			sprops := map[string]any{
				"kind": "skill", keyVersion: p.Version, keyDescription: s.Description,
				"repository": in.Repo, "commit": in.Commit, "plugin": p.Name,
			}
			e := portEntity{Identifier: p.Name + "-" + s.Name, Title: s.Name, Properties: sprops, Relations: map[string]any{}}
			if err := add(e); err != nil {
				return nil, nil, err
			}
		}
	}
	findings = append(findings, Finding{Message: "Port entities carry the properties kind, version, description, repository, commit and digests; add them to blueprint " + blueprint + " or edit the files"})
	index, err := jsonBytes(idx)
	if err != nil {
		return nil, nil, err
	}
	files = append(files, File{Path: "index.json", Data: index})
	out, err := finish(files)
	return out, findings, err
}

// portIdentifier keeps the characters Port accepts in an identifier and caps its length.
func portIdentifier(s string) string {
	s = strings.Trim(portUnsafe.ReplaceAllString(s, "-"), "-")
	if len(s) > portIdentifierMax {
		s = strings.TrimRight(s[:portIdentifierMax], "-")
	}
	return s
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
