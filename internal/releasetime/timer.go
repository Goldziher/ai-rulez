package releasetime

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// CommitDater returns the date in the git data of a tag (internal/includes.RemoteTagDate).
type CommitDater func(ctx context.Context, tag tagresolve.RawTag) (time.Time, error)

// Options configure a Timer for one source.
type Options struct {
	// Mode is a config.AgeSource* value; "" means auto.
	Mode string
	// Source identifies the repository in the first-seen record (the redacted URL).
	Source string
	// Forge looks up release publish times; nil disables the forge source.
	Forge forge.Client
	// Seen is the first-seen record; nil disables that source.
	Seen *SeenStore
	// Commit reads the date from the git data; nil disables that source.
	Commit CommitDater
	// Clock is the time source; the zero value is the wall clock.
	Clock ambient.Clock
}

// Timer implements tagresolve.ReleaseTimer for one repository.
type Timer struct {
	opt  Options
	repo forge.Repo
	// repoErr is why the source is not a forge repository ("" when it is).
	repoErr error

	mu   sync.Mutex
	memo map[string]memoEntry
}

type memoEntry struct {
	rt  tagresolve.ReleaseTime
	err error
}

var _ tagresolve.ReleaseTimer = (*Timer)(nil)

// New builds a Timer for the repository at opt.Source.
func New(opt Options) *Timer {
	if opt.Mode == "" {
		opt.Mode = config.AgeSourceAuto
	}
	t := &Timer{opt: opt, memo: map[string]memoEntry{}}
	t.repo, t.repoErr = forge.ParseRepo(opt.Source)
	return t
}

// ReleaseTime implements tagresolve.ReleaseTimer. The answer for a tag is
// remembered for the life of the Timer, so one run asks the forge once per tag.
func (t *Timer) ReleaseTime(ctx context.Context, tag tagresolve.RawTag) (tagresolve.ReleaseTime, error) {
	key := tag.Name + "\x00" + tag.Commit
	t.mu.Lock()
	if m, ok := t.memo[key]; ok {
		t.mu.Unlock()
		return m.rt, m.err
	}
	t.mu.Unlock()
	rt, err := t.lookup(ctx, tag)
	t.mu.Lock()
	t.memo[key] = memoEntry{rt, err}
	t.mu.Unlock()
	return rt, err
}

func (t *Timer) lookup(ctx context.Context, tag tagresolve.RawTag) (tagresolve.ReleaseTime, error) {
	switch t.opt.Mode {
	case config.AgeSourceForge:
		return t.fromForge(ctx, tag)
	case config.AgeSourceFirstSeen:
		return t.fromSeen(tag)
	case config.AgeSourceCommit:
		return t.fromCommit(ctx, tag)
	}
	// auto: the most trusted source that answers. A tag with no first-seen
	// record is "seen now", not "old": it is held back, never waved through
	// on a forgeable commit date. The commit date is used only when the
	// first-seen record cannot be kept.
	forgeRT, forgeErr := t.fromForge(ctx, tag)
	if forgeErr == nil {
		return forgeRT, nil
	}
	logger.Debug("release time: forge unavailable, trying first-seen", "tag", tag.Name, "error", forgeErr)
	seenRT, seenErr := t.fromSeen(tag)
	if seenErr == nil {
		return seenRT, nil
	}
	logger.Debug("release time: first-seen unavailable, trying the commit date", "tag", tag.Name, "error", seenErr)
	commitRT, commitErr := t.fromCommit(ctx, tag)
	if commitErr != nil {
		return tagresolve.ReleaseTime{}, fmt.Errorf("no release time for %s: %w", tag.Name, errors.Join(forgeErr, seenErr, commitErr))
	}
	return commitRT, nil
}

func (t *Timer) fromForge(ctx context.Context, tag tagresolve.RawTag) (tagresolve.ReleaseTime, error) {
	switch {
	case t.opt.Forge == nil:
		return tagresolve.ReleaseTime{}, errors.New("no forge client")
	case t.repoErr != nil:
		return tagresolve.ReleaseTime{}, t.repoErr
	}
	rel, err := t.opt.Forge.Release(ctx, t.repo, tag.Name)
	if err != nil {
		return tagresolve.ReleaseTime{}, fmt.Errorf("forge release time of %s: %w", tag.Name, err)
	}
	return tagresolve.ReleaseTime{At: rel.Published, From: tagresolve.SourceForge}, nil
}

func (t *Timer) fromSeen(tag tagresolve.RawTag) (tagresolve.ReleaseTime, error) {
	if t.opt.Seen == nil {
		return tagresolve.ReleaseTime{}, errors.New("no first-seen record")
	}
	at, _, err := t.opt.Seen.FirstSeen(t.opt.Source, tag.Name, tag.Commit, t.opt.Clock.Now())
	if err != nil {
		return tagresolve.ReleaseTime{}, err //nolint:wrapcheck // already contextual
	}
	return tagresolve.ReleaseTime{At: at, From: tagresolve.SourceFirstSeen}, nil
}

func (t *Timer) fromCommit(ctx context.Context, tag tagresolve.RawTag) (tagresolve.ReleaseTime, error) {
	if t.opt.Commit == nil {
		return tagresolve.ReleaseTime{}, errors.New("no way to read the commit date")
	}
	at, err := t.opt.Commit(ctx, tag)
	if err != nil {
		return tagresolve.ReleaseTime{}, fmt.Errorf("commit date of %s: %w", tag.Name, err)
	}
	return tagresolve.ReleaseTime{At: at, From: tagresolve.SourceCommit}, nil
}

// Observe records the tags of the repository in the first-seen record.
func (t *Timer) Observe(tags []tagresolve.RawTag) {
	if t.opt.Seen == nil {
		return
	}
	if err := t.opt.Seen.Observe(t.opt.Source, tags, t.opt.Clock.Now()); err != nil {
		logger.Debug("could not record the tags seen", "error", err)
	}
}
