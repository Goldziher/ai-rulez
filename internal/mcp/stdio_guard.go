package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MaxFrameBytes caps one inbound JSON-RPC line. A longer line is answered with
// an invalid-request error and discarded; it does not end the session.
const MaxFrameBytes = 16 * 1024 * 1024

// maxConsecutiveReadErrors bounds how often the connection tolerates a read
// error in a row before giving up, so a broken stream cannot spin forever.
const maxConsecutiveReadErrors = 64

const jsonRPCVersion = "2.0"

// NewGuardedStdioTransport is the stdio transport of both MCP modes. The Go
// SDK ends the whole session on the first malformed frame; this one validates
// every line first, answers the bad ones with -32700 / -32600 itself and
// forwards only well-formed frames, so a confused client cannot take the
// server down.
//
// log receives the guard's debug reports; nil is the CLI's logger.
func NewGuardedStdioTransport(in io.Reader, out io.Writer, log logger.Logger) sdkmcp.Transport {
	return newGuardedTransport(in, out, MaxFrameBytes, log)
}

func newGuardedTransport(in io.Reader, out io.Writer, frameLimit int, log logger.Logger) sdkmcp.Transport {
	log = logger.Or(log)
	w := &lockedWriter{w: out}
	pr, pw := io.Pipe()
	go pumpValidFrames(in, pw, w, frameLimit, log)
	return &tolerantTransport{inner: &sdkmcp.IOTransport{
		Reader: pr, Writer: nopWriteCloser{w}, MaxLineLength: -1,
	}, out: w, log: log}
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p) //nolint:wrapcheck // plain writer passthrough
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// pumpValidFrames copies the valid lines of in to out, answering invalid ones.
func pumpValidFrames(in io.Reader, out *io.PipeWriter, replies io.Writer, frameLimit int, log logger.Logger) {
	br := bufio.NewReaderSize(in, 64*1024)
	for {
		line, tooLong, err := readBoundedLine(br, frameLimit)
		switch {
		case tooLong:
			writeRPCError(log, replies, nil, jsonrpc.CodeInvalidRequest, "request line exceeds the size limit and was dropped")
		case len(bytes.TrimSpace(line)) > 0:
			if code, msg, id := validateFrame(line); code != 0 {
				writeRPCError(log, replies, id, code, msg)
			} else if _, werr := out.Write(append(bytes.TrimSpace(line), '\n')); werr != nil {
				return
			}
		}
		if err != nil {
			_ = out.CloseWithError(err)
			return
		}
	}
}

// readBoundedLine reads up to the next newline. A line longer than limit is
// consumed and reported as tooLong without being buffered. err is io.EOF (or
// the reader's error) once the stream ends; the final unterminated line, if
// any, is still returned.
func readBoundedLine(br *bufio.Reader, limit int) (line []byte, tooLong bool, err error) {
	var buf []byte
	for {
		chunk, rerr := br.ReadSlice('\n')
		if !tooLong {
			if len(buf)+len(chunk) > limit {
				tooLong, buf = true, nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		if errors.Is(rerr, bufio.ErrBufferFull) {
			continue
		}
		return buf, tooLong, rerr //nolint:wrapcheck // io.EOF is the stream end signal
	}
}

// validateFrame reports the JSON-RPC error a line deserves, or code 0 when it
// is safe to hand to the SDK. id is the request id to echo when it is usable.
func validateFrame(line []byte) (code int64, msg string, id json.RawMessage) {
	line = bytes.TrimSpace(line)
	if !utf8.Valid(line) || !json.Valid(line) {
		return jsonrpc.CodeParseError, "parse error: frame is not valid UTF-8 JSON", nil
	}
	if line[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(line, &batch); err != nil || len(batch) == 0 {
			return jsonrpc.CodeInvalidRequest, "invalid request: batch must be a non-empty array", nil
		}
		for _, raw := range batch {
			if c, m, _ := validateObject(raw); c != 0 {
				return c, m, nil
			}
		}
		return 0, "", nil
	}
	return validateObject(line)
}

func validateObject(raw []byte) (code int64, msg string, id json.RawMessage) {
	var obj struct {
		JSONRPC json.RawMessage `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  json.RawMessage `json:"method"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return jsonrpc.CodeInvalidRequest, "invalid request: expected a JSON object", nil
	}
	if len(obj.ID) > 0 {
		if c := obj.ID[0]; c == '{' || c == '[' || c == 't' || c == 'f' {
			return jsonrpc.CodeInvalidRequest, "invalid request: id must be a string, number or null", nil
		}
		id = obj.ID
	}
	var version string
	if err := json.Unmarshal(obj.JSONRPC, &version); err != nil || version != jsonRPCVersion {
		return jsonrpc.CodeInvalidRequest, `invalid request: jsonrpc must be "2.0"`, id
	}
	return 0, "", id
}

func writeRPCError(log logger.Logger, w io.Writer, id json.RawMessage, code int64, message string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": jsonRPCVersion, "id": id, "error": map[string]any{"code": code, "message": message},
	})
	if err != nil {
		return
	}
	if _, err := w.Write(append(body, '\n')); err != nil {
		log.Debug("MCP: could not answer a malformed frame", "error", err)
	}
}

// tolerantTransport turns recoverable read errors of the inner connection
// (for example a batch the negotiated protocol version forbids) into
// invalid-request responses instead of ending the session.
type tolerantTransport struct {
	inner sdkmcp.Transport
	out   io.Writer
	log   logger.Logger
}

func (t *tolerantTransport) Connect(ctx context.Context) (sdkmcp.Connection, error) {
	conn, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // transport errors pass through unchanged
	}
	return &tolerantConn{Connection: conn, out: t.out, log: t.log}, nil
}

type tolerantConn struct {
	sdkmcp.Connection
	out io.Writer
	log logger.Logger
}

func (c *tolerantConn) Read(ctx context.Context) (jsonrpc.Message, error) {
	for failures := 0; ; failures++ {
		msg, err := c.Connection.Read(ctx)
		if err == nil {
			return msg, nil
		}
		if ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) ||
			failures >= maxConsecutiveReadErrors {
			return nil, err //nolint:wrapcheck // end of stream passes through unchanged
		}
		c.log.Debug("MCP: dropped a frame the SDK rejected", "error", err)
		writeRPCError(c.log, c.out, nil, jsonrpc.CodeInvalidRequest, "invalid request: "+err.Error())
	}
}
