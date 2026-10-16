package llm

import (
	"context"
	"regexp"
	"strings"
	"sync/atomic"
)

// ProviderBedrock is the provider prefix of Amazon Bedrock models.
const ProviderBedrock = "bedrock"

// RegionFromProfile is the source BedrockRegion reports for a region only the AWS_PROFILE profile names.
const RegionFromProfile = "the AWS_PROFILE profile"

// awsRegionRe matches an AWS region name such as eu-north-1, us-gov-west-1 or cn-north-1.
var awsRegionRe = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-\d{1,2}$`)

// BedrockCredentials keeps the AWS credentials of a Bedrock client fresh for a run of any length.
//
// The liter-llm client signs Bedrock requests with the keys it finds in the process environment,
// read on every request, and has no hook for a credential provider. An implementation resolves the
// credentials (the AWS default chain: environment, AWS_PROFILE and shared config including SSO and
// assume-role, web identity, container and instance roles, renewed before they expire) and exports
// every change to the environment before the next call.
type BedrockCredentials interface {
	// Acquire makes the environment carry current credentials and returns the release of the
	// hold that keeps them in place for one call: a call in flight is never overtaken by a renewal.
	Acquire(ctx context.Context) (release func(), err error)
	// Invalidate drops the cached credentials, so the next Acquire resolves them again. A call the
	// provider refused as unauthenticated uses it once before the failure is reported.
	Invalidate()
}

// BedrockSupport is what a binary plugs in to run Bedrock models: the AWS SDK lives in its own
// package (internal/llm/bedrockaws), and only the command line registers it, so the library
// packages that import this one do not link the SDK. Without it Bedrock still works with keys
// (AWS_BEARER_TOKEN_BEDROCK, or AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY) in the environment, and
// the region comes from the environment alone.
type BedrockSupport struct {
	// ProfileRegion reads the region of a shared-config profile from local files; "" when it has none.
	ProfileRegion func(ctx context.Context, profile string) string
	// NewCredentials returns the credential source; a process-wide exporter shares it across clients.
	NewCredentials func(getenv func(string) string) BedrockCredentials
	// OwnsEnvironment distinguishes credentials exported by this support from user static keys.
	OwnsEnvironment func(getenv func(string) string) bool
}

var bedrockSupport atomic.Pointer[BedrockSupport]

// RegisterBedrockSupport installs the AWS credential chain for every client this process builds.
func RegisterBedrockSupport(s BedrockSupport) { bedrockSupport.Store(&s) }

// BedrockRegion is the region the AWS tools would use: AWS_REGION, else AWS_DEFAULT_REGION (the
// order the AWS SDKs use), else the region of the shared-config profile AWS_PROFILE names. It also
// returns the source for an error message. "" means no region is configured.
func BedrockRegion(ctx context.Context, getenv func(string) string) (region, source string) {
	if region, source = bedrockEnvRegion(getenv); region != "" {
		return region, source
	}
	profile := strings.TrimSpace(getenv("AWS_PROFILE"))
	if profile == "" {
		return "", ""
	}
	if s := bedrockSupport.Load(); s != nil && s.ProfileRegion != nil {
		return s.ProfileRegion(ctx, profile), RegionFromProfile
	}
	return "", RegionFromProfile
}

// bedrockEnvRegion is the region the environment names: AWS_REGION, else AWS_DEFAULT_REGION.
func bedrockEnvRegion(getenv func(string) string) (region, source string) {
	for _, name := range []string{"AWS_REGION", "AWS_DEFAULT_REGION"} {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v, name
		}
	}
	return "", ""
}

// bedrockHostOf is the Bedrock runtime endpoint of a region.
func bedrockHostOf(region string) string {
	suffix := "amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		suffix = "amazonaws.com.cn"
	}
	return "bedrock-runtime." + region + "." + suffix
}

// BedrockEgressHost is the Bedrock runtime endpoint of the region from BedrockRegion. Without a
// region the host is not known and the result is "".
func BedrockEgressHost(ctx context.Context, getenv func(string) string) (string, error) {
	region, name := BedrockRegion(ctx, getenv)
	if region == "" {
		return "", nil
	}
	if !awsRegionRe.MatchString(region) {
		return "", newError(KindConfig, "%s is not an AWS region name (set it to a region such as eu-north-1)", name)
	}
	return bedrockHostOf(region), nil
}

// needsBedrockAuth reports whether the credential chain must be resolved: a Bedrock model with no
// API key variable named, no Bedrock API key (AWS_BEARER_TOKEN_BEDROCK) and no static key pair in
// the environment (the user's own keys are used as they are).
func needsBedrockAuth(cfg Config, getenv func(string) string) bool {
	if routeProvider(cfg) != ProviderBedrock || cfg.APIKeyEnv != "" || strings.TrimSpace(getenv("AWS_BEARER_TOKEN_BEDROCK")) != "" {
		return false
	}
	if s := bedrockSupport.Load(); s != nil && s.OwnsEnvironment != nil && s.OwnsEnvironment(getenv) {
		return true
	}
	return strings.TrimSpace(getenv("AWS_ACCESS_KEY_ID")) == "" || strings.TrimSpace(getenv("AWS_SECRET_ACCESS_KEY")) == ""
}

// registeredBedrockCredentials returns the credential source of the registered support, or nil.
func registeredBedrockCredentials(getenv func(string) string) BedrockCredentials {
	if s := bedrockSupport.Load(); s != nil && s.NewCredentials != nil {
		return s.NewCredentials(getenv)
	}
	return nil
}
