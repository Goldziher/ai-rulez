package bedrockaws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// expiringProvider is a credential provider whose keys expire: each Retrieve after the expiry returns
// the next generation, the way an SSO or assume-role session renews.
type expiringProvider struct {
	mu       sync.Mutex
	gen      int
	ttl      time.Duration
	retrieve int
}

func (f *expiringProvider) Retrieve(context.Context) (aws.Credentials, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retrieve++
	f.gen++
	return aws.Credentials{
		AccessKeyID: fmt.Sprintf("ASIAGEN%d", f.gen), SecretAccessKey: fmt.Sprintf("secret%d", f.gen), SessionToken: fmt.Sprintf("token%d", f.gen),
		CanExpire: true, Expires: time.Now().Add(f.ttl),
	}, nil
}

// envRecorder is a SetEnv that records the environment the client would sign with.
type envRecorder struct {
	mu  sync.Mutex
	env map[string]string
	log []string
}

func (e *envRecorder) set(name, value string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.env == nil {
		e.env = map[string]string{}
	}
	e.env[name] = value
	e.log = append(e.log, name+"="+value)
	return nil
}

func (e *envRecorder) get(name string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.env[name]
}

func acquire(t *testing.T, c *Credentials) {
	t.Helper()
	release, err := c.Acquire(context.Background())
	require.NoError(t, err)
	release()
}

func TestCredentialsRefreshWhenTheyExpireMidRun(t *testing.T) {
	// Arrange
	prov := &expiringProvider{ttl: 80 * time.Millisecond}
	rec := &envRecorder{}
	c := New(func(string) string { return "" }, Options{Credentials: prov, SetEnv: rec.set})

	// Act
	acquire(t, c)
	first := rec.get("AWS_ACCESS_KEY_ID")
	acquire(t, c)
	cachedAgain := rec.get("AWS_ACCESS_KEY_ID")
	time.Sleep(120 * time.Millisecond) // the session expires mid-run
	acquire(t, c)

	// Assert
	assert.Equal(t, "ASIAGEN1", first)
	assert.Equal(t, "ASIAGEN1", cachedAgain, "credentials that are still valid are not resolved again")
	assert.Equal(t, "ASIAGEN2", rec.get("AWS_ACCESS_KEY_ID"), "expired credentials are replaced before the next call")
	assert.Equal(t, "token2", rec.get("AWS_SESSION_TOKEN"))
	assert.Equal(t, 2, prov.retrieve)
}

func TestInvalidateMakesTheNextAcquireResolveAgain(t *testing.T) {
	prov := &expiringProvider{ttl: time.Hour}
	rec := &envRecorder{}
	c := New(func(string) string { return "" }, Options{Credentials: prov, SetEnv: rec.set})

	acquire(t, c)
	c.Invalidate()
	acquire(t, c)

	assert.Equal(t, "ASIAGEN2", rec.get("AWS_ACCESS_KEY_ID"))
}

func TestAnUnchangedKeyIsExportedOnce(t *testing.T) {
	prov := &expiringProvider{ttl: time.Hour}
	rec := &envRecorder{}
	c := New(func(string) string { return "" }, Options{Credentials: prov, SetEnv: rec.set})

	for range 5 {
		acquire(t, c)
	}

	assert.Len(t, rec.log, 3, "key id, secret and token are exported once")
}

func profileOnlyRegion(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config")
	require.NoError(t, os.WriteFile(cfgFile, []byte("[profile work]\nregion = ap-southeast-2\n"), 0o600))
	t.Setenv("AWS_CONFIG_FILE", cfgFile)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "none"))
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
}

func TestBedrockRegionComesFromTheAWSProfile(t *testing.T) {
	// Arrange
	profileOnlyRegion(t)
	Register()
	t.Cleanup(func() { llm.RegisterBedrockSupport(llm.BedrockSupport{}) })

	// Act
	host, err := llm.BedrockEgressHost(context.Background(), func(k string) string { return map[string]string{"AWS_PROFILE": "work"}[k] })
	preferred, _ := llm.BedrockEgressHost(context.Background(), func(k string) string {
		return map[string]string{"AWS_PROFILE": "work", "AWS_REGION": "eu-north-1"}[k]
	})
	unknown, _ := llm.BedrockEgressHost(context.Background(), func(k string) string { return map[string]string{"AWS_PROFILE": "nope"}[k] })

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "bedrock-runtime.ap-southeast-2.amazonaws.com", host)
	assert.Equal(t, "bedrock-runtime.eu-north-1.amazonaws.com", preferred, "AWS_REGION wins over the profile")
	assert.Empty(t, unknown, "a profile with no region leaves the host unknown")
}

func TestAProfileOnlyRegionIsExportedForTheClient(t *testing.T) {
	// Arrange
	profileOnlyRegion(t)
	rec := &envRecorder{}
	c := New(func(k string) string { return map[string]string{"AWS_PROFILE": "work"}[k] }, Options{Credentials: &expiringProvider{ttl: time.Hour}, SetEnv: rec.set})
	Register()
	t.Cleanup(func() { llm.RegisterBedrockSupport(llm.BedrockSupport{}) })

	// Act
	acquire(t, c)

	// Assert
	assert.Equal(t, "ap-southeast-2", rec.get("AWS_REGION"))
}

func TestLaterClientsReuseTheRefreshingCredentialChain(t *testing.T) {
	support := newSupport()
	rec := &envRecorder{}
	first := support.NewCredentials(rec.get).(*Credentials)
	first.provider = aws.NewCredentialsCache(&expiringProvider{ttl: time.Hour})
	first.setenv = rec.set
	acquire(t, first)
	require.True(t, support.OwnsEnvironment(rec.get), "exported SDK keys are not user static keys")
	second := support.NewCredentials(rec.get).(*Credentials)
	require.Same(t, first, second, "a later client must retain the original SDK provider rather than snapshot its exports")
	second.Invalidate()
	acquire(t, second)
	assert.Equal(t, "ASIAGEN2", rec.get("AWS_ACCESS_KEY_ID"))
	assert.True(t, support.OwnsEnvironment(rec.get))
	require.NoError(t, rec.set("AWS_ACCESS_KEY_ID", "user-replacement"))
	assert.False(t, support.OwnsEnvironment(rec.get), "a user's replacement key is not managed")
}

func TestDifferentSourcesCannotReplaceCredentialsDuringACall(t *testing.T) {
	rec := &envRecorder{}
	environment := &Environment{}
	first := New(rec.get, Options{Credentials: &expiringProvider{ttl: time.Hour}, SetEnv: rec.set, Environment: environment})
	second := New(rec.get, Options{Credentials: &expiringProvider{ttl: time.Hour}, SetEnv: rec.set, Environment: environment})
	release, err := first.Acquire(context.Background())
	require.NoError(t, err)
	finished := make(chan error, 1)
	go func() {
		unlock, err := second.Acquire(context.Background())
		if err == nil {
			unlock()
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		release()
		t.Fatalf("another source acquired the process environment during a call: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	release()
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("the second source did not acquire after release")
	}
	// Switching back must export the first source again, even though its own key did not change.
	before := len(rec.log)
	acquire(t, first)
	assert.Equal(t, before+3, len(rec.log))
}
