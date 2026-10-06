package emit

import (
	"regexp"

	"github.com/samber/oops"
)

// cursorIndexPath is where Cursor reads a multi-plugin repository's marketplace
// manifest (https://cursor.com/docs/reference/plugins, checked 2026-10-06).
const cursorIndexPath = ".cursor-plugin/marketplace.json"

// cursorNamePattern is the kebab-case Cursor documents for marketplace and
// plugin identifiers.
var cursorNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

type cursorOwner struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type cursorMeta struct {
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
}

type cursorEntry struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	Category    string   `json:"category,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

type cursorIndex struct {
	Name     string        `json:"name"`
	Owner    cursorOwner   `json:"owner"`
	Metadata *cursorMeta   `json:"metadata,omitempty"`
	Plugins  []cursorEntry `json:"plugins"`
}

// cursorTeam writes the tree a team marketplace imports: a Cursor marketplace
// manifest at the repository root and each plugin below plugins/<name>, ready to
// commit to the repository the team marketplace points at. Cursor documents no
// way to pin a git ref in the manifest, so the release is the commit that
// carries this tree; the sources are relative paths.
type cursorTeam struct{}

func init() { register(cursorTeam{}) }

func (cursorTeam) Name() string   { return "cursor-team-marketplace" }
func (cursorTeam) Status() string { return StatusVerified }

func (cursorTeam) Emit(in Input) ([]File, []Finding, error) {
	if len(in.Plugins) == 0 {
		return nil, nil, oops.Errorf("cursor-team-marketplace: nothing to emit, the bundle holds no plugin")
	}
	name := in.Market.Name
	if name == "" {
		name = in.Plugins[0].Name
	}
	if !cursorNamePattern.MatchString(name) {
		return nil, nil, oops.Hint("set a lower-case [marketplace] name").Errorf("cursor-team-marketplace: marketplace name %q is not kebab-case", name)
	}
	var (
		files    []File
		findings []Finding
	)
	owner := cursorOwner{Name: in.Market.OwnerName, Email: in.Market.OwnerEmail}
	if owner.Name == "" {
		owner.Name = name
		findings = append(findings, Finding{Message: "the marketplace has no owner; using its name. Set [marketplace.owner] or [plugin.author]"})
	}
	idx := cursorIndex{Name: name, Owner: owner}
	if in.Market.Description != "" || (len(in.Plugins) == 1 && in.Version != "") {
		idx.Metadata = &cursorMeta{Description: in.Market.Description}
		if len(in.Plugins) == 1 {
			idx.Metadata.Version = in.Plugins[0].Version
		}
	}
	for i := range in.Plugins {
		p := &in.Plugins[i]
		if !cursorNamePattern.MatchString(p.Name) {
			return nil, nil, oops.Hint("Cursor plugin names are lower-case").Errorf("cursor-team-marketplace: plugin name %q is not kebab-case", p.Name)
		}
		if !hasFile(p.Files, ".cursor-plugin/plugin.json") {
			return nil, nil, oops.Hint("publish with the cursor runtime in [plugin] runtimes or --runtime").
				Errorf("cursor-team-marketplace: plugin %q has no .cursor-plugin/plugin.json", p.Name)
		}
		dir := "plugins/" + p.Name
		idx.Plugins = append(idx.Plugins, cursorEntry{
			Name: p.Name, Source: dir, Description: p.Description, Version: p.Version,
			Category: p.Category, Keywords: p.Keywords,
		})
		for _, f := range p.Files {
			files = append(files, File{Path: dir + "/" + f.Path, Data: f.Data})
		}
	}
	data, err := jsonBytes(idx)
	if err != nil {
		return nil, nil, err
	}
	files = append(files, File{Path: cursorIndexPath, Data: data})
	out, err := finish(files)
	return out, findings, err
}

func hasFile(files []File, p string) bool {
	for _, f := range files {
		if f.Path == p {
			return true
		}
	}
	return false
}
