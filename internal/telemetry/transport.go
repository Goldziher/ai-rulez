package telemetry

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	_ "google.golang.org/grpc/encoding/gzip" // registers the gzip compressor used for every export call
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/samber/oops"
)

// Content types of the OTLP/HTTP bodies.
const (
	contentTypeJSON     = "application/json"
	contentTypeProtobuf = "application/x-protobuf"
)

// defaultGRPCPort is the OTLP/gRPC port a collector listens on by default.
const defaultGRPCPort = "4317"

// The three transports share one encoder. The allowlisted attribute set is built
// once, as OTLP JSON (otlp.go); http/protobuf and grpc decode that exact JSON
// into the OTLP protobuf messages (protojson rejects an unknown field, so the
// JSON can never carry something the protobuf schema lacks) and send those. A
// field therefore cannot reach one transport and not another: transport_test.go
// decodes the three and compares them.

// toProto decodes an encoded OTLP JSON request into its protobuf message.
func toProto(path string, body []byte) (proto.Message, error) {
	var msg proto.Message
	switch path {
	case PathLogs:
		msg = &collogs.ExportLogsServiceRequest{}
	case PathMetrics:
		msg = &colmetrics.ExportMetricsServiceRequest{}
	default:
		return nil, oops.Errorf("telemetry: no protobuf message for path %q", path)
	}
	if err := (protojson.UnmarshalOptions{}).Unmarshal(body, msg); err != nil {
		return nil, oops.Wrapf(err, "decode OTLP request as protobuf")
	}
	return msg, nil
}

// wire is one request body prepared for the configured transport.
type wire struct {
	// body is the gzipped HTTP body (json and protobuf).
	body        []byte
	contentType string
	// msg is the message of a gRPC call.
	msg proto.Message
}

// prepare encodes a JSON request for the transport once, outside the retry loop.
func (x *Exporter) prepare(path string, jsonBody []byte) (wire, error) {
	switch x.protocol() {
	case config.TelemetryProtocolHTTPProtobuf:
		msg, err := toProto(path, jsonBody)
		if err != nil {
			return wire{}, err
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
		if err != nil {
			return wire{}, oops.Wrapf(err, "encode protobuf request")
		}
		compressed, err := gzipBytes(raw)
		return wire{body: compressed, contentType: contentTypeProtobuf}, err
	case config.TelemetryProtocolGRPC:
		msg, err := toProto(path, jsonBody)
		return wire{msg: msg}, err
	}
	compressed, err := gzipBytes(jsonBody)
	return wire{body: compressed, contentType: contentTypeJSON}, err
}

func (x *Exporter) protocol() string {
	if x.Protocol == "" {
		return config.TelemetryProtocolHTTPJSON
	}
	return x.Protocol
}

// send makes one attempt over the transport.
func (x *Exporter) send(ctx context.Context, path string, w wire, headers http.Header) error {
	if x.protocol() == config.TelemetryProtocolGRPC {
		return x.sendGRPC(ctx, path, w.msg, headers)
	}
	return x.postHTTP(ctx, path, w, headers)
}

// grpcState is the lazily dialed connection of an exporter, closed when a flush ends.
type grpcState struct {
	mu   sync.Mutex
	conn *grpc.ClientConn
}

// GRPCTarget turns an endpoint URL into the host:port a gRPC client dials and
// reports whether TLS applies (https). A missing port is the OTLP/gRPC default.
func GRPCTarget(endpoint string) (target string, useTLS bool, err error) {
	parsed, perr := url.Parse(strings.TrimRight(config.NormalizeTelemetryEndpoint(endpoint, config.TelemetryProtocolGRPC), "/"))
	if perr != nil || parsed.Host == "" {
		return "", false, oops.Errorf("invalid gRPC endpoint")
	}
	host := parsed.Host
	if parsed.Port() == "" {
		host = parsed.Hostname()
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		host += ":" + defaultGRPCPort
	}
	return host, parsed.Scheme == "https", nil
}

func (x *Exporter) dial() (*grpc.ClientConn, error) {
	x.grpc.mu.Lock()
	defer x.grpc.mu.Unlock()
	if x.grpc.conn != nil {
		return x.grpc.conn, nil
	}
	target, useTLS, err := GRPCTarget(x.Endpoint)
	if err != nil {
		return nil, err
	}
	creds := insecure.NewCredentials()
	if useTLS {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, oops.Errorf("dial collector: %s", firstLine(err.Error()))
	}
	x.grpc.conn = conn
	return conn, nil
}

// closeTransport releases the gRPC connection; HTTP needs nothing.
func (x *Exporter) closeTransport() {
	x.grpc.mu.Lock()
	defer x.grpc.mu.Unlock()
	if x.grpc.conn != nil {
		_ = x.grpc.conn.Close() //nolint:errcheck // the flush is over; a close error changes nothing
		x.grpc.conn = nil
	}
}

func (x *Exporter) sendGRPC(ctx context.Context, path string, msg proto.Message, headers http.Header) error {
	conn, err := x.dial()
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, x.timeout())
	defer cancel()
	if len(headers) > 0 {
		md := metadata.MD{}
		for key, values := range headers {
			md.Append(strings.ToLower(key), values...)
		}
		reqCtx = metadata.NewOutgoingContext(reqCtx, md)
	}
	switch req := msg.(type) {
	case *collogs.ExportLogsServiceRequest:
		_, err = collogs.NewLogsServiceClient(conn).Export(reqCtx, req, grpc.UseCompressor("gzip"))
	case *colmetrics.ExportMetricsServiceRequest:
		_, err = colmetrics.NewMetricsServiceClient(conn).Export(reqCtx, req, grpc.UseCompressor("gzip"))
	default:
		return oops.Errorf("telemetry: unexpected gRPC message for %s", path)
	}
	return classifyGRPC(err)
}

// classifyGRPC maps a gRPC status onto the exporter's two classes: transient
// (retried with backoff) and permanent (the batch is dropped and counted).
func classifyGRPC(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return &transientError{msg: "network error: " + networkReason(err)}
	}
	switch st.Code() {
	case codes.OK:
		return nil
	case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded, codes.Aborted, codes.Canceled, codes.Internal, codes.Unknown:
		return &transientError{msg: "collector returned " + st.Code().String(), retryAfter: retryInfo(st)}
	}
	return fmt.Errorf("%w: grpc code %s", ErrRejected, st.Code())
}

// retryInfo reads the server's RetryInfo detail, the gRPC form of Retry-After.
func retryInfo(st *status.Status) time.Duration {
	for _, detail := range st.Details() {
		if info, ok := detail.(*errdetails.RetryInfo); ok && info.GetRetryDelay() != nil {
			return info.GetRetryDelay().AsDuration()
		}
	}
	return 0
}

// postHTTP makes one OTLP/HTTP attempt, JSON or protobuf.
func (x *Exporter) postHTTP(ctx context.Context, path string, w wire, headers http.Header) error {
	reqCtx, cancel := context.WithTimeout(ctx, x.timeout())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, strings.TrimRight(x.Endpoint, "/")+path, bytes.NewReader(w.body))
	if err != nil {
		return oops.Errorf("build request: invalid endpoint")
	}
	req.Header = headers.Clone()
	req.Header.Set("Content-Type", w.contentType)
	req.Header.Set("Content-Encoding", "gzip")
	return x.finishHTTP(req)
}
