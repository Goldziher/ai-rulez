package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// guardedPeer runs a skill server over the guarded stdio transport.
type guardedPeer struct {
	t    *testing.T
	in   io.WriteCloser
	out  *bufio.Scanner
	done chan error
}

func startGuardedPeer(t *testing.T, frameLimit int) *guardedPeer {
	t.Helper()
	cat, err := BuildCatalog("p", "claude", testServed(), SkillFilter{})
	require.NoError(t, err)
	srv := NewSkillServerWith("test", cat, ServeOptions{})
	clientToServer, serverIn := io.Pipe()
	serverOut, clientFromServer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.GetMCPServer().Run(ctx, srv.WrapTransport(newGuardedTransport(clientToServer, clientFromServer, frameLimit, nil)))
	}()
	t.Cleanup(func() {
		cancel()
		_ = serverIn.Close()
		_ = clientFromServer.Close()
	})
	scanner := bufio.NewScanner(serverOut)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	p := &guardedPeer{t: t, in: serverIn, out: scanner, done: done}
	p.send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	require.NotNil(t, p.read(), "initialize must be answered")
	p.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return p
}

func (p *guardedPeer) send(line string) {
	p.t.Helper()
	_, err := io.WriteString(p.in, line+"\n")
	require.NoError(p.t, err)
}

// read returns the next response object, or nil on EOF / timeout.
func (p *guardedPeer) read() map[string]any {
	p.t.Helper()
	type result struct {
		msg map[string]any
		ok  bool
	}
	ch := make(chan result, 1)
	go func() {
		if !p.out.Scan() {
			ch <- result{}
			return
		}
		var m map[string]any
		require.NoError(p.t, json.Unmarshal(p.out.Bytes(), &m), "server wrote a non-JSON line: %q", p.out.Text())
		ch <- result{m, true}
	}()
	select {
	case r := <-ch:
		return r.msg
	case <-time.After(10 * time.Second):
		return nil
	}
}

func errorCode(m map[string]any) float64 {
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(float64)
	return c
}

func TestGuardedStdio_MalformedInputKeepsServerAlive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		line string
		// wantCode is the error the bad line must be answered with; 0 means any
		// answer (or none) is acceptable as long as the server survives.
		wantCode float64
		answered bool
	}{
		{"not json", "not json", -32700, true},
		{"truncated object", "{", -32700, true},
		{"invalid utf8", "\xff\xfe\x00garbage", -32700, true},
		{"empty array", "[]", -32600, true},
		{"bare scalar", "42", -32600, true},
		{"wrong version", `{"jsonrpc":"1.0","id":3,"method":"tools/list"}`, -32600, true},
		{"missing version", `{"id":4,"method":"tools/list"}`, -32600, true},
		{"object id", `{"jsonrpc":"2.0","id":{"a":1},"method":"tools/list"}`, -32600, true},
		{"bool id", `{"jsonrpc":"2.0","id":true,"method":"tools/list"}`, -32600, true},
		{"batch with bad member", `[{"jsonrpc":"2.0","id":5,"method":"tools/list"},7]`, -32600, true},
		{"unknown method", `{"jsonrpc":"2.0","id":6,"method":"nope/nope"}`, -32601, true},
		{"params wrong type", `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":"str"}`, -32602, true},
		{"resources/read control char uri", `{"jsonrpc":"2.0","id":8,"method":"resources/read","params":{"uri":"\u0000"}}`, -32602, true},
		{"resources/read newline uri", `{"jsonrpc":"2.0","id":9,"method":"resources/read","params":{"uri":"skill://x/\n\u001f\u007f"}}`, -32602, true},
		{"skills/get control char uri", `{"jsonrpc":"2.0","id":10,"method":"skills/get","params":{"uri":"\u0000\u001b"}}`, -32602, true},
		{"skills/get uri not a string", `{"jsonrpc":"2.0","id":11,"method":"skills/get","params":{"uri":123}}`, -32602, true},
		{"unmatched response", `{"jsonrpc":"2.0","id":12,"result":{}}`, 0, false},
		{"blank line", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			p := startGuardedPeer(t, 1<<20)

			// Act
			p.send(tt.line)
			var first map[string]any
			if tt.answered {
				first = p.read()
			}
			p.send(`{"jsonrpc":"2.0","id":99,"method":"tools/list"}`)
			probe := p.read()

			// Assert
			if tt.answered {
				require.NotNil(t, first, "the bad line must be answered")
				assert.InDelta(t, tt.wantCode, errorCode(first), 0, "response: %v", first)
			}
			require.NotNil(t, probe, "server must still serve after %q", tt.name)
			assert.InDelta(t, 99, probe["id"], 0)
			assert.Nil(t, probe["error"])
			select {
			case err := <-p.done:
				t.Fatalf("server ended: %v", err)
			default:
			}
		})
	}
}

func TestGuardedStdio_OverlongLineIsAnsweredAndDropped(t *testing.T) {
	t.Parallel()
	// Arrange
	p := startGuardedPeer(t, 4096)

	// Act
	p.send(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"x":"` + strings.Repeat("a", 50_000) + `"}}`)
	first := p.read()
	p.send(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	probe := p.read()

	// Assert
	require.NotNil(t, first)
	assert.InDelta(t, -32600, errorCode(first), 0)
	require.NotNil(t, probe)
	assert.InDelta(t, 3, probe["id"], 0)
}

func TestGuardedStdio_BadRequestEchoesUsableID(t *testing.T) {
	t.Parallel()
	p := startGuardedPeer(t, 1<<20)
	p.send(`{"jsonrpc":"1.0","id":"abc","method":"tools/list"}`)
	got := p.read()
	require.NotNil(t, got)
	assert.Equal(t, "abc", got["id"])
}

func TestResourceNotFound_EscapesControlCharacters(t *testing.T) {
	t.Parallel()
	err := resourceNotFound("a\x00b\n\"")
	raw, mErr := json.Marshal(err)
	require.NoError(t, mErr)
	assert.True(t, json.Valid(raw), string(raw))
}
