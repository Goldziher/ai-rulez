package remote

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type URLValidator struct {
	blockedNetworks []net.IPNet
	allowedSchemes  []string
}

func newURLValidator() *URLValidator {
	return &URLValidator{
		blockedNetworks: getDefaultBlockedNetworks(),
		allowedSchemes:  []string{"https", "http"},
	}
}

func (v *URLValidator) Validate(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("empty URL not allowed")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if !v.isAllowedScheme(parsedURL.Scheme) {
		return fmt.Errorf("scheme %q not allowed (must be http or https)", parsedURL.Scheme)
	}

	hostname := parsedURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("missing hostname in URL")
	}

	if portStr := parsedURL.Port(); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port in URL: %w", err)
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("port %d out of valid range (1-65535)", port)
		}
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname %q: %w", hostname, err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses found for hostname %q", hostname)
	}

	for _, ip := range ips {
		if v.isBlockedIP(ip) {
			return fmt.Errorf("IP address %s is blocked (hostname: %s)", ip.String(), hostname)
		}
	}

	return v.validateHostnamePatterns(hostname)
}

func (v *URLValidator) isAllowedScheme(scheme string) bool {
	scheme = strings.ToLower(scheme)
	for _, allowed := range v.allowedSchemes {
		if scheme == allowed {
			return true
		}
	}
	return false
}

func (v *URLValidator) isBlockedIP(ip net.IP) bool {
	for _, network := range v.blockedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (v *URLValidator) validateHostnamePatterns(hostname string) error {
	hostname = strings.ToLower(hostname)

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

	metadataPatterns := []string{
		"169.254.169.254",
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

func getDefaultBlockedNetworks() []net.IPNet {
	blockedCIDRs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",

		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"240.0.0.0/4",

		"::1/128",
		"fe80::/10",
		"ff00::/8",
		"fc00::/7",

		"0.0.0.0/8",
		"100.64.0.0/10",
		"198.18.0.0/15",
	}

	var networks []net.IPNet
	for _, cidr := range blockedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR in blocked networks: %s", cidr))
		}
		networks = append(networks, *network)
	}

	return networks
}
