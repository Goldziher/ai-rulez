package lint

import (
	"strings"
	"testing"
)

func telemetryFixture(telemetry string) map[string]string {
	return map[string]string{
		".ai-rulez/config.toml": "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n" + telemetry,
		".ai-rulez/rules/r.md":  "---\nname: r\n---\nbody\n",
	}
}

func TestTelemetryConfigInvalid(t *testing.T) {
	cases := map[string]string{
		"insecure endpoint":  "[telemetry]\notlp_endpoint = \"http://collector.example.org\"\n",
		"literal credential": "[telemetry]\nheaders_env = [\"Bearer abc123\"]\n",
		"bad sample":         "[telemetry]\nsample = 2\n",
		"unknown protocol":   "[telemetry]\notlp_protocol = \"carrier-pigeon\"\n",
		"grpc endpoint path": "[telemetry]\notlp_protocol = \"grpc\"\notlp_endpoint = \"https://collector.example.org/v1/logs\"\n",
	}
	for name, toml := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, telemetryFixture(toml))
			gitAdd(t, root)
			findings := lintDir(t, root)
			if countCode(findings, CodeTelemetryConfigInvalid) == 0 {
				t.Errorf("expected AR9K0: %v", findings)
			}
			for _, f := range findings {
				if f.Code == CodeTelemetryConfigInvalid && f.Severity != SeverityError {
					t.Errorf("AR9K0 severity = %s", f.Severity)
				}
				if f.Code == CodeTelemetryConfigInvalid && strings.Contains(f.Message, "Bearer abc123") {
					t.Errorf("the literal value must not be echoed: %s", f.Message)
				}
			}
		})
	}
}

func TestTelemetryRepoKeyIgnoredWarns(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, telemetryFixture("[telemetry]\nenabled = true\nallow_network = true\notlp_endpoint = \"https://collector.example.org\"\nservice_name = \"team-x\"\n"))
	gitAdd(t, root)
	findings := lintDir(t, root)
	if countCode(findings, CodeTelemetryKeyIgnored) != 1 {
		t.Errorf("expected one AR9K1: %v", findings)
	}
	if countCode(findings, CodeTelemetryConfigInvalid) != 0 {
		t.Errorf("a valid table must not raise AR9K0: %v", findings)
	}
}

func TestTelemetryCleanConfigIsSilent(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, telemetryFixture("[telemetry]\nenabled = true\nsample = 0.5\n"))
	gitAdd(t, root)
	findings := lintDir(t, root)
	if countCode(findings, CodeTelemetryConfigInvalid)+countCode(findings, CodeTelemetryKeyIgnored) != 0 {
		t.Errorf("unexpected telemetry findings: %v", findings)
	}
}
