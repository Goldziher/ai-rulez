package telemetry

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type collector struct {
	mu       sync.Mutex
	requests []capturedRequest
	// statuses are returned in order; the last repeats.
	statuses   []int
	retryAfter string
}

type capturedRequest struct {
	Path   string
	Header http.Header
	Body   []byte
}

func (c *collector) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := gzip.NewReader(r.Body)
		var body []byte
		if err == nil {
			body, _ = io.ReadAll(zr)
		}
		c.mu.Lock()
		c.requests = append(c.requests, capturedRequest{Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
		status := http.StatusOK
		if n := len(c.requests) - 1; n < len(c.statuses) {
			status = c.statuses[n]
		} else if len(c.statuses) > 0 {
			status = c.statuses[len(c.statuses)-1]
		}
		retry := c.retryAfter
		c.mu.Unlock()
		if retry != "" && status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", retry)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"partialSuccess":{}}`))
	})
}

func (c *collector) paths() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, r := range c.requests {
		out = append(out, r.Path)
	}
	return out
}

func newExporter(t *testing.T, endpoint string) (*Exporter, *Spool, *[]time.Duration) {
	t.Helper()
	spool := &Spool{Dir: t.TempDir()}
	var sleeps []time.Duration
	return &Exporter{
		Spool: spool, Endpoint: endpoint, Encoder: Encoder{ServiceName: "ai-rulez"},
		Now: fixedClock, Backoff: time.Second, Jitter: func() float64 { return 0 },
		Sleep: func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil },
		Getenv: func(k string) string {
			if k == "OTLP_HEADERS" {
				return "authorization=Bearer s3cret, x-team=platform"
			}
			return ""
		},
		HeadersEnv: []string{"OTLP_HEADERS"},
	}, spool, &sleeps
}

func fill(t *testing.T, s *Spool, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		require.NoError(t, s.Append(newTestEvent(i)))
	}
}

