package approval

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// LoadOwnerSetAt is LoadOwnerSet reading CODEOWNERS from the git revision rev
// (normally the merge base of the reviewed range) instead of the working tree.
// A change under review cannot then authorize its own approvals by editing
// CODEOWNERS.
func LoadOwnerSetAt(baseDir, configDir, from, rev string) *OwnerSet {
	return loadOwnerSet(baseDir, configDir, from, func(root, rel string) ([]byte, error) {
		data, found, err := workspace.ReadFileAt(context.Background(), root, rev, rel, nil)
		if err != nil {
			return nil, oops.With("rev", rev, "path", rel).Wrapf(err, "read %s at %s", rel, rev)
		}
		if !found {
			return nil, oops.Errorf("%s does not exist at %s", rel, rev)
		}
		return data, nil
	})
}

// OwnershipChanges lists what decides who may approve and differs between the
// merge base rev and the working tree: the CODEOWNERS file that approvers_from
// names (when set) and the [governance] table. A change that edits them is
// reviewed by the people it would empower, so each is an AR716 finding.
func OwnershipChanges(cfg *config.Config, rev string) ([]string, error) {
	git := gitutil.Git{}
	top := git.TopLevel(cfg.ConfigDir)
	if top == "" {
		return nil, oops.Errorf("%s is not inside a git work tree", cfg.ConfigDir)
	}
	var out []string
	if g := cfg.Governance; g != nil && g.ApproversFrom != "" {
		candidates := codeownersLocations
		if g.ApproversFrom != config.ApproversFromCodeowners {
			candidates = []string{filepath.ToSlash(g.ApproversFrom)}
		}
		for _, rel := range candidates {
			head, headErr := readOwnersFile(top, rel)
			base, baseOK, baseErr := workspace.ReadFileAt(context.Background(), top, rev, rel, nil)
			if baseErr != nil {
				return nil, oops.With("rev", rev, "path", rel).Wrapf(baseErr, "read %s at %s", rel, rev)
			}
			if (headErr == nil) != baseOK || (headErr == nil && !bytes.Equal(head, base)) {
				out = append(out, "the CODEOWNERS file "+rel+" changed since "+short(rev)+"; approvers_from is read from the base, so a change cannot authorize its own approvals")
			}
		}
	}
	rel := gitutil.RepoRelative(top, filepath.Join(cfg.ConfigDir, "config.toml"))
	if rel == "" {
		return out, nil
	}
	headRaw, _ := os.ReadFile(filepath.Join(top, filepath.FromSlash(rel))) //nolint:gosec,errcheck // a missing file is an empty table
	baseRaw, _, err := workspace.ReadFileAt(context.Background(), top, rev, rel, nil)
	if err != nil {
		return nil, oops.With("rev", rev, "path", rel).Wrapf(err, "read %s at %s", rel, rev)
	}
	if !reflect.DeepEqual(governanceTable(headRaw), governanceTable(baseRaw)) {
		out = append(out, "the [governance] table of "+rel+" changed since "+short(rev)+"; the policy that authorizes approvals is part of the reviewed change")
	}
	return out, nil
}

// governanceTable is the [governance] table of a config.toml; nil when there is
// none or the file does not parse.
func governanceTable(raw []byte) any {
	var doc map[string]any
	if strings.TrimSpace(string(raw)) == "" || toml.Unmarshal(raw, &doc) != nil {
		return nil
	}
	return doc["governance"]
}
