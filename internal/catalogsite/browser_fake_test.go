package catalogsite

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The protocol client is verified against a fake browser, so its framing, reply
// matching, event waiting and problem collection are tested on every machine, not
// only where Chrome runs. The fake is this test binary started again with
// fakeChromeEnv set; it speaks the pipe protocol on file descriptors 3 and 4.

const fakeChromeEnv = "AI_RULEZ_FAKE_CHROME"

// TestFakeChromeProcess is the fake browser. It does nothing in a normal run.
func TestFakeChromeProcess(t *testing.T) {
	if os.Getenv(fakeChromeEnv) != "1" {
		t.Skip("helper process of the browser client tests")
	}
	in, out := os.NewFile(3, "commands"), os.NewFile(4, "replies")
	reader := bufio.NewReader(in)
	send := func(v map[string]any) {
		data, err := json.Marshal(v)
		if err != nil {
			os.Exit(2)
		}
		_, _ = out.Write(append(data, 0)) //nolint:errcheck // the parent going away ends the loop
	}
	for {
		raw, err := reader.ReadBytes(0)
		if err != nil {
			os.Exit(0)
		}
		var msg struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Expression string `json:"expression"`
				URL        string `json:"url"`
			} `json:"params"`
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(bytes.TrimSuffix(raw, []byte{0}), &msg) != nil {
			os.Exit(2)
		}
		reply := map[string]any{"id": msg.ID, "result": map[string]any{}}
		if msg.SessionID != "" {
			reply["sessionId"] = msg.SessionID
		}
		var after []map[string]any
		switch msg.Method {
		case "Target.createTarget":
			reply["result"] = map[string]any{"targetId": "T1"}
		case "Target.attachToTarget":
			reply["result"] = map[string]any{"sessionId": "S1"}
		case "Page.navigate":
			if msg.Params.URL == "file:///fails" {
				reply["result"] = map[string]any{"errorText": "net::ERR_FILE_NOT_FOUND"}
				break
			}
			after = append(after,
				map[string]any{"method": "Runtime.consoleAPICalled", "sessionId": "S1", "params": map[string]any{"type": "error", "args": []any{map[string]any{"value": "boom"}}}},
				map[string]any{"method": "Runtime.consoleAPICalled", "sessionId": "S1", "params": map[string]any{"type": "log", "args": []any{map[string]any{"value": "fine"}}}},
				map[string]any{"method": "Log.entryAdded", "sessionId": "S1", "params": map[string]any{"entry": map[string]any{"level": "error", "source": "security", "text": "Refused to load", "url": "x"}}},
				map[string]any{"method": "Network.requestWillBeSent", "sessionId": "S1", "params": map[string]any{"request": map[string]any{"url": "https://evil.example/x.js"}}},
				map[string]any{"method": "Network.requestWillBeSent", "sessionId": "S1", "params": map[string]any{"request": map[string]any{"url": "file:///ok.css"}}},
				map[string]any{"method": "Page.loadEventFired", "sessionId": "S1", "params": map[string]any{"timestamp": 1}},
			)
		case "Runtime.evaluate":
			reply["result"] = map[string]any{"result": map[string]any{"value": true}}
			switch msg.Params.Expression {
			case "21*2":
				reply["result"] = map[string]any{"result": map[string]any{"value": 42}}
			case `"text"`:
				reply["result"] = map[string]any{"result": map[string]any{"value": "text"}}
			}
		}
		send(reply)
		for _, e := range after {
			send(e)
		}
	}
}

func startFakeBrowser(t *testing.T) *browser {
	t.Helper()
	t.Setenv(fakeChromeEnv, "1")
	exe, err := os.Executable()
	require.NoError(t, err)
	return launchBrowser(t, exe, []string{"-test.run=^TestFakeChromeProcess$", "-test.v=false"})
}

func TestBrowserClient_RoundTripsCommandsAndEvents(t *testing.T) {
	// Arrange
	b := startFakeBrowser(t)
	require.Equal(t, "S1", b.session)
	require.Empty(t, b.problems())

	// Act
	b.navigate("file:///index.html")

	// Assert
	assert.Equal(t, 42, b.evalInt("21*2"))
	assert.Equal(t, "text", b.evalString(`"text"`))
	assert.True(t, b.evalBool("true"))
	assert.Equal(t, 1, b.count("Page.loadEventFired"))
	problems := b.problems()
	assert.Contains(t, problems, "console.error: boom")
	assert.NotContains(t, strings.Join(problems, "\n"), "fine", "console.log is not a problem")
	assert.Contains(t, problems, "log security/error: Refused to load x")
	assert.Contains(t, problems, "network request: https://evil.example/x.js")
	assert.Len(t, problems, 3, "a file:// request is not a problem")
}

func TestBrowserClient_WaitsForEachLoad(t *testing.T) {
	// Arrange
	b := startFakeBrowser(t)

	// Act
	b.navigate("file:///a.html")
	b.navigate("file:///b.html")
	b.click("a", false)

	// Assert
	assert.Equal(t, 2, b.count("Page.loadEventFired"), "the second navigation waited for its own load event")
}
