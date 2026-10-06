package publish

import (
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/publish/emit"
)

// Extras asks for the files a release carries besides the archive: a pinned
// marketplace index and emitter output. It is separate from Build so a
// multi-plugin publish can produce one aggregate index and one emitter run for
// all plugins, while each plugin keeps its own dist.
type Extras struct {
	Channel string
	// Pin, PinCommit and PinDirty describe the pinned index; Pin nil skips it.
	Pin       *Pin
	PinCommit string
	PinDirty  bool
	Emit      *EmitRequest
	Plugins   []emit.Plugin
}

// BuildExtras renders the pinned index and the emitters. The files are keyed by
// dist-relative path; warnings are notes for the operator.
func BuildExtras(x Extras) (files map[string][]byte, warnings []string, err error) {
	files = map[string][]byte{}
	if x.Pin != nil {
		if x.PinDirty {
			return nil, nil, newError(CodeSource, ExitGate, "commit the changes first; a pinned index names a commit",
				"the source tree is dirty, so a marketplace pinned to its commit would not hold what is being published")
		}
		data, err := PinIndex(*x.Pin, x.PinCommit)
		if err != nil {
			return nil, nil, err
		}
		files[IndexPath(x.Channel)] = data
	}
	if x.Emit == nil || len(x.Emit.Names) == 0 {
		return files, nil, nil
	}
	names := append([]string(nil), x.Emit.Names...)
	sort.Strings(names)
	prev := ""
	for _, name := range names {
		if name == prev {
			continue
		}
		prev = name
		e, ok := emit.Lookup(name)
		if !ok {
			return nil, nil, newError(CodeConfig, ExitFailed, "known emitters: "+strings.Join(emit.Names(), ", "), "unknown emitter %q", name)
		}
		if e.Status() == emit.StatusExperimental {
			if !x.Emit.Experimental {
				return nil, nil, newError(CodeConfig, ExitFailed, "review the output against your target and pass --experimental",
					"emitter %s is experimental: its format is not verified against vendor documentation", name)
			}
			warnings = append(warnings, CodeExperimental+" emitter "+name+" is experimental: the format is not verified against vendor documentation")
		}
		in := x.Emit.Base
		in.Plugins = x.Plugins
		in.Options = x.Emit.Options[name]
		out, findings, err := e.Emit(in)
		if err != nil {
			return nil, nil, newError(CodeConfig, ExitFailed, "", "emitter %s failed: %v", name, err)
		}
		for _, f := range findings {
			warnings = append(warnings, name+": "+f.Message)
		}
		for _, f := range out {
			dest := EmitDir + "/" + name + "/" + f.Path
			if !ValidPath(dest) {
				return nil, nil, newError(CodeBundleUnsafe, ExitFailed, "", "emitter %s produced the unsafe path %q", name, f.Path)
			}
			if _, dup := files[dest]; dup {
				return nil, nil, newError(CodeBundleUnsafe, ExitFailed, "", "two emitters render to %s", dest)
			}
			files[dest] = f.Data
		}
	}
	return files, warnings, nil
}
