package telemetry

import (
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// fakeCollector receives the same two requests over any transport and records
// them decoded as protobuf, so the three transports compare on one type.
type fakeCollector struct {
	mu      sync.Mutex
	logs    []*collogs.ExportLogsServiceRequest
	metrics []*colmetrics.ExportMetricsServiceRequest
	headers []http.Header
	md      []metadata.MD
	// codes are the gRPC statuses returned in order, the last repeating; nil is OK.
	codes []codes.Code
	calls int
}

func (c *fakeCollector) nextCode() codes.Code {
	c.calls++
	if c.calls-1 < len(c.codes) {
		return c.codes[c.calls-1]
	}
	if len(c.codes) > 0 {
		return c.codes[len(c.codes)-1]
	}
	return codes.OK
}

func (c *fakeCollector) store(path string, body []byte, decode func([]byte, proto.Message) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch path {
	case PathLogs:
		msg := &collogs.ExportLogsServiceRequest{}
		if err := decode(body, msg); err != nil {
			return err
		}
		c.logs = append(c.logs, msg)
	case PathMetrics:
		msg := &colmetrics.ExportMetricsServiceRequest{}
		if err := decode(body, msg); err != nil {
			return err
		}
		c.metrics = append(c.metrics, msg)
	}
	return nil
}

// httpHandler serves OTLP/HTTP in either encoding, chosen by Content-Type.
func (c *fakeCollector) httpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "gzip", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(zr)
		decode := func(b []byte, m proto.Message) error { return proto.Unmarshal(b, m) }
		if r.Header.Get("Content-Type") == contentTypeJSON {
			decode = func(b []byte, m proto.Message) error { return protojson.Unmarshal(b, m) }
		}
		if err := c.store(r.URL.Path, body, decode); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.headers = append(c.headers, r.Header.Clone())
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
}

type grpcLogs struct {
	collogs.UnimplementedLogsServiceServer
	c *fakeCollector
}

func (s grpcLogs) Export(ctx context.Context, req *collogs.ExportLogsServiceRequest) (*collogs.ExportLogsServiceResponse, error) {
	s.c.mu.Lock()
	code := s.c.nextCode()
	if code == codes.OK {
		s.c.logs = append(s.c.logs, req)
	}
	md, _ := metadata.FromIncomingContext(ctx)
	s.c.md = append(s.c.md, md)
	s.c.mu.Unlock()
	if code != codes.OK {
		return nil, status.Error(code, "refused")
	}
	return &collogs.ExportLogsServiceResponse{}, nil
}

type grpcMetrics struct {
	colmetrics.UnimplementedMetricsServiceServer
	c *fakeCollector
}

func (s grpcMetrics) Export(_ context.Context, req *colmetrics.ExportMetricsServiceRequest) (*colmetrics.ExportMetricsServiceResponse, error) {
	s.c.mu.Lock()
	s.c.metrics = append(s.c.metrics, req)
	s.c.mu.Unlock()
	return &colmetrics.ExportMetricsServiceResponse{}, nil
}

// startGRPC serves the fake collector on a loopback port and returns its endpoint.
func startGRPC(t *testing.T, c *fakeCollector) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	collogs.RegisterLogsServiceServer(srv, grpcLogs{c: c})
	colmetrics.RegisterMetricsServiceServer(srv, grpcMetrics{c: c})
	go func() { _ = srv.Serve(l) }() //nolint:errcheck // stopped by the cleanup
	t.Cleanup(srv.Stop)
	return "http://" + l.Addr().String()
}

func flushThrough(t *testing.T, protocol, endpoint string, events []Event) (FlushResult, error) {
	t.Helper()
	x, spool, _ := newExporter(t, endpoint)
	x.Protocol = protocol
	x.Encoder.IncludeSession, x.Encoder.IncludePaths = true, true
	for i := range events {
		require.NoError(t, spool.Append(&events[i]))
	}
	return x.Flush(context.Background())
}

