package gitutil

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// Memo remembers the answers to the structural questions asked of a repository
// (is dir in a work tree, where is its top level, where is info/exclude, which
// git directories): the same few questions are asked of the same directory many
// times in one operation. It is opt-in and scoped by its context: nothing is
// cached unless the caller put a Memo there with WithMemo, and the answers live
// exactly as long as that context, so a command that creates or removes a
// repository starts over with a new one. Questions about history (HEAD, refs,
// file lists) are never memoised.
type Memo struct {
	mu sync.Mutex
	m  map[memoKey]memoVal
}

// memoKey names a question. The runner is part of it: two Git values over
// different runners (a fake in a test, a different host) are not asking the same
// repository, so they do not share answers.
type memoKey struct {
	dir, args string
	runner    runner.Runner
}

type memoVal struct {
	out []byte
	err error
}

type memoCtxKey struct{}

// WithMemo returns ctx carrying a fresh Memo, or ctx itself when it has one.
func WithMemo(ctx context.Context) context.Context {
	if memoFrom(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, memoCtxKey{}, &Memo{m: make(map[memoKey]memoVal)})
}

func memoFrom(ctx context.Context) *Memo {
	m, ok := ctx.Value(memoCtxKey{}).(*Memo)
	if !ok {
		return nil
	}
	return m
}

// memoRunner returns the runner as a map key. ok is false for a runner without
// an identity: a func adapter, or a value holding something that cannot be
// compared (using it as a key would panic, or make unrelated runners share an
// answer). Such a runner's questions are never memoised.
func (g Git) memoRunner() (r runner.Runner, ok bool) {
	r = g.runner()
	if r == nil || !reflect.ValueOf(r).Comparable() {
		return nil, false
	}
	return r, true
}

// revParse runs `git rev-parse args...` in dir, through the context's Memo when
// it has one. Failures are remembered too: "not a repository" is an answer. A
// run cut short by the context's own cancellation or deadline is not: it says
// nothing about the repository.
func (g Git) revParse(ctx context.Context, dir string, args ...string) ([]byte, error) {
	memo := memoFrom(ctx)
	run := func() ([]byte, error) {
		out, _, err := g.run(ctx, dir, nil, append([]string{"rev-parse"}, args...)...)
		return out, err
	}
	if memo == nil {
		return run()
	}
	r, ok := g.memoRunner()
	if !ok {
		return run()
	}
	key := memoKey{dir: dir, args: strings.Join(args, "\x00"), runner: r}
	memo.mu.Lock()
	v, ok := memo.m[key]
	memo.mu.Unlock()
	if ok {
		return v.out, v.err
	}
	out, err := run()
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return out, err
	}
	memo.mu.Lock()
	memo.m[key] = memoVal{out, err}
	memo.mu.Unlock()
	return out, err
}
