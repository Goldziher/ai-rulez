package remote

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLValidator_Validate(t *testing.T) {
	validator := NewURLValidator()

	testCases := []struct {
		name        string
		url         string
		shouldError bool
		errorMsg    string
	}{
		// Valid URLs (using real domains that resolve)
		{
			name:        "valid_https_url",
			url:         "https://google.com/path/to/file.yaml",
			shouldError: false,
		},
		{
			name:        "valid_http_url",
			url:         "http://google.com/path/to/file.yaml",
			shouldError: false,
		},
		{
			name:        "valid_url_with_port",
			url:         "https://google.com:443/file.yaml",
			shouldError: false,
		},

		// Invalid schemes
		{
			name:        "ftp_scheme_blocked",
			url:         "ftp://example.com/file.yaml",
			shouldError: true,
			errorMsg:    "scheme \"ftp\" not allowed",
		},
		{
			name:        "file_scheme_blocked",
			url:         "file:///etc/passwd",
			shouldError: true,
			errorMsg:    "scheme \"file\" not allowed",
		},

		// Empty and malformed URLs
		{
			name:        "empty_url",
			url:         "",
			shouldError: true,
			errorMsg:    "empty URL not allowed",
		},
		{
			name:        "malformed_url",
			url:         "not-a-url",
			shouldError: true,
			errorMsg:    "scheme",
		},
		{
			name:        "missing_hostname",
			url:         "https:///path",
			shouldError: true,
			errorMsg:    "missing hostname in URL",
		},

		// Localhost and loopback addresses
		{
			name:        "localhost_blocked",
			url:         "http://localhost:8080/file.yaml",
			shouldError: true,
			errorMsg:    "IP address",
		},
		{
			name:        "localhost_caps_blocked",
			url:         "http://LOCALHOST/file.yaml",
			shouldError: true,
			errorMsg:    "IP address",
		},
		{
			name:        "loopback_ip_blocked",
			url:         "http://127.0.0.1:8080/file.yaml",
			shouldError: true,
			errorMsg:    "IP address 127.0.0.1 is blocked",
		},
		{
			name:        "ipv6_loopback_blocked",
			url:         "http://[::1]/file.yaml",
			shouldError: true,
			errorMsg:    "IP address ::1 is blocked",
		},

		// Private IP ranges (RFC 1918)
		{
			name:        "private_10_blocked",
			url:         "http://10.0.0.1/file.yaml",
			shouldError: true,
			errorMsg:    "IP address 10.0.0.1 is blocked",
		},
		{
			name:        "private_172_blocked",
			url:         "http://172.16.0.1/file.yaml",
			shouldError: true,
			errorMsg:    "IP address 172.16.0.1 is blocked",
		},
		{
			name:        "private_192_blocked",
			url:         "http://192.168.1.1/file.yaml",
			shouldError: true,
			errorMsg:    "IP address 192.168.1.1 is blocked",
		},

		// Metadata service endpoints
		{
			name:        "aws_metadata_blocked",
			url:         "http://169.254.169.254/latest/meta-data/",
			shouldError: true,
			errorMsg:    "IP address 169.254.169.254 is blocked",
		},
		{
			name:        "gcp_metadata_blocked",
			url:         "http://metadata.google.internal/computeMetadata/v1/",
			shouldError: true,
			errorMsg:    "failed to resolve hostname",
		},

		// Invalid ports
		{
			name:        "invalid_port_zero",
			url:         "https://example.com:0/file.yaml",
			shouldError: true,
			errorMsg:    "port 0 out of valid range",
		},
		{
			name:        "invalid_port_too_high",
			url:         "https://example.com:65536/file.yaml",
			shouldError: true,
			errorMsg:    "port 65536 out of valid range",
		},
		{
			name:        "invalid_port_non_numeric",
			url:         "https://example.com:abc/file.yaml",
			shouldError: true,
			errorMsg:    "invalid URL format",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate(tc.url)

			if tc.shouldError {
				require.Error(t, err, "Expected error for URL: %s", tc.url)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg, "Error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, "Expected no error for URL: %s", tc.url)
			}
		})
	}
}

