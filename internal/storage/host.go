package storage

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// NormalizeHost returns the eTLD+1 of host using the Public Suffix List,
// stripping subdomains accurately (e.g. a.b.com.cn -> b.com.cn, sub.github.io -> sub.github.io).
// Bare IPs and single-label names are returned unchanged.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return host
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if net.ParseIP(host) != nil {
		return host
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if !strings.Contains(host, ".") {
		return host
	}
	etldPlusOne, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return etldPlusOne
}
