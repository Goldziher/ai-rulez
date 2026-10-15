package commands

import (
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

const providerBedrock = "bedrock"

// awsRegionRe matches an AWS region name such as eu-north-1, us-gov-west-1 or cn-north-1.
var awsRegionRe = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-\d{1,2}$`)

// bedrockEgressHost is the Bedrock runtime endpoint of the region named by AWS_REGION, else
// AWS_DEFAULT_REGION (the order the AWS SDKs use). Without a region the host is not known.
func bedrockEgressHost(getenv func(string) string) (string, error) {
	region := strings.TrimSpace(getenv("AWS_REGION"))
	name := "AWS_REGION"
	if region == "" {
		region, name = strings.TrimSpace(getenv("AWS_DEFAULT_REGION")), "AWS_DEFAULT_REGION"
	}
	if region == "" {
		return "", nil
	}
	if !awsRegionRe.MatchString(region) {
		return "", oops.Hint("Set it to a region such as eu-north-1").Errorf("%s is not an AWS region name", name)
	}
	suffix := "amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		suffix = "amazonaws.com.cn"
	}
	return "bedrock-runtime." + region + "." + suffix, nil
}

// awsCredentials are the temporary or long-lived keys the default AWS credential chain resolved.
type awsCredentials struct {
	AccessKeyID, SecretAccessKey, SessionToken string
}

// awsResolver resolves credentials the way the AWS CLI does.
type awsResolver func(ctx context.Context) (awsCredentials, error)

// resolveAWSCredentialChain walks the default AWS chain: environment, AWS_PROFILE and the shared
// config files (including SSO and assume-role profiles), web identity (AWS_WEB_IDENTITY_TOKEN_FILE),
// container and EC2 instance roles.
func resolveAWSCredentialChain(ctx context.Context) (awsCredentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return awsCredentials{}, oops.Wrapf(err, "load the AWS configuration")
	}
	c, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return awsCredentials{}, oops.Hint("Sign in (aws sso login), set AWS_PROFILE, or export AWS_BEARER_TOKEN_BEDROCK").
			Wrapf(err, "resolve AWS credentials for Bedrock")
	}
	return awsCredentials{AccessKeyID: c.AccessKeyID, SecretAccessKey: c.SecretAccessKey, SessionToken: c.SessionToken}, nil
}

// setProcessEnv sets name, or removes it when value is empty.
func setProcessEnv(name, value string) error {
	if value == "" {
		return os.Unsetenv(name)
	}
	return os.Setenv(name, value)
}

// exportBedrockCredentials makes the AWS credential chain usable by the model client. The
// liter-llm client signs Bedrock requests with the keys it finds in the process environment
// (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN) and has no hook for a
// credential provider, so the resolved keys are exported there for the life of this command.
// Nothing is done for another provider, with an API key or AWS_BEARER_TOKEN_BEDROCK in play, or
// when the key pair is already in the environment.
func exportBedrockCredentials(ctx context.Context, lc llm.Config, getenv func(string) string, resolve awsResolver, set func(name, value string) error) error {
	if routedProvider(lc) != providerBedrock || lc.APIKeyEnv != "" || strings.TrimSpace(getenv("AWS_BEARER_TOKEN_BEDROCK")) != "" {
		return nil
	}
	if strings.TrimSpace(getenv("AWS_ACCESS_KEY_ID")) != "" && strings.TrimSpace(getenv("AWS_SECRET_ACCESS_KEY")) != "" {
		return nil
	}
	creds, err := resolve(ctx)
	if err != nil {
		return err
	}
	for _, kv := range [][2]string{
		{"AWS_ACCESS_KEY_ID", creds.AccessKeyID},
		{"AWS_SECRET_ACCESS_KEY", creds.SecretAccessKey},
		{"AWS_SESSION_TOKEN", creds.SessionToken},
	} {
		if err := set(kv[0], kv[1]); err != nil {
			return oops.Wrapf(err, "export %s", kv[0])
		}
	}
	return nil
}
