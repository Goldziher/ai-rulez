package remote

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// URLValidator provides SSRF protection for remote URLs
type URLValidator struct {
	blockedNetworks []net.IPNet
	allowedSchemes  []string
}

// NewURLValidator creates a new URL validator with default SSRF protection
func NewURLValidator() *URLValidator {
	return &URLValidator{
		blockedNetworks: getDefaultBlockedNetworks(),
		allowedSchemes:  []string{"https", "http"},
	}
}

// Validate checks if a URL is safe to fetch, preventing SSRF attacks
func (v *URLValidator) Validate(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("empty URL not allowed")
	}

	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Validate scheme
	if !v.isAllowedScheme(parsedURL.Scheme) {
		return fmt.Errorf("scheme %q not allowed (must be http or https)", parsedURL.Scheme)
	}

	// Extract hostname and port
	hostname := parsedURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("missing hostname in URL")
	}

	// Validate port range if specified
	if portStr := parsedURL.Port(); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port in URL: %w", err)
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("port %d out of valid range (1-65535)", port)
		}
	}

	// Resolve hostname to IP addresses
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname %q: %w", hostname, err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses found for hostname %q", hostname)
	}

	// Check all resolved IPs against blocked networks
	for _, ip := range ips {
		if v.isBlockedIP(ip) {
			return fmt.Errorf("IP address %s is blocked (hostname: %s)", ip.String(), hostname)
		}
	}

	// Additional checks for suspicious patterns
	return v.validateHostnamePatterns(hostname)
}

// isAllowedScheme checks if the URL scheme is permitted
func (v *URLValidator) isAllowedScheme(scheme string) bool {
	scheme = strings.ToLower(scheme)
	for _, allowed := range v.allowedSchemes {
		if scheme == allowed {
			return true
		}
	}
	return false
}

// isBlockedIP checks if an IP address is in any of the blocked networks
func (v *URLValidator) isBlockedIP(ip net.IP) bool {
	for _, network := range v.blockedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// validateHostnamePatterns performs additional hostname validation
func (v *URLValidator) validateHostnamePatterns(hostname string) error {
	hostname = strings.ToLower(hostname)

	// Block localhost variations
	localhostPatterns := []string{
		"localhost",
		"127.",
		"::1",
		"0.0.0.0",
	}

	for _, pattern := range localhostPatterns {
		if strings.Contains(hostname, pattern) {
			return fmt.Errorf("localhost/loopback addresses not allowed: %s", hostname)
		}
	}

	// Block metadata service endpoints (cloud providers)
	metadataPatterns := []string{
		"169.254.169.254", // AWS, Azure, GCP metadata service
		"metadata.google.internal",
		"metadata.azure.com",
	}

	for _, pattern := range metadataPatterns {
		if strings.Contains(hostname, pattern) {
			return fmt.Errorf("metadata service endpoints not allowed: %s", hostname)
		}
	}

	return nil
}

// getDefaultBlockedNetworks returns the default list of blocked IP networks
func getDefaultBlockedNetworks() []net.IPNet {
	blockedCIDRs := []string{
		// IPv4 Private Networks (RFC 1918)
		"10.0.0.0/8",     // Class A private
		"172.16.0.0/12",  // Class B private
		"192.168.0.0/16", // Class C private

		// IPv4 Special Use (RFC 5735)
		"127.0.0.0/8",    // Loopback
		"169.254.0.0/16", // Link-local (APIPA)
		"224.0.0.0/4",    // Multicast
		"240.0.0.0/4",    // Reserved

		// IPv6 Special Use (RFC 4291, RFC 3513)
		"::1/128",   // IPv6 loopback
		"fe80::/10", // IPv6 link-local
		"ff00::/8",  // IPv6 multicast
		"fc00::/7",  // IPv6 unique local addresses

		// Additional blocked ranges
		"0.0.0.0/8",     // "This" network
		"100.64.0.0/10", // Carrier-grade NAT (RFC 6598)
		"198.18.0.0/15", // Benchmarking (RFC 2544)
	}

	var networks []net.IPNet
	for _, cidr := range blockedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// This should never happen with hardcoded CIDRs
			panic(fmt.Sprintf("invalid CIDR in blocked networks: %s", cidr))
		}
		networks = append(networks, *network)
	}

	return networks
}
