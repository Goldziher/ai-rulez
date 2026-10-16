package llm

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCredentials is a BedrockCredentials whose keys renew on demand: each Invalidate makes the
// next Acquire resolve the next generation, the way a refreshed SSO or assume-role session does.
type fakeCredentials struct {
	mu          sync.Mutex
	generation  int
	acquires    int
	invalidates int
}

func (f *fakeCredentials) Acquire(context.Context) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquires++
	if f.generation == 0 {
		f.generation = 1
	}
	return func() {}, nil
}

func (f *fakeCredentials) Invalidate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidates++
	f.generation++
}

func (f *fakeCredentials) invalidations() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.invalidates
}

func (f *fakeCredentials) current() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.generation
}

const bedrockReply = `{"output":{"message":{"role":"assistant","content":[{"text":"done"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2}}`

func TestNeedsBedrockAuth(t *testing.T) {
	bedrock := Config{Model: "bedrock/global.anthropic.claude-sonnet-5-5"}
	tests := []struct {
		name string
		cfg  Config
		env  map[string]string
		want bool
	}{
		{"profile or chain", bedrock, nil, true},
		{"another provider", Config{Model: "openai/gpt-4o"}, nil, false},
		{"a Bedrock API key", bedrock, map[string]string{"AWS_BEARER_TOKEN_BEDROCK": "t"}, false},
		{"a static key pair is the user's own", bedrock, map[string]string{"AWS_ACCESS_KEY_ID": "a", "AWS_SECRET_ACCESS_KEY": "b"}, false},
		{"a named key variable", Config{Model: bedrock.Model, APIKeyEnv: "MY_KEY"}, nil, false},
		{"only a key id is not a pair", bedrock, map[string]string{"AWS_ACCESS_KEY_ID": "a"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, needsBedrockAuth(tc.cfg, func(k string) string { return tc.env[k] }))
		})
	}
}

func TestManagedEnvironmentKeysStillNeedBedrockRefresh(t *testing.T) {
	RegisterBedrockSupport(BedrockSupport{OwnsEnvironment: func(func(string) string) bool { return true }})
	t.Cleanup(func() { bedrockSupport.Store(nil) })
	cfg := Config{Provider: ProviderBedrock, Model: "global.anthropic.claude-sonnet-5-5"}
	getenv := func(name string) string {
		return map[string]string{"AWS_ACCESS_KEY_ID": "exported-sdk-key", "AWS_SECRET_ACCESS_KEY": "exported-sdk-secret"}[name]
	}
	assert.True(t, needsBedrockAuth(cfg, getenv), "an earlier client's exported keys still require the credential chain")
}

func TestBedrockChatRenewsCredentialsAndRetriesAnExpiredOne(t *testing.T) {
	// Arrange: the first request is refused as unauthenticated (the session expired between the
	// credential check and the request); the retry carries the renewed keys and succeeds.
	t.Setenv("AWS_ACCESS_KEY_ID", "placeholder") // liter-llm insists on keys when it builds the client
	t.Setenv("AWS_SECRET_ACCESS_KEY", "placeholder")
	creds := &fakeCredentials{}
	var calls atomic.Int32
	var generations guarded[[]int]
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		generations.mu.Lock()
		generations.v = append(generations.v, creds.current())
		generations.mu.Unlock()
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"The security token included in the request is expired"}`)
			return
		}
		fmt.Fprint(w, bedrockReply)
	})
	cfg := localConfig(srv, Config{Provider: ProviderBedrock, Model: "global.anthropic.claude-sonnet-5-5", MaxRetries: -1})
	l, err := newLiterLLM(cfg, func(string) string { return "" }, creds)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	// Act
	resp, err := l.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "done", resp.Text)
	assert.Equal(t, []int{1, 2}, generations.get(), "the retry runs with renewed credentials")
	assert.Equal(t, 1, creds.invalidations())
}

func TestBedrockChatReportsAnAuthFailureThatPersists(t *testing.T) {
	// Arrange
	t.Setenv("AWS_ACCESS_KEY_ID", "placeholder")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "placeholder")
	creds := &fakeCredentials{}
	var calls atomic.Int32
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"denied"}`)
	})
	cfg := localConfig(srv, Config{Provider: ProviderBedrock, Model: "global.anthropic.claude-sonnet-5-5", MaxRetries: -1})
	l, err := newLiterLLM(cfg, func(string) string { return "" }, creds)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	// Act
	_, err = l.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})

	// Assert
	var e *Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, KindAuth, e.Kind)
	assert.EqualValues(t, 2, calls.Load(), "one renewal, then the failure is reported")
}

