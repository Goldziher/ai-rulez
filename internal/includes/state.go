package includes

import (
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// resolutionState is what the fetches of one load resolved to: the commit and
// digest of each source, the tag a version constraint resolved to, and the
// resolution problems. It is attached to the loaded config, so two loads in one
// process keep separate records; nothing here is process-wide, and a refresh of
// one project never wipes another's.
type resolutionState struct {
	mu       sync.Mutex
	observed map[string]observed
	tags     map[string]tagInfo
	problems map[string]string
}

// stateCreateMu guards attaching a state to a config.
var stateCreateMu sync.Mutex

// stateFor returns the resolution state of cfg, creating and attaching one on
// first use. A nil config gets a detached state (nothing to attach it to).
func stateFor(cfg *config.Config) *resolutionState {
	if cfg == nil {
		return newResolutionState()
	}
	stateCreateMu.Lock()
	defer stateCreateMu.Unlock()
	if st, ok := cfg.ResolutionState.(*resolutionState); ok {
		return st
	}
	st := newResolutionState()
	cfg.ResolutionState = st
	return st
}

func newResolutionState() *resolutionState {
	return &resolutionState{
		observed: map[string]observed{},
		tags:     map[string]tagInfo{},
		problems: map[string]string{},
	}
}

func (s *resolutionState) record(baseDir, kind, name string, o observed) {
	s.mu.Lock()
	s.observed[observedKey(baseDir, kind, name)] = o
	s.mu.Unlock()
}

func (s *resolutionState) observedFor(baseDir, kind, name string) (observed, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.observed[observedKey(baseDir, kind, name)]
	return o, ok
}

func (s *resolutionState) recordTag(baseDir, kind, name string, t tagInfo) {
	s.mu.Lock()
	s.tags[observedKey(baseDir, kind, name)] = t
	s.mu.Unlock()
}

func (s *resolutionState) tagFor(baseDir, kind, name string) (tagInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tags[observedKey(baseDir, kind, name)]
	return t, ok
}

func (s *resolutionState) recordProblem(baseDir, kind, name, msg string) {
	s.mu.Lock()
	s.problems[observedKey(baseDir, kind, name)] = msg
	s.mu.Unlock()
}

func (s *resolutionState) problemFor(baseDir, kind, name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.problems[observedKey(baseDir, kind, name)]
}
