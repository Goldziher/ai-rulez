package importer

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// importedSuffix is appended to the name of an item whose file already exists
// with other content when --merge adds beside an existing tree.
const importedSuffix = "-imported"

// maxMergeSuffix bounds the -imported-N search for a free name.
const maxMergeSuffix = 99

// mergeRename makes room for the items of a --merge run: an item whose files
// already exist with other content is renamed (NAME-imported, then
// NAME-imported-2, ...) so nothing existing is touched, and an item that is
// already there byte for byte stays as it is. With keepNames nothing is renamed;
// the collision is left to be reported as a conflict.
func mergeRename(plan *Plan, intoAbs, domain string, keepNames bool) error {
	taken := map[string]bool{}
	for i := range plan.Items {
		taken[plan.Items[i].Rel()] = true
	}
	for i := range plan.Items {
		it := &plan.Items[i]
		differs, err := differsOnDisk(it, intoAbs, domain)
		if err != nil {
			return err
		}
		if !differs || keepNames {
			continue
		}
		orig := it.Name
		renamed, ok := freeName(*it, intoAbs, domain, taken)
		if !ok {
			return oops.Errorf("no free name for %s %q: %s through %s%d all exist", it.Kind, orig, importedSuffix, importedSuffix+"-", maxMergeSuffix)
		}
		delete(taken, it.Rel())
		src := ""
		if len(it.Sources) > 0 {
			src = it.Sources[0]
		}
		oldRel := it.Rel()
		it.Name = renamed
		if it.Kind == KindSkill {
			it.Main = []byte(setFrontmatterName(string(it.Main), renamed))
		}
		taken[it.Rel()] = true
		plan.retarget(oldRel, it.Rel())
		plan.add(newFinding(StatusApproximated, src, litName, it.Rel(),
			fmt.Sprintf("%s already exists with other content and --merge never touches existing files; imported as %s", oldRel, renamed)))
	}
	return nil
}

// retarget points the findings that name an item's old path at its new one.
func (p *Plan) retarget(oldRel, newRel string) {
	for i := range p.Findings {
		if p.Findings[i].Target == oldRel {
			p.Findings[i].Target = newRel
		}
	}
}

// differsOnDisk reports whether any file of the item exists with other content.
func differsOnDisk(it *Item, intoAbs, domain string) (bool, error) {
	for _, f := range it.Files() {
		existing, err := os.ReadFile(filepath.Join(intoAbs, filepath.FromSlash(placed(domain, f.Path))))
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return false, oops.With("path", f.Path).Wrapf(err, "read existing %s", f.Path)
		case !bytes.Equal(existing, f.Data):
			return true, nil
		}
	}
	return false, nil
}

// freeName finds NAME-imported, NAME-imported-2, ... that no planned item uses
// and that is either absent from the disk or already holds this very content
// (a second run reuses the name it chose the first time).
func freeName(it Item, intoAbs, domain string, taken map[string]bool) (string, bool) {
	for n := 1; n <= maxMergeSuffix; n++ {
		candidate := it.Name + importedSuffix
		if n > 1 {
			candidate += "-" + strconv.Itoa(n)
		}
		try := it
		try.Name = candidate
		if try.Kind == KindSkill {
			try.Main = []byte(setFrontmatterName(string(it.Main), candidate))
		}
		if taken[try.Rel()] {
			continue
		}
		if _, err := os.Lstat(filepath.Join(intoAbs, filepath.FromSlash(placed(domain, try.Rel())))); err != nil {
			return candidate, true
		}
		if differs, err := differsOnDisk(&try, intoAbs, domain); err == nil && !differs {
			return candidate, true
		}
	}
	return "", false
}

// applyDelivery sets the delivery of the imported skills: the global default of
// [skills], or the default of one domain when the import goes into a domain.
// Skills already in the project share the global default, which the report says.
func applyDelivery(cfg *config.Config, plan *Plan, delivery, domain string) error {
	if delivery == "" {
		return nil
	}
	d, ok := config.ParseDelivery(delivery)
	if !ok {
		return oops.Hint("Use static, served or both").Errorf("unknown --delivery %q", delivery)
	}
	if !plan.hasSkills() {
		plan.add(newFinding(StatusApproximated, "(project)", "delivery", "",
			"--delivery has no skill to apply to; nothing was written for it"))
		return nil
	}
	if domain != "" {
		cfg.DomainSettings = config.DomainConfigs{domain: {Delivery: string(d)}}
		return nil
	}
	cfg.Skills = &config.SkillsConfig{Delivery: string(d)}
	return nil
}

// hasSkills reports whether the plan imports any shared skill or installed skill.
func (p *Plan) hasSkills() bool {
	if len(p.InstalledSkills) > 0 {
		return true
	}
	for i := range p.Items {
		if p.Items[i].Kind == KindSkill && !p.Items[i].Local {
			return true
		}
	}
	return false
}

// mergeDelivery adds the imported delivery to an existing config. An existing
// value wins; a differing one is reported.
func mergeDelivery(merged, add *config.Config) (added int, notes []mergeNote) {
	if add.Skills != nil {
		switch {
		case merged.Skills == nil || merged.Skills.Delivery == "":
			if merged.Skills == nil {
				merged.Skills = &config.SkillsConfig{}
			}
			merged.Skills.Delivery = add.Skills.Delivery
			added++
		case merged.Skills.Delivery != add.Skills.Delivery:
			notes = append(notes, mergeNote{"skills.delivery",
				"[skills] delivery is " + merged.Skills.Delivery + " in the existing config and was kept; --delivery asked for " + add.Skills.Delivery})
		}
	}
	for name, dc := range add.DomainSettings {
		cur, ok := merged.DomainSettings[name]
		switch {
		case !ok || cur.Delivery == "":
			if merged.DomainSettings == nil {
				merged.DomainSettings = config.DomainConfigs{}
			}
			cur.Delivery = dc.Delivery
			merged.DomainSettings[name] = cur
			added++
		case cur.Delivery != dc.Delivery:
			notes = append(notes, mergeNote{"domains." + name + ".delivery",
				"[domains." + name + "] delivery is " + cur.Delivery + " in the existing config and was kept; --delivery asked for " + dc.Delivery})
		}
	}
	return added, notes
}

// describeCollisions renders the --keep-names error.
func describeCollisions(c []string) string {
	return strings.Join(c, "; ")
}