func TestBedrockEgressHost(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{"region from AWS_REGION", map[string]string{"AWS_REGION": "eu-north-1"}, "bedrock-runtime.eu-north-1.amazonaws.com", false},
		{"AWS_REGION wins", map[string]string{"AWS_REGION": "us-east-1", "AWS_DEFAULT_REGION": "eu-north-1"}, "bedrock-runtime.us-east-1.amazonaws.com", false},
		{"default region", map[string]string{"AWS_DEFAULT_REGION": "ap-southeast-2"}, "bedrock-runtime.ap-southeast-2.amazonaws.com", false},
		{"gov region", map[string]string{"AWS_REGION": "us-gov-west-1"}, "bedrock-runtime.us-gov-west-1.amazonaws.com", false},
		{"china region", map[string]string{"AWS_REGION": "cn-north-1"}, "bedrock-runtime.cn-north-1.amazonaws.com.cn", false},
		{"no region", nil, "", false},
		{"a profile region needs the AWS support, which a library does not register", map[string]string{"AWS_PROFILE": "work"}, "", false},
		{"not a region", map[string]string{"AWS_REGION": "evil.example.com/x"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BedrockEgressHost(context.Background(), func(k string) string { return tc.env[k] })
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDiagnoseNamesTheBedrockHost(t *testing.T) {
	cfg := Config{Model: "bedrock/global.anthropic.claude-sonnet-5-5", AllowNetwork: true}

	withRegion := Diagnose(cfg, Options{Getenv: func(k string) string { return map[string]string{"AWS_REGION": "eu-north-1"}[k] }})
	noRegion := Diagnose(cfg, Options{Getenv: func(string) string { return "" }})

	assert.Equal(t, "bedrock-runtime.eu-north-1.amazonaws.com", withRegion.BaseURLHost)
	assert.Empty(t, withRegion.Problems)
	assert.Empty(t, noRegion.BaseURLHost)
	assert.Contains(t, noRegion.Problems[0], "no AWS region")
}

func TestRegisteredSupportServesEveryClient(t *testing.T) {
	// Arrange
	t.Setenv("AWS_ACCESS_KEY_ID", "placeholder")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "placeholder")
	creds := &fakeCredentials{}
	RegisterBedrockSupport(BedrockSupport{
		ProfileRegion:  func(context.Context, string) string { return "ap-southeast-2" },
		NewCredentials: func(func(string) string) BedrockCredentials { return creds },
	})
	t.Cleanup(func() { bedrockSupport.Store(nil) })
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, bedrockReply) })
	cfg := localConfig(srv, Config{Provider: ProviderBedrock, Model: "global.anthropic.claude-sonnet-5-5", MaxRetries: -1})

	// Act
	host, err := BedrockEgressHost(context.Background(), func(k string) string { return map[string]string{"AWS_PROFILE": "work"}[k] })
	l, lerr := newLiterLLM(cfg, func(string) string { return "" }, nil)
	require.NoError(t, lerr)
	t.Cleanup(func() { _ = l.Close() })
	_, cerr := l.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "bedrock-runtime.ap-southeast-2.amazonaws.com", host)
	require.NoError(t, cerr)
	assert.GreaterOrEqual(t, creds.acquireCount(), 2, "the client asked the registered credentials at build time and for the call")
}

func (f *fakeCredentials) acquireCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acquires
}
