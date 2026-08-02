package policy

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/Tech-S1/kiosk-display/internal/links"
)

func ParseHTTP(raw string) *url.URL {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}
	return u
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func isLANIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func hostInList(host string, allowedHosts []string, autoAllowSubdomains bool) bool {
	host = normalizeHost(host)
	for _, allowed := range allowedHosts {
		allowed = normalizeHost(allowed)
		if allowed == "" {
			continue
		}
		if host == allowed {
			return true
		}
		if autoAllowSubdomains && strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func lanIPExplicitlyAllowed(ip net.IP, allowedHosts []string) bool {
	return ip != nil && hostInList(ip.String(), allowedHosts, false)
}

func isBlockedLANHost(host string, allowedHosts []string) bool {
	host = normalizeHost(host)
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || !isLANIP(ip) {
		return false
	}
	return !lanIPExplicitlyAllowed(ip, allowedHosts)
}

func resolvesToDisallowedLAN(host string, allowedHosts []string) bool {
	host = normalizeHost(host)
	if net.ParseIP(host) != nil {
		return false
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return true
	}
	for _, ip := range ips {
		if isLANIP(ip) && !lanIPExplicitlyAllowed(ip, allowedHosts) {
			return true
		}
	}
	return false
}

func isLoopbackHost(host string) bool {
	switch normalizeHost(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	}
}

func urlPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func IsDisplayUI(raw string, displayPort int) bool {
	u := ParseHTTP(raw)
	if u == nil || displayPort < 1 {
		return false
	}
	if !isLoopbackHost(u.Hostname()) {
		return false
	}
	return urlPort(u) == strconv.Itoa(displayPort)
}

func hostAllowed(host string, allowedHosts []string, autoAllowSubdomains bool) bool {
	host = normalizeHost(host)
	if isBlockedLANHost(host, allowedHosts) {
		return false
	}
	return hostInList(host, allowedHosts, autoAllowSubdomains)
}

func NavURLAllowed(raw string, items []links.Item, allowedHosts []string, autoAllowLinkHosts, autoAllowSubdomains bool, displayPort int) bool {
	u := ParseHTTP(raw)
	if u == nil {
		return false
	}
	if IsDisplayUI(raw, displayPort) {
		return true
	}
	host := normalizeHost(u.Hostname())
	if isLoopbackHost(host) {
		return false
	}
	if isBlockedLANHost(host, allowedHosts) || resolvesToDisallowedLAN(host, allowedHosts) {
		return false
	}
	if hostAllowed(host, allowedHosts, autoAllowSubdomains) {
		return true
	}
	if !autoAllowLinkHosts {
		return false
	}
	for _, item := range items {
		if linkHostAllowed(raw, item.URL, allowedHosts) {
			return true
		}
	}
	return false
}

func linkHostAllowed(raw, linkRaw string, allowedHosts []string) bool {
	target := ParseHTTP(raw)
	link := ParseHTTP(linkRaw)
	if target == nil || link == nil {
		return false
	}
	linkHost := normalizeHost(link.Hostname())
	if isBlockedLANHost(linkHost, allowedHosts) || isLoopbackHost(linkHost) {
		return false
	}
	if normalizeHost(target.Hostname()) != linkHost {
		return false
	}
	return target.Scheme == link.Scheme && urlPort(target) == urlPort(link)
}

func normalizeURLPath(p string) string {
	p = strings.TrimRight(strings.TrimSpace(p), "/")
	if p == "" {
		return "/"
	}
	return p
}

func urlsMatchExact(raw, linkRaw string) bool {
	target := ParseHTTP(raw)
	link := ParseHTTP(linkRaw)
	if target == nil || link == nil {
		return false
	}
	if normalizeHost(target.Hostname()) != normalizeHost(link.Hostname()) {
		return false
	}
	if target.Scheme != link.Scheme || urlPort(target) != urlPort(link) {
		return false
	}
	return normalizeURLPath(target.Path) == normalizeURLPath(link.Path)
}

func ExactLinkLabel(raw string, items []links.Item) string {
	raw = strings.TrimSpace(raw)
	for _, item := range items {
		label := strings.TrimSpace(item.Label)
		if label == "" || !urlsMatchExact(raw, item.URL) {
			continue
		}
		return label
	}
	return ""
}

func PageURLAllowed(raw string, items []links.Item, allowedHosts []string, autoAllowLinkHosts, autoAllowSubdomains bool, displayPort int) bool {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	switch {
	case raw == "", lower == "about:blank", strings.HasPrefix(lower, "about:blank#"):
		return true
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return NavURLAllowed(raw, items, allowedHosts, autoAllowLinkHosts, autoAllowSubdomains, displayPort)
	default:
		return false
	}
}
