// Package bedrockaws resolves and refreshes the AWS credentials of Amazon Bedrock models through the
// AWS SDK's default credential chain. It is separate from internal/llm so that only the command line
// links the SDK; the command registers it once (Register) and every model client the process builds
// then signs Bedrock requests with credentials that are renewed before they expire.
package bedrockaws

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// refreshTimeout bounds one credential resolution (an SSO or STS round trip).
const refreshTimeout = 30 * time.Second

// Register installs the AWS credential chain for the Bedrock models of this process.
func Register() {
	llm.RegisterBedrockSupport(newSupport())
}

func newSupport() llm.BedrockSupport {
	var mu sync.Mutex
	var shared *Credentials
	environment := &Environment{}
	return llm.BedrockSupport{
		ProfileRegion: profileRegion,
		NewCredentials: func(getenv func(string) string) llm.BedrockCredentials {
			mu.Lock()
			defer mu.Unlock()
			if shared == nil {
				shared = New(getenv, Options{Environment: environment})
			}
			return shared
		},
		OwnsEnvironment: func(getenv func(string) string) bool {
			mu.Lock()
			defer mu.Unlock()
			return shared != nil && shared.ownsEnvironment(getenv)
		},
	}
}

// profileRegion reads the region of a shared-config profile. It reads local files only and never
// resolves credentials or calls the network; "" when the profile has no region.
func profileRegion(ctx context.Context, profile string) string {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithSharedConfigProfile(profile), awsconfig.WithCredentialsProvider(aws.AnonymousCredentials{}))
	if err != nil {
		return ""
	}
	return cfg.Region
}

// Options are the test seams of Credentials; the zero value is production.
type Options struct {
	// Credentials replaces the default AWS chain.
	Credentials aws.CredentialsProvider
	// SetEnv replaces os.Setenv/os.Unsetenv.
	SetEnv func(name, value string) error
	// Environment coordinates sources writing the same environment. The registered support
	// owns one shared instance; standalone sources get a private one when omitted.
	Environment *Environment
}

// Credentials is the llm.BedrockCredentials of the AWS chain. Sources writing the same
// environment share an Environment, which is held for every request in flight.
type Credentials struct {
	getenv      func(string) string
	setenv      func(name, value string) error
	environment *Environment

	initMu   sync.Mutex // guards provider and region
	provider *aws.CredentialsCache
	region   bool
}

// Environment coordinates credential exports and calls using them. Its zero value is ready to use.
type Environment struct {
	mu    sync.RWMutex
	owner *Credentials
	last  aws.Credentials
}

func sameCredentials(a, b aws.Credentials) bool {
	return a.AccessKeyID == b.AccessKeyID && a.SecretAccessKey == b.SecretAccessKey && a.SessionToken == b.SessionToken
}

func (c *Credentials) ownsEnvironment(getenv func(string) string) bool {
	c.environment.mu.RLock()
	defer c.environment.mu.RUnlock()
	return c.environment.owner == c && sameCredentials(c.environment.last, aws.Credentials{
		AccessKeyID: getenv("AWS_ACCESS_KEY_ID"), SecretAccessKey: getenv("AWS_SECRET_ACCESS_KEY"), SessionToken: getenv("AWS_SESSION_TOKEN"),
	})
}

// New returns the credential source for a client.
func New(getenv func(string) string, o Options) *Credentials {
	c := &Credentials{getenv: getenv, setenv: o.SetEnv, environment: o.Environment}
	if c.environment == nil {
		c.environment = &Environment{}
	}
	if c.setenv == nil {
		c.setenv = setProcessEnv
	}
	if o.Credentials != nil {
		c.provider = aws.NewCredentialsCache(o.Credentials)
	}
	return c
}

// setProcessEnv sets name, or removes it when value is empty.
func setProcessEnv(name, value string) error {
	if value == "" {
		return os.Unsetenv(name)
	}
	return os.Setenv(name, value)
}

// chain loads the default AWS credential chain once.
func (c *Credentials) chain(ctx context.Context) (*aws.CredentialsCache, error) {
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if c.provider != nil {
		return c.provider, nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, authError("load the AWS configuration for Bedrock: %v", err)
	}
	if cc, ok := cfg.Credentials.(*aws.CredentialsCache); ok {
		c.provider = cc
	} else {
		c.provider = aws.NewCredentialsCache(cfg.Credentials)
	}
	return c.provider, nil
}

// Acquire implements llm.BedrockCredentials.
func (c *Credentials) Acquire(ctx context.Context) (release func(), err error) {
	p, err := c.chain(ctx)
	if err != nil {
		return nil, err
	}
	rctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	creds, err := p.Retrieve(rctx)
	if err != nil {
		return nil, authError("resolve AWS credentials for Bedrock (sign in with `aws sso login`, set AWS_PROFILE, or export AWS_BEARER_TOKEN_BEDROCK): %v", err)
	}
	for {
		c.environment.mu.RLock()
		if c.environment.owner == c && sameCredentials(c.environment.last, creds) {
			return c.environment.mu.RUnlock, nil
		}
		c.environment.mu.RUnlock()
		c.environment.mu.Lock()
		if c.environment.owner == c && sameCredentials(c.environment.last, creds) {
			c.environment.mu.Unlock()
			continue
		}
		// A failed export can leave a partially changed environment; never reuse its old cache.
		c.environment.owner = nil
		err := c.export(ctx, creds)
		if err == nil {
			c.environment.owner, c.environment.last = c, creds
		}
		c.environment.mu.Unlock()
		if err != nil {
			return nil, err
		}
	}
}

// Invalidate implements llm.BedrockCredentials.
func (c *Credentials) Invalidate() {
	c.initMu.Lock()
	p := c.provider
	c.initMu.Unlock()
	if p != nil {
		p.Invalidate()
	}
}

// export puts creds in the environment while c.environment holds its write lock.
func (c *Credentials) export(ctx context.Context, creds aws.Credentials) error {
	pairs := [][2]string{
		{"AWS_ACCESS_KEY_ID", creds.AccessKeyID},
		{"AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey},
		{"AWS_SESSION_TOKEN", creds.SessionToken},
	}
	// The client reads the region from the environment; a region that only the profile names is
	// exported too, or the request would be signed for the wrong one.
	c.initMu.Lock()
	first := !c.region
	c.region = true
	c.initMu.Unlock()
	if first {
		if region, source := llm.BedrockRegion(ctx, c.getenv); region != "" && source == llm.RegionFromProfile {
			pairs = append(pairs, [2]string{"AWS_REGION", region})
		}
	}
	for _, kv := range pairs {
		if err := c.setenv(kv[0], kv[1]); err != nil {
			return &llm.Error{Kind: llm.KindConfig, Message: "export " + kv[0] + ": " + err.Error()}
		}
	}
	return nil
}

func authError(format string, args ...any) error {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	return &llm.Error{Kind: llm.KindAuth, Message: strings.TrimSpace(msg)}
}