func TestURLValidator_isAllowedScheme(t *testing.T) {
	validator := NewURLValidator()

	testCases := []struct {
		scheme   string
		expected bool
	}{
		{"http", true},
		{"https", true},
		{"HTTP", true},
		{"HTTPS", true},
		{"ftp", false},
		{"file", false},
		{"gopher", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.scheme, func(t *testing.T) {
			result := validator.isAllowedScheme(tc.scheme)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestURLValidator_isBlockedIP(t *testing.T) {
	validator := NewURLValidator()

	testCases := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Public IPs (should not be blocked)
		{"public_ip_1", "8.8.8.8", false},
		{"public_ip_2", "1.1.1.1", false},
		{"public_ip_3", "208.67.222.222", false},

		// Private IPs (should be blocked)
		{"private_10", "10.0.0.1", true},
		{"private_172", "172.16.0.1", true},
		{"private_192", "192.168.1.1", true},

		// Loopback (should be blocked)
		{"loopback_127", "127.0.0.1", true},
		{"loopback_ipv6", "::1", true},

		// Link-local (should be blocked)
		{"link_local", "169.254.1.1", true},
		{"link_local_ipv6", "fe80::1", true},

		// Multicast (should be blocked)
		{"multicast_ipv4", "224.0.0.1", true},
		{"multicast_ipv6", "ff02::1", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			require.NotNil(t, ip, "Failed to parse IP: %s", tc.ip)

			result := validator.isBlockedIP(ip)
			assert.Equal(t, tc.expected, result, "IP %s should have blocked=%v", tc.ip, tc.expected)
		})
	}
}

func TestURLValidator_validateHostnamePatterns(t *testing.T) {
	validator := NewURLValidator()

	testCases := []struct {
		name        string
		hostname    string
		shouldError bool
		errorMsg    string
	}{
		// Valid hostnames
		{"valid_domain", "example.com", false, ""},
		{"valid_subdomain", "api.example.com", false, ""},
		{"valid_with_dash", "my-service.example.com", false, ""},

		// Localhost patterns
		{"localhost", "localhost", true, "localhost/loopback addresses not allowed"},
		{"localhost_caps", "LOCALHOST", true, "localhost/loopback addresses not allowed"},
		{"localhost_subdomain", "api.localhost", true, "localhost/loopback addresses not allowed"},
		{"contains_127", "test.127.0.0.1", true, "localhost/loopback addresses not allowed"},

		// Metadata services
		{"aws_metadata", "169.254.169.254", true, "metadata service endpoints not allowed"},
		{"gcp_metadata", "metadata.google.internal", true, "metadata service endpoints not allowed"},
		{"azure_metadata", "metadata.azure.com", true, "metadata service endpoints not allowed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.validateHostnamePatterns(tc.hostname)

			if tc.shouldError {
				require.Error(t, err, "Expected error for hostname: %s", tc.hostname)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err, "Expected no error for hostname: %s", tc.hostname)
			}
		})
	}
}

func TestGetDefaultBlockedNetworks(t *testing.T) {
	networks := getDefaultBlockedNetworks()

	// Should have multiple blocked networks
	assert.Greater(t, len(networks), 10, "Should have multiple blocked networks")

	// Check that some key networks are included
	expectedCIDRs := []string{
		"10.0.0.0/8",
		"127.0.0.0/8",
		"192.168.0.0/16",
		"172.16.0.0/12",
	}

	for _, expectedCIDR := range expectedCIDRs {
		found := false
		for _, network := range networks {
			if network.String() == expectedCIDR {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected CIDR %s should be in blocked networks", expectedCIDR)
	}
}

// Benchmark tests to ensure validation is performant
func BenchmarkURLValidator_Validate(b *testing.B) {
	validator := NewURLValidator()
	url := "https://example.com/path/to/config.yaml"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(url)
	}
}

func BenchmarkURLValidator_ValidateBlocked(b *testing.B) {
	validator := NewURLValidator()
	url := "http://127.0.0.1:8080/config.yaml"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(url)
	}
}
