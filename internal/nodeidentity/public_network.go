package nodeidentity

import (
	"net"
	"strings"
)

// NormalizePublicIPv4 returns a globally routable IPv4 address or an empty
// string. Local, VPC, CGNAT, loopback, link-local, documentation, and reserved
// ranges are intentionally rejected for node public_ip persistence.
func NormalizePublicIPv4(value string) string {
	host := strings.TrimSpace(value)
	if host == "" {
		return ""
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.Trim(host, "[]\"'")

	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	ip4 := ip.To4()
	if ip4 == nil || !isPublicIPv4(ip4) {
		return ""
	}
	return net.IP(ip4).String()
}

func NormalizePublicEndpoint(endpoint, publicIP string) string {
	endpoint = strings.TrimSpace(endpoint)
	publicIP = NormalizePublicIPv4(publicIP)
	if endpoint == "" {
		return ""
	}

	host, port, ok := splitEndpoint(endpoint)
	if !ok {
		ip := net.ParseIP(strings.Trim(endpoint, "[]"))
		if ip == nil {
			if strings.EqualFold(endpoint, "localhost") {
				return ""
			}
			return endpoint
		}
		ip4 := ip.To4()
		if ip4 != nil && isPublicIPv4(ip4) {
			return net.IP(ip4).String()
		}
		if publicIP != "" {
			return publicIP
		}
		return ""
	}
	if port == "" {
		if publicIP != "" {
			return publicIP
		}
		return endpoint
	}
	if host == "" {
		if publicIP == "" {
			return ""
		}
		return net.JoinHostPort(publicIP, port)
	}

	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		if strings.EqualFold(host, "localhost") {
			return ""
		}
		return endpoint
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	if isPublicIPv4(ip4) {
		return net.JoinHostPort(net.IP(ip4).String(), port)
	}
	if publicIP == "" {
		return ""
	}
	return net.JoinHostPort(publicIP, port)
}

func NormalizeReportedNetwork(publicIP, endpoint string) (string, string) {
	normalizedPublicIP := NormalizePublicIPv4(publicIP)
	normalizedEndpoint := NormalizePublicEndpoint(endpoint, normalizedPublicIP)
	if normalizedPublicIP == "" {
		normalizedPublicIP = NormalizePublicIPv4(normalizedEndpoint)
		if normalizedPublicIP == "" {
			host, _, ok := splitEndpoint(normalizedEndpoint)
			if ok {
				normalizedPublicIP = NormalizePublicIPv4(host)
			}
		}
	}
	return normalizedPublicIP, normalizedEndpoint
}

func splitEndpoint(endpoint string) (string, string, bool) {
	host, port, err := net.SplitHostPort(endpoint)
	if err == nil {
		return host, port, true
	}
	if strings.HasPrefix(endpoint, ":") && len(endpoint) > 1 {
		return "", strings.TrimPrefix(endpoint, ":"), true
	}
	if idx := strings.LastIndex(endpoint, ":"); idx > 0 && idx < len(endpoint)-1 && !strings.Contains(endpoint[idx+1:], ":") {
		return endpoint[:idx], endpoint[idx+1:], true
	}
	return endpoint, "", false
}

func isPublicIPv4(ip net.IP) bool {
	if len(ip) != net.IPv4len {
		return false
	}
	first, second, third := ip[0], ip[1], ip[2]
	switch {
	case first == 0:
		return false
	case first == 10:
		return false
	case first == 100 && second >= 64 && second <= 127:
		return false
	case first == 127:
		return false
	case first == 169 && second == 254:
		return false
	case first == 172 && second >= 16 && second <= 31:
		return false
	case first == 192 && second == 168:
		return false
	case first == 192 && second == 0 && third == 0:
		return false
	case first == 192 && second == 0 && third == 2:
		return false
	case first == 198 && (second == 18 || second == 19):
		return false
	case first == 198 && second == 51 && third == 100:
		return false
	case first == 203 && second == 0 && third == 113:
		return false
	case first >= 224:
		return false
	default:
		return true
	}
}
