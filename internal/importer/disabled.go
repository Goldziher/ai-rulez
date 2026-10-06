package importer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// Imported hooks and allow rules are not active until the user says so: convert
// writes them to config.toml as a commented block (or, with --enable-hooks and
// --enable-permissions, as live TOML). A comment is inert to every ai-rulez
// command, so nothing runs or widens until a person has read it and removed the
// leading "# ".

const disabledMarker = "# Imported by `ai-rulez convert` and DISABLED: review, then uncomment or rerun convert with --enable-hooks / --enable-permissions."

// disabledSet is the part of the plan that stays inert.
type disabledSet struct {
	Hooks []config.HookGroup
	Allow []string
}

func (d disabledSet) empty() bool { return len(d.Hooks) == 0 && len(d.Allow) == 0 }

// splitEnabled decides what goes live. Hooks run commands, so they are disabled
// unless enable.hooks is set. Of the permission rules only allow is held back:
// an imported allow would widen every harness (the rule was written for one
// tool), while ask and deny can only narrow what a harness may do.
func splitEnabled(plan *Plan, hooks, permissions bool) (live config.Config, off disabledSet) {
	if hooks {
		live.Hooks = plan.Hooks
	} else {
		off.Hooks = plan.Hooks
	}
	perms := config.Permissions{Ask: plan.Permissions.Ask, Deny: plan.Permissions.Deny}
	if permissions {
		perms.Allow = plan.Permissions.Allow
	} else {
		off.Allow = plan.Permissions.Allow
	}
	if !perms.IsEmpty() {
		live.Permissions = &perms
	}
	return live, off
}

// withoutExisting drops from d what the existing config already declares, so a
// second run adds nothing.
func (d *disabledSet) withoutExisting(existing *config.Config) {
	have := map[string]bool{}
	for _, g := range existing.Hooks {
		have[hookGroupKey(g)] = true
	}
	var hooks []config.HookGroup
	for _, g := range d.Hooks {
		if !have[hookGroupKey(g)] {
			hooks = append(hooks, g)
		}
	}
	d.Hooks = hooks
	if existing.Permissions != nil {
		known := map[string]bool{}
		for _, r := range existing.Permissions.Allow {
			known[r] = true
		}
		var allow []string
		for _, r := range d.Allow {
			if !known[r] {
				allow = append(allow, r)
			}
		}
		d.Allow = allow
	}
}

// render returns the commented block, or "" when nothing is held back.
func (d disabledSet) render() (string, error) {
	if d.empty() {
		return "", nil
	}
	var b strings.Builder
	b.WriteString(disabledMarker + "\n")
	if len(d.Hooks) > 0 {
		b.WriteString("# Each hook runs a command on your machine; read every command first.\n")
		doc, err := config.MarshalTOML(&config.Config{Hooks: d.Hooks})
		if err != nil {
			return "", oops.Wrapf(err, "render disabled hooks")
		}
		b.WriteString(commentOut(afterHeader(doc)))
	}
	if len(d.Allow) > 0 {
		doc, err := config.MarshalTOML(&config.Config{Permissions: &config.Permissions{Allow: d.Allow}})
		if err != nil {
			return "", oops.Wrapf(err, "render disabled permission rules")
		}
		if len(d.Hooks) > 0 {
			b.WriteString("#\n")
		}
		b.WriteString("# Allow rules widen what every harness may do; add them to [permissions] only after review.\n")
		b.WriteString(commentOut(strings.TrimPrefix(afterHeader(doc), "[permissions]\n")))
	}
	return b.String(), nil
}

// afterHeader drops the generated header (comments, version, name) that
// MarshalTOML puts in front of the first table.
func afterHeader(doc []byte) string {
	text := string(doc)
	for _, marker := range []string{"[[hooks]]", "[permissions]"} {
		if i := strings.Index(text, marker); i >= 0 {
			return text[i:]
		}
	}
	return ""
}

func commentOut(text string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			b.WriteString("#\n")
			continue
		}
		b.WriteString("# " + line + "\n")
	}
	return b.String()
}

// appendDisabled adds the block to config.toml when it is written and does not
// hold it yet. It returns the file action after the change.
func appendDisabled(files map[string][]byte, block, action string) string {
	cur, ok := files[configTOML]
	if block == "" || !ok || action == "" {
		return action
	}
	if bytes.Contains(cur, []byte(block)) {
		return action
	}
	out := append(bytes.TrimRight(cur, "\n"), '\n', '\n')
	files[configTOML] = append(out, block...)
	if action == ActionUnchanged {
		return ActionMerge
	}
	return action
}

// mergeHooksAndPermissions adds live hooks and permission rules to an existing
// config; entries it already has are kept. added counts what was appended.
func mergeHooksAndPermissions(merged, add *config.Config) (added int) {
	have := map[string]bool{}
	for _, g := range merged.Hooks {
		have[hookGroupKey(g)] = true
	}
	for _, g := range add.Hooks {
		if !have[hookGroupKey(g)] {
			merged.Hooks = append(merged.Hooks, g)
			added++
		}
	}
	if add.Permissions == nil {
		return added
	}
	if merged.Permissions == nil {
		merged.Permissions = &config.Permissions{}
	}
	for _, list := range []struct{ dst, src *[]string }{
		{&merged.Permissions.Allow, &add.Permissions.Allow},
		{&merged.Permissions.Ask, &add.Permissions.Ask},
		{&merged.Permissions.Deny, &add.Permissions.Deny},
	} {
		known := map[string]bool{}
		for _, r := range *list.dst {
			known[r] = true
		}
		for _, r := range *list.src {
			if !known[r] {
				*list.dst = append(*list.dst, r)
				added++
			}
		}
	}
	return added
}

// reportDisabled records what convert did with hooks and permission rules, so
// the report never leaves a hook unaccounted for.
func reportDisabled(plan *Plan, hooksOn, permsOn bool) {
	if n := len(plan.Hooks); n > 0 {
		if hooksOn {
			plan.add(newFinding(StatusNeedsAction, "(hooks)", "hooks", "config.toml",
				fmt.Sprintf("%d hook group(s) are enabled in config.toml (--enable-hooks); each runs a command on your machine, read them before generating", n)))
		} else {
			plan.add(newFinding(StatusNeedsAction, "(hooks)", "hooks", "config.toml",
				fmt.Sprintf("%d hook group(s) were written as a commented block, not enabled: each runs a command on your machine. Review them, then uncomment or rerun with --enable-hooks", n)))
		}
	}
	if n := len(plan.Permissions.Allow); n > 0 {
		if permsOn {
			plan.add(newFinding(StatusNeedsAction, "(permissions)", "permissions.allow", "config.toml",
				fmt.Sprintf("%d allow rule(s) are enabled in config.toml (--enable-permissions) and apply to every harness; review them", n)))
		} else {
			plan.add(newFinding(StatusNeedsAction, "(permissions)", "permissions.allow", "config.toml",
				fmt.Sprintf("%d allow rule(s) were written as a commented block, not enabled: an allow would apply to every harness. Review them, then uncomment or rerun with --enable-permissions", n)))
		}
	}
}
