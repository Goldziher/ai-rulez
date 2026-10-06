package lockfile

import "sort"

// A role's rendered outputs are pinned as one aggregate [[output]] entry keyed by
// the role (role = "dev", digest = "sha256:..."), next to the default outputs,
// which carry a path and no role. The lock holds one digest per role; the
// per-file digests are computed on demand for `lock --diff`.

// DefaultOutputs returns the output pins of the default rendering (no role).
func (f *File) DefaultOutputs() []OutputPin {
	if f == nil {
		return nil
	}
	var out []OutputPin
	for _, o := range f.Output {
		if o.Role == "" {
			out = append(out, o)
		}
	}
	return out
}

// RoleOutputs returns the role output pins, sorted by role.
func (f *File) RoleOutputs() []OutputPin {
	if f == nil {
		return nil
	}
	var out []OutputPin
	for _, o := range f.Output {
		if o.Role != "" {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out
}

// RoleOutput returns the output digest pinned for role, and whether there is one.
func (f *File) RoleOutput(role string) (string, bool) {
	for _, o := range f.RoleOutputs() {
		if o.Role == role {
			return o.Digest, true
		}
	}
	return "", false
}

// SetRoleOutputs replaces every role output pin with pins.
func (f *File) SetRoleOutputs(pins []OutputPin) {
	kept := f.DefaultOutputs()
	f.Output = append(kept, pins...)
}