func TestExporter_PostsLogsAndMetricsWithExpectedShape(t *testing.T) {
	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()
	x, spool, _ := newExporter(t, srv.URL)
	events := sampleEvents()
	for i := range events {
		require.NoError(t, spool.Append(&events[i]))
	}

	result, err := x.Flush(context.Background())
	require.NoError(t, err)
	assert.Equal(t, FlushResult{Sent: 4, Batches: 1}, result)
	assert.Equal(t, []string{"/v1/logs", "/v1/metrics"}, c.paths())

	req := c.requests[0]
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "gzip", req.Header.Get("Content-Encoding"))
	assert.Equal(t, "Bearer s3cret", req.Header.Get("Authorization"))
	assert.Equal(t, "platform", req.Header.Get("X-Team"))

	var logs struct {
		ResourceLogs []struct {
			Resource struct {
				Attributes []struct{ Key string } `json:"attributes"`
			}
			ScopeLogs []struct {
				LogRecords []map[string]any `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	require.NoError(t, json.Unmarshal(req.Body, &logs))
	require.Len(t, logs.ResourceLogs, 1)
	assert.Equal(t, "service.name", logs.ResourceLogs[0].Resource.Attributes[0].Key)
	assert.Len(t, logs.ResourceLogs[0].ScopeLogs[0].LogRecords, 4)
	assert.Contains(t, string(c.requests[1].Body), `"ai_rulez.item.loads"`)

	remaining, _, err := spool.Pending()
	require.NoError(t, err)
	assert.Empty(t, remaining, "delivered events leave the spool")
	st := spool.ReadState()
	assert.Equal(t, "ok", st.LastStatus)
	assert.Equal(t, int64(4), st.Sent)
	assert.Equal(t, "2026-10-05T09:12:44Z", st.LastFlush)
	assert.NotContains(t, st.LastError+st.LastStatus, "s3cret")
}

func TestExporter_RetriesTransientStatusesWithBackoff(t *testing.T) {
	c := &collector{statuses: []int{503, 429, 200, 200}, retryAfter: "7"}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()
	x, spool, sleeps := newExporter(t, srv.URL)
	fill(t, spool, 3)

	result, err := x.Flush(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, result.Sent)
	assert.Equal(t, []time.Duration{time.Second, 7 * time.Second}, *sleeps, "exponential base, then Retry-After")
}

func TestExporter_GivesUpAfterRetriesAndKeepsTheSpool(t *testing.T) {
	c := &collector{statuses: []int{503}}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()
	x, spool, sleeps := newExporter(t, srv.URL)
	fill(t, spool, 3)

	_, err := x.Flush(context.Background())
	require.Error(t, err)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, *sleeps)
	events, _, _ := spool.Pending()
	assert.Len(t, events, 3, "undelivered events stay for the next flush")
	st := spool.ReadState()
	assert.Equal(t, "retry", st.LastStatus)
	assert.Contains(t, st.LastError, "503")
}

func TestExporter_RejectedBatchIsDroppedAndCounted(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 413, 302, 307, 500, 501, 505} {
		c := &collector{statuses: []int{status}}
		srv := httptest.NewServer(c.handler())
		x, spool, sleeps := newExporter(t, srv.URL)
		fill(t, spool, 2)
		result, err := x.Flush(context.Background())
		srv.Close()
		require.ErrorIs(t, err, ErrRejected, status)
		assert.Equal(t, 2, result.Rejected)
		assert.Empty(t, *sleeps, "a permanent status is not retried")
		events, _, _ := spool.Pending()
		assert.Empty(t, events, "a batch the collector will never accept must not block the queue")
		assert.Equal(t, int64(2), spool.ReadState().Rejected)
	}
}

func TestExporter_BatchesAtBatchMax(t *testing.T) {
	c := &collector{}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()
	x, spool, _ := newExporter(t, srv.URL)
	x.BatchMax = 2
	fill(t, spool, 5)
	result, err := x.Flush(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, result.Batches)
	assert.Len(t, c.paths(), 6)
}

func TestExporter_DeadEndpointKeepsEventsAndReturnsFast(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close()) // nothing listens: connection refused

	x, spool, _ := newExporter(t, "http://"+addr)
	fill(t, spool, 2)
	_, err = x.Flush(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), addr, "errors never carry the URL")
	events, _, _ := spool.Pending()
	assert.Len(t, events, 2)
}

func TestExporter_TimesOutOnABlackholeCollector(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	x, spool, _ := newExporter(t, srv.URL)
	x.Timeout, x.Retries = 100*time.Millisecond, 1
	fill(t, spool, 1)
	start := time.Now()
	_, err := x.Flush(context.Background())
	require.Error(t, err)
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Contains(t, err.Error(), "timeout")
}

func TestExporter_DoesNotFollowRedirects(t *testing.T) {
	var hit bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	x, spool, _ := newExporter(t, redirector.URL)
	x.Retries = 1
	fill(t, spool, 1)
	_, _ = x.Flush(context.Background()) //nolint:errcheck // the redirect is a failure; only the target matters
	assert.False(t, hit, "credentials must not be forwarded to a redirect target")
}

func TestExporter_MalformedHeaderEnvNamesTheVariableNotTheValue(t *testing.T) {
	x, spool, _ := newExporter(t, "http://127.0.0.1:1")
	x.Getenv = func(string) string { return "no-equals-sign-here" }
	fill(t, spool, 1)
	_, err := x.Flush(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OTLP_HEADERS")
	assert.NotContains(t, err.Error(), "no-equals-sign-here")
}

func TestExporter_OnlyOneFlusherAtATime(t *testing.T) {
	x, spool, _ := newExporter(t, "http://127.0.0.1:1")
	release, ok := spool.TryFlushLock()
	require.True(t, ok)
	defer release()
	result, err := x.Flush(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Skipped)
}

func TestNetworkReasonHidesURL(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:1/secret/path?token=abc", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	assert.False(t, strings.Contains(networkReason(err), "secret"))
}
