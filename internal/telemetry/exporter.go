package telemetry

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/samber/oops"
)

// Exporter defaults.
const (
	DefaultBatchMax     = 200
	DefaultRequestLimit = 5 * time.Second
	DefaultRetries      = 3
	DefaultBackoff      = time.Second
	maxRetryAfter       = 30 * time.Second
	responseLimit       = 64 << 10
)

// ErrRejected means the collector refused a batch for a reason retrying cannot
// fix (400, 401, 403, 404, 413). The batch is dropped and counted.
var ErrRejected = errors.New("collector rejected the batch")

// Exporter ships the spool to an OTLP collector over OTLP/HTTP (JSON or protobuf)
// or gRPC: batched, gzipped,
// retried with exponential backoff, at-least-once (event ids de-duplicate).
type Exporter struct {
	Spool    *Spool
	Encoder  Encoder
	Endpoint string
	// Protocol is http/json (the default), http/protobuf or grpc.
	Protocol string
	// HeadersEnv names environment variables holding "k=v,k2=v2" header lists.
	HeadersEnv []string
	Getenv     func(string) string
	Client     *http.Client
	grpc       grpcState

	BatchMax int
	Retries  int
	Backoff  time.Duration
	// Timeout bounds one HTTP request.
	Timeout time.Duration
	// Now, Sleep and Jitter are injectable for tests.
	Now    Clock
	Sleep  func(context.Context, time.Duration) error
	Jitter func() float64
}

// FlushResult summarizes one flush.
type FlushResult struct {
	Sent     int
	Rejected int
	Batches  int
	// Skipped is set when another flusher holds the lock.
	Skipped bool
}

func (x *Exporter) client() *http.Client {
	if x.Client != nil {
		return x.Client
	}
	client := &http.Client{
		Transport: http.DefaultTransport,
		Timeout:   x.timeout(),
		// A redirect could carry the headers to another host: never follow one.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		client.Transport = base.Clone() // honors HTTPS_PROXY
	}
	return client
}

func (x *Exporter) timeout() time.Duration {
	if x.Timeout > 0 {
		return x.Timeout
	}
	return DefaultRequestLimit
}

func (x *Exporter) now() time.Time {
	if x.Now != nil {
		return x.Now()
	}
	return SystemClock()
}

// Flush sends the pending spool. Delivered and rejected events leave the spool;
// events that could not be delivered stay for the next flush.
func (x *Exporter) Flush(ctx context.Context) (FlushResult, error) {
	var result FlushResult
	release, ok := x.Spool.TryFlushLock()
	if !ok {
		result.Skipped = true
		return result, nil
	}
	defer release()
	defer x.closeTransport()
	// Whatever the caller asked for, the flush ends before its lock can look stale.
	ctx, cancel := context.WithTimeout(ctx, MaxFlushTimeout)
	defer cancel()
	x.Client = x.client() // one transport for every request of this exporter

	events, _, err := x.Spool.Pending()
	if err != nil {
		return result, err
	}
	if len(events) == 0 {
		return result, nil
	}
	headers, err := x.headers()
	if err != nil {
		return result, errors.Join(err, x.record(err, "error", result))
	}
	batchMax := x.BatchMax
	if batchMax <= 0 {
		batchMax = DefaultBatchMax
	}

	var flushErr error
	for start := 0; start < len(events) && flushErr == nil; start += batchMax {
		batch := events[start:min(start+batchMax, len(events))]
		ids := make(map[string]bool, len(batch))
		for i := range batch {
			ids[batch[i].EventID] = true
		}
		err := x.sendBatch(ctx, batch, headers)
		switch {
		case err == nil:
			result.Sent += len(batch)
			result.Batches++
		case errors.Is(err, ErrRejected):
			// Dropped, not retried: the collector will never accept this body.
			result.Rejected += len(batch)
			flushErr = err
		default:
			flushErr = err
			continue // keep the batch in the spool for the next flush
		}
		flushErr = errors.Join(flushErr, x.Spool.Remove(ids))
	}
	status := "ok"
	switch {
	case errors.Is(flushErr, ErrRejected):
		status = "rejected"
	case flushErr != nil:
		status = "retry"
	}
	return result, errors.Join(flushErr, x.record(flushErr, status, result))
}

// record writes the bookkeeping. It returns an error only when the state file
// itself cannot be written.
func (x *Exporter) record(cause error, status string, result FlushResult) error {
	now := FormatTime(x.now())
	return x.Spool.UpdateState(func(st *State) {
		st.LastAttempt, st.LastStatus = now, status
		st.Sent += int64(result.Sent)
		st.Rejected += int64(result.Rejected)
		if cause == nil {
			st.LastFlush, st.LastError = now, ""
			return
		}
		st.LastError = sanitizeError(cause)
	})
}

