package llm

import (
	"context"
	"testing"
)

// replyClient answers every chat with a fixed reply and counts the calls.
type replyClient struct {
	Fake
	reply ChatResponse
	calls int
}

func (r *replyClient) Chat(_ context.Context, _ ChatRequest) (ChatResponse, error) {
	r.calls++
	return r.reply, nil
}

func TestCacheSkipsTruncatedFilteredAndEmptyReplies(t *testing.T) {
	for name, reply := range map[string]ChatResponse{
		"length":         {Text: "cut o", FinishReason: "length"},
		"content_filter": {Text: "x", FinishReason: "content_filter"},
		"empty":          {Text: "  ", FinishReason: "stop"},
	} {
		t.Run(name, func(t *testing.T) {
			backend := &replyClient{reply: reply}
			m := Wrap(backend, allowed(Config{Model: "gpt-4o-mini"}), Options{ConfigDir: t.TempDir()})
			for range 2 {
				if _, err := m.Chat(context.Background(), chatReq("q")); err != nil {
					t.Fatal(err)
				}
			}
			if backend.calls != 2 {
				t.Fatalf("a %s reply must not be cached, backend calls=%d", name, backend.calls)
			}
		})
	}
	backend := &replyClient{reply: ChatResponse{Text: "done", FinishReason: "stop"}}
	m := Wrap(backend, allowed(Config{Model: "gpt-4o-mini"}), Options{ConfigDir: t.TempDir()})
	for range 2 {
		if _, err := m.Chat(context.Background(), chatReq("q")); err != nil {
			t.Fatal(err)
		}
	}
	if backend.calls != 1 {
		t.Fatalf("a complete reply is cached, backend calls=%d", backend.calls)
	}
}

func TestCacheKeyCoversTheInjectedCompletionCap(t *testing.T) {
	dir := t.TempDir()
	backend := &replyClient{reply: ChatResponse{Text: "done", FinishReason: "stop"}}
	capped := Wrap(backend, allowed(Config{Model: "gpt-4o-mini", MaxCalls: 10}), Options{ConfigDir: dir})
	if _, err := capped.Chat(context.Background(), chatReq("q")); err != nil {
		t.Fatal(err)
	}
	uncapped := Wrap(backend, allowed(Config{Model: "gpt-4o-mini"}), Options{ConfigDir: dir})
	resp, err := uncapped.Chat(context.Background(), chatReq("q"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Cached || backend.calls != 2 {
		t.Fatalf("a reply made under the injected cap must not serve an uncapped run: cached=%v calls=%d", resp.Cached, backend.calls)
	}
}