func logRecordsOf(reqs []*collogs.ExportLogsServiceRequest) []*logspb.LogRecord {
	var out []*logspb.LogRecord
	for _, req := range reqs {
		for _, rl := range req.GetResourceLogs() {
			for _, sl := range rl.GetScopeLogs() {
				out = append(out, sl.GetLogRecords()...)
			}
		}
	}
	return out
}

func attributeKeys(logs []*collogs.ExportLogsServiceRequest) []string {
	set := map[string]bool{}
	for _, rec := range logRecordsOf(logs) {
		for _, kv := range rec.GetAttributes() {
			set[kv.GetKey()] = true
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// The same events through all three transports must reach the collector as the
// same OTLP messages, and the exported attribute set must stay inside the
// allowlist: the closed-allowlist guarantee does not depend on the transport.
func TestTransports_ExportTheSameAllowlistedAttributes(t *testing.T) {
	evalEvents, _ := EvalEvents(sampleEvalResults())
	events := append(sampleEvents(), evalEvents...)

	jsonC, protoC, grpcC := &fakeCollector{}, &fakeCollector{}, &fakeCollector{}
	jsonSrv := httptest.NewServer(jsonC.httpHandler())
	defer jsonSrv.Close()
	protoSrv := httptest.NewServer(protoC.httpHandler())
	defer protoSrv.Close()
	grpcEndpoint := startGRPC(t, grpcC)

	for name, run := range map[string]struct{ protocol, endpoint string }{
		"json": {config.TelemetryProtocolHTTPJSON, jsonSrv.URL}, "protobuf": {config.TelemetryProtocolHTTPProtobuf, protoSrv.URL}, "grpc": {config.TelemetryProtocolGRPC, grpcEndpoint},
	} {
		result, err := flushThrough(t, run.protocol, run.endpoint, append([]Event(nil), events...))
		require.NoError(t, err, name)
		assert.Equal(t, FlushResult{Sent: len(events), Batches: 1}, result, name)
	}

	for name, c := range map[string]*fakeCollector{"json": jsonC, "protobuf": protoC, "grpc": grpcC} {
		require.Len(t, c.logs, 1, name)
		require.Len(t, c.metrics, 1, name)
	}
	assert.True(t, proto.Equal(jsonC.logs[0], protoC.logs[0]), "protobuf logs differ from the JSON logs")
	assert.True(t, proto.Equal(jsonC.logs[0], grpcC.logs[0]), "gRPC logs differ from the JSON logs")
	assert.True(t, proto.Equal(jsonC.metrics[0], protoC.metrics[0]), "protobuf metrics differ from the JSON metrics")
	assert.True(t, proto.Equal(jsonC.metrics[0], grpcC.metrics[0]), "gRPC metrics differ from the JSON metrics")

	allowed := map[string]bool{"event.name": true}
	for _, a := range Allowlist {
		allowed[a.Name] = true
	}
	for name, c := range map[string]*fakeCollector{"json": jsonC, "protobuf": protoC, "grpc": grpcC} {
		keys := attributeKeys(c.logs)
		assert.Contains(t, keys, "ai_rulez.eval.pass_rate", name+": eval scores travel as doubles on every transport")
		assert.Equal(t, 4, gaugeCount(c.metrics), name+": one gauge per score")
		assert.Equal(t, attributeKeys(jsonC.logs), keys, name)
		for _, key := range keys {
			assert.True(t, allowed[key], "%s exports %q, which is not on the allowlist", name, key)
		}
	}
}

func TestTransports_HeadersTravelOverEachTransport(t *testing.T) {
	protoC, grpcC := &fakeCollector{}, &fakeCollector{}
	protoSrv := httptest.NewServer(protoC.httpHandler())
	defer protoSrv.Close()
	grpcEndpoint := startGRPC(t, grpcC)

	_, err := flushThrough(t, config.TelemetryProtocolHTTPProtobuf, protoSrv.URL, sampleEvents())
	require.NoError(t, err)
	assert.Equal(t, contentTypeProtobuf, protoC.headers[0].Get("Content-Type"))
	assert.Equal(t, "Bearer s3cret", protoC.headers[0].Get("Authorization"))

	_, err = flushThrough(t, config.TelemetryProtocolGRPC, grpcEndpoint, sampleEvents())
	require.NoError(t, err)
	assert.Equal(t, []string{"Bearer s3cret"}, grpcC.md[0].Get("authorization"))
	assert.Equal(t, []string{"platform"}, grpcC.md[0].Get("x-team"))
	assert.Equal(t, []string{"gzip"}, grpcC.md[0].Get("grpc-accept-encoding")[:1], "requests are gzip-compressed like the HTTP transports")
}

func TestGRPC_TransientCodesAreRetriedAndPermanentOnesDropTheBatch(t *testing.T) {
	tests := []struct {
		name       string
		codes      []codes.Code
		wantSent   int
		wantReject int
		wantSleeps int
		wantErr    bool
	}{
		{name: "unavailable then ok", codes: []codes.Code{codes.Unavailable, codes.ResourceExhausted, codes.OK}, wantSent: 3, wantSleeps: 2},
		{name: "invalid argument is permanent", codes: []codes.Code{codes.InvalidArgument}, wantReject: 3, wantErr: true},
		{name: "unauthenticated is permanent", codes: []codes.Code{codes.Unauthenticated}, wantReject: 3, wantErr: true},
		{name: "always unavailable keeps the spool", codes: []codes.Code{codes.Unavailable}, wantSleeps: 3, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeCollector{codes: tt.codes}
			x, spool, sleeps := newExporter(t, startGRPC(t, c))
			x.Protocol = config.TelemetryProtocolGRPC
			x.Timeout = 2 * time.Second
			fill(t, spool, 3)

			result, err := x.Flush(context.Background())

			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.wantSent, result.Sent)
			assert.Equal(t, tt.wantReject, result.Rejected)
			assert.Len(t, *sleeps, tt.wantSleeps)
			pending, _, _ := spool.Pending()
			if tt.wantSent == 0 && tt.wantReject == 0 {
				assert.Len(t, pending, 3)
			} else {
				assert.Empty(t, pending)
			}
		})
	}
}

func TestGRPCTarget(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
		tls      bool
		wantErr  bool
	}{
		{endpoint: "https://collector.example.org", want: "collector.example.org:4317", tls: true},
		{endpoint: "https://collector.example.org:4443/", want: "collector.example.org:4443", tls: true},
		{endpoint: "collector.internal:4317", want: "collector.internal:4317", tls: true},
		{endpoint: "collector.internal", want: "collector.internal:4317", tls: true},
		{endpoint: "http://127.0.0.1:4317", want: "127.0.0.1:4317"},
		{endpoint: "http://[::1]", want: "[::1]:4317"},
		{endpoint: "://bad", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			got, useTLS, err := GRPCTarget(tt.endpoint)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.tls, useTLS)
		})
	}
}

