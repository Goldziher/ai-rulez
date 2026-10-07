package approval

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

const originProbeTimeout = 5 * time.Second

// originOf returns a reader of the forge repository behind the origin remote of
// dir, run once (under ctx) and only when a review-linked approval is checked.
func originOf(ctx context.Context, dir string) func() (forge.Repo, bool) {
	var (
		once sync.Once
		repo forge.Repo
		ok   bool
	)
	return func() (forge.Repo, bool) {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(ctx, originProbeTimeout)
			defer cancel()
			out, err := gitutil.Command(ctx, dir, "remote", "get-url", "origin").Output()
			if err != nil {
				return
			}
			parsed, perr := forge.ParseRepo(strings.TrimSpace(string(out)))
			if perr != nil {
				return
			}
			repo, ok = parsed, true
		})
		return repo, ok
	}
}