func (x *Exporter) sendBatch(ctx context.Context, batch []Event, headers http.Header) error {
	now := x.now()
	logs, err := x.Encoder.EncodeLogs(batch, now)
	if err != nil {
		return oops.Wrapf(err, "encode logs")
	}
	if err := x.post(ctx, "/v1/logs", logs, headers); err != nil {
		return err
	}
	metrics, err := x.Encoder.EncodeMetrics(batch, now)
	if err != nil {
		return oops.Wrapf(err, "encode metrics")
	}
	if metrics == nil {
		return nil
	}
	return x.post(ctx, "/v1/metrics", metrics, headers)
}

// post sends one JSON-encoded request over the configured transport, retrying
// 429, 502, 503, 504 (gRPC: the equivalent codes) and network errors.
func (x *Exporter) post(ctx context.Context, path string, body []byte, headers http.Header) error {
	prepared, err := x.prepare(path, body)
	if err != nil {
		return err
	}
	retries := x.Retries
	if retries == 0 {
		retries = DefaultRetries
	}
	backoff := x.Backoff
	if backoff <= 0 {
		backoff = DefaultBackoff
	}
	var last error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			if err := x.sleep(ctx, x.delay(backoff, attempt, last)); err != nil {
				return errors.Join(last, err)
			}
		}
		last = x.send(ctx, path, prepared, headers)
		var transient *transientError
		if !errors.As(last, &transient) {
			return last
		}
	}
	return last
}

type transientError struct {
	msg        string
	retryAfter time.Duration
}

func (e *transientError) Error() string { return e.msg }

func (x *Exporter) delay(base time.Duration, attempt int, last error) time.Duration {
	d := base << (attempt - 1)
	var transient *transientError
	if errors.As(last, &transient) && transient.retryAfter > 0 {
		d = min(transient.retryAfter, maxRetryAfter)
	}
	jitter := rand.Float64 //nolint:gosec // backoff jitter is not security sensitive
	if x.Jitter != nil {
		jitter = x.Jitter
	}
	return d + time.Duration(float64(d)*0.25*jitter())
}

func (x *Exporter) sleep(ctx context.Context, d time.Duration) error {
	if x.Sleep != nil {
		return x.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// finishHTTP performs the request and classifies the response.
func (x *Exporter) finishHTTP(req *http.Request) error {
	resp, err := x.client().Do(req)
	if err != nil {
		return &transientError{msg: "network error: " + networkReason(err)}
	}
	defer resp.Body.Close()                                              //nolint:errcheck // response body of a finished request
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, responseLimit)) //nolint:errcheck // drained so the connection can be reused

	switch code := resp.StatusCode; {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusTooManyRequests, code == http.StatusBadGateway, code == http.StatusServiceUnavailable, code == http.StatusGatewayTimeout:
		return &transientError{msg: fmt.Sprintf("collector returned %d", code), retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	default:
		// Everything else is permanent: a 4xx, a redirect (the collector URL is wrong, and
		// retrying would keep the events forever) or a 5xx outside 502/503/504.
		return fmt.Errorf("%w: status %d", ErrRejected, code)
	}
}

func parseRetryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}

// networkReason describes a transport failure without the URL, which net/http
// embeds in its errors.
func networkReason(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Op + " failed"
	}
	return "connection failed"
}

// sanitizeError keeps an error short and free of anything URL- or header-shaped.
func sanitizeError(err error) string {
	text := firstLine(err.Error())
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, oops.Wrapf(err, "compress request")
	}
	if err := zw.Close(); err != nil {
		return nil, oops.Wrapf(err, "compress request")
	}
	return buf.Bytes(), nil
}

// headers resolves HeadersEnv into request headers. A name whose variable is
// unset is skipped (doctor reports it); a malformed pair is an error that names
// the variable, never its value.
func (x *Exporter) headers() (http.Header, error) {
	getenv := x.Getenv
	if getenv == nil {
		getenv = ambient.GetenvFunc(nil)
	}
	header := http.Header{}
	for _, name := range x.HeadersEnv {
		if !plausibleHeaderEnvName(name) {
			continue // a pasted credential is neither looked up nor named in an error
		}
		raw := getenv(name)
		if raw == "" {
			continue
		}
		for _, pair := range strings.Split(raw, ",") {
			key, value, ok := strings.Cut(pair, "=")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if !ok || key == "" || !validHeaderKey(key) || strings.ContainsAny(value, "\r\n") {
				return nil, oops.Errorf("headers_env %s: expected key=value pairs", name)
			}
			header.Add(key, value)
		}
	}
	return header, nil
}

func validHeaderKey(key string) bool {
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return true
}

// HeaderNamesSet reports, for doctor, which of the named variables are set.
func (x *Exporter) HeaderNamesSet() (set, unset []string) {
	getenv := x.Getenv
	if getenv == nil {
		getenv = ambient.GetenvFunc(nil)
	}
	for _, name := range x.HeadersEnv {
		if !plausibleHeaderEnvName(name) {
			unset = append(unset, hiddenHeaderName) // never look up or print a pasted credential
			continue
		}
		if getenv(name) != "" {
			set = append(set, name)
		} else {
			unset = append(unset, name)
		}
	}
	return set, unset
}