func TestToProto_RejectsAFieldTheSchemaLacks(t *testing.T) {
	_, err := toProto(PathLogs, []byte(`{"resourceLogs":[],"prompt":"leak"}`))
	require.Error(t, err, "protojson refuses unknown fields, so the JSON cannot carry what the protobuf schema does not define")
	_, err = toProto("/v1/traces", []byte(`{}`))
	require.Error(t, err)
}

func TestGRPC_DeadEndpointKeepsEventsAndHidesTheAddress(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	x, spool, _ := newExporter(t, "http://"+addr)
	x.Protocol, x.Retries, x.Timeout = config.TelemetryProtocolGRPC, 1, time.Second
	fill(t, spool, 2)

	_, err = x.Flush(context.Background())

	require.Error(t, err)
	assert.NotContains(t, err.Error(), addr)
	pending, _, _ := spool.Pending()
	assert.Len(t, pending, 2)
}

func gaugeCount(reqs []*colmetrics.ExportMetricsServiceRequest) int {
	n := 0
	for _, req := range reqs {
		for _, rm := range req.GetResourceMetrics() {
			n += gaugesIn(rm)
		}
	}
	return n
}

func gaugesIn(rm *metricspb.ResourceMetrics) int {
	n := 0
	for _, sm := range rm.GetScopeMetrics() {
		for _, m := range sm.GetMetrics() {
			if m.GetGauge() != nil {
				n++
			}
		}
	}
	return n
}
