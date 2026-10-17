package includes

import (
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// extraWant is a lock want recorded while resolving, with the origin the lock
// entry carries.
type extraWant struct {
	want   lockfile.Want
	origin string
}

// addWant records that a resolution fetched or checked a source the lock should
// cover, so BuildLock writes it even though the configuration alone cannot name
// it (a marketplace plugin's own repository).
func (s *resolutionState) addWant(baseDir string, w lockfile.Want, origin string) {
	s.mu.Lock()
	s.extra[observedKey(baseDir, w.Kind, w.Name)] = extraWant{want: w, origin: origin}
	s.mu.Unlock()
}

// recordDir remembers the directory a tree was read from.
func (s *resolutionState) recordDir(baseDir, kind, name, dir string) {
	s.mu.Lock()
	s.dirs[observedKey(baseDir, kind, name)] = dir
	s.mu.Unlock()
}

func (s *resolutionState) dirFor(baseDir, kind, name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dirs[observedKey(baseDir, kind, name)]
}

func (s *resolutionState) originFor(baseDir, kind, name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.extra[observedKey(baseDir, kind, name)].origin
}

// extraWants lists the recorded wants of baseDir's load in a stable order.
func (s *resolutionState) extraWants(baseDir string) []lockfile.Want {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []lockfile.Want
	for key := range s.extra {
		if len(key) > len(baseDir) && key[:len(baseDir)] == baseDir && key[len(baseDir)] == 0 {
			out = append(out, s.extra[key].want)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}
