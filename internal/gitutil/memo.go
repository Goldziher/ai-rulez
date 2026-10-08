package gitutil

import (
	"context"
	"strings"
	"sync"
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

type memoKey struct{ dir, args string }

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
	m, _ := ctx.Value(memoCtxKey{}).(*Memo)
	return m
}

// revParse runs `git rev-parse args...` in dir, through the context's Memo when
// it has one. Failures are remembered too: "not a repository" is an answer.
func (g Git) revParse(ctx context.Context, dir string, args ...string) ([]byte, error) {
	memo := memoFrom(ctx)
	run := func() ([]byte, error) {
		out, _, err := g.run(ctx, dir, nil, append([]string{"rev-parse"}, args...)...)
		return out, err
	}
	if memo == nil {
		return run()
	}
	key := memoKey{dir, strings.Join(args, "\x00")}
	memo.mu.Lock()
	v, ok := memo.m[key]
	memo.mu.Unlock()
	if ok {
		return v.out, v.err
	}
	out, err := run()
	memo.mu.Lock()
	memo.m[key] = memoVal{out, err}
	memo.mu.Unlock()
	return out, err
}
