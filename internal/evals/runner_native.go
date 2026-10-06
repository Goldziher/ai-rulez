package evals

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// defaultNativeConcurrency is how many activation runs a native adapter has in
// flight: each is a separate harness process, so the limit protects the machine
// and the provider's rate limit.
const defaultNativeConcurrency = 4

// activationOutcome is what one run of a prompt produced: the skills of the
// installed set that loaded, the usage the harness reported, or an error.
type activationOutcome struct {
	fired   map[string]bool
	input   int
	output  int
	costUSD float64
	err     string
}

// activationRunFunc runs one prompt once.
type activationRunFunc func(ctx context.Context, c *Case) activationOutcome

// runActivationRuns repeats every prompt of an activation request req.Runs times
// through one (at most concurrency at a time) and aggregates the outcomes into
// the protocol's response. It enforces req.MaxCostUSD itself: once the spend
// reaches it, the remaining runs are not started and count as errors.
func runActivationRuns(ctx context.Context, req *Request, concurrency int, one activationRunFunc) (*Response, error) {
	if req.Mode != ModeActivation {
		return nil, fmt.Errorf("this runner only answers activation requests, not %q", req.Mode)
	}
	runs := req.Runs
	if runs < 1 {
		runs = 1
	}
	if concurrency < 1 {
		concurrency = defaultNativeConcurrency
	}
	type job struct {
		c *Case
	}
	jobs := make(chan job)
	var mu sync.Mutex
	outcomes := make(map[string][]activationOutcome, len(req.Cases))
	spent := 0.0

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				mu.Lock()
				stop := req.MaxCostUSD > 0 && spent >= req.MaxCostUSD
				mu.Unlock()
				var out activationOutcome
				switch {
				case stop:
					out.err = "the --max-cost budget was reached before this run started"
				case ctx.Err() != nil:
					out.err = ctx.Err().Error()
				default:
					out = one(ctx, j.c)
				}
				mu.Lock()
				spent += out.costUSD
				outcomes[j.c.ID] = append(outcomes[j.c.ID], out)
				mu.Unlock()
			}
		}()
	}
	for i := range req.Cases {
		for r := 0; r < runs; r++ {
			jobs <- job{c: &req.Cases[i]}
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("activation run interrupted: %w", err)
	}

	resp := &Response{Version: ProtocolVersion}
	for i := range req.Cases {
		resp.Results = append(resp.Results, aggregateActivation(&req.Cases[i], outcomes[req.Cases[i].ID]))
	}
	return resp, nil
}

// aggregateActivation folds the runs of one prompt into a result: the counts over
// the runs that produced an outcome, and an error when none did.
func aggregateActivation(c *Case, outs []activationOutcome) Result {
	res := Result{Case: c.ID, Arm: ArmWith}
	counts := map[string]int{}
	usable, errored := 0, 0
	var lastErr string
	for i := range outs {
		o := &outs[i]
		res.InputTokens += o.input
		res.OutputTokens += o.output
		res.CostUSD += o.costUSD
		if o.err != "" {
			lastErr = o.err
			errored++
			continue
		}
		usable++
		if len(o.fired) == 0 {
			counts[firedNone]++
		}
		for id := range o.fired {
			counts[id]++
		}
	}
	if usable == 0 {
		res.Error = lastErr
		if res.Error == "" {
			res.Error = "no run produced a result"
		}
		return res
	}
	res.Runs, res.ErroredRuns, res.FiredCounts = usable, errored, counts
	for id, n := range counts {
		if id != firedNone && n > 0 {
			res.Fired = append(res.Fired, id)
		}
	}
	sort.Strings(res.Fired)
	if c.Target != "" {
		fired := counts[c.Target]*2 > usable
		res.Triggered = &fired
	}
	return res
}
