package commands

import (
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
)

// roleOutputFiles renders the outputs of a role the way `lock --roles` pins them
// (nothing is written) and lists them as the files being approved. note explains
// a rendering that failed.
func roleOutputFiles(cfg *config.Config, role string) (files []approvedFile, note string) {
	outputs, err := generator.LockRoleOutputs(cfg, role)
	if err != nil {
		return nil, "the role cannot be rendered: " + err.Error()
	}
	return outputFiles(outputs), ""
}

func outputFiles(outputs []contentlock.Output) []approvedFile {
	files := make([]approvedFile, 0, len(outputs))
	for _, o := range outputs {
		f := approvedFile{Path: o.Path, Size: int64(len(o.Data)), Executable: o.Mode&0o100 != 0}
		if len(o.Data) <= approveMaxFileSize {
			f.Data = o.Data
		}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

// roleOutputDigest renders role and returns the aggregate digest the lock pins
// for it: the digest an approval of the role's outputs is bound to.
func roleOutputDigest(cfg *config.Config, role string) (string, error) {
	outputs, err := generator.LockRoleOutputs(cfg, role)
	if err != nil {
		return "", oops.Wrap(err)
	}
	leaves := make([]contentlock.Leaf, 0, len(outputs))
	for _, o := range outputs {
		leaves = append(leaves, contentlock.Leaf{Path: o.Path, Mode: contentlock.ModeFor(o.Mode), Data: o.Data})
	}
	digest, err := contentlock.TreeDigest("role-output", leaves)
	if err != nil {
		return "", oops.Wrap(err)
	}
	return digest, nil
}

// checkRoleOutputs refuses to approve role outputs whose rendering no longer
// matches the pin: the reviewer was shown the current rendering, and an approval
// binds to the pin, so the two must be the same bytes.
func (e *approveEnv) checkRoleOutputs(subs []approval.Subject) error {
	for _, s := range subs {
		if s.Kind != approval.KindRoleOutput {
			continue
		}
		digest, err := roleOutputDigest(e.cfg, s.ID)
		if err != nil {
			return err
		}
		if digest != s.Digest {
			return oops.Hint("run `ai-rulez lock --roles` to re-pin the role, then approve it").
				Errorf("the outputs of role %s changed since they were pinned: the pin %s is not what is rendered now", safeText(s.ID), shortDigest(s.Digest))
		}
	}
	return nil
}
