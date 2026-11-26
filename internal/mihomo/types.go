package mihomo

import (
	"net"
	"path/filepath"
	"strings"
)

const (
	DirectOutboundName = "DIRECT"
	UnknownRuleGroup   = "unknown"

	DNSModeNormal           = "normal"
	DNSModeFakeIP           = "fake-ip"
	DNSModeFakeIPUnresolved = "fake-ip-unresolved"
	DNSModeRedirHost        = "redir-host"
)

// Settings holds connection credentials for a Mihomo controller API.
type Settings struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

// Normalize trims trailing slashes and whitespace from Mihomo settings.
func (s Settings) Normalize() Settings {
	s.URL = strings.TrimRight(strings.TrimSpace(s.URL), "/")
	s.Secret = strings.TrimSpace(s.Secret)
	return s
}

// VersionInfo represents the payload returned by Mihomo GET /version.
type VersionInfo struct {
	Version string `json:"version"`
	Meta    bool   `json:"meta"`
	Premium bool   `json:"premium"`
}

// KernelConfig captures relevant fields from Mihomo GET /configs.
type KernelConfig struct {
	Mode      string `json:"mode"`
	AllowLan  bool   `json:"allow-lan"`
	MixedPort int    `json:"mixed-port"`
	Tun       struct {
		Enable bool   `json:"enable"`
		Stack  string `json:"stack"`
	} `json:"tun"`
	DNS struct {
		Enable       bool   `json:"enable"`
		EnhancedMode string `json:"enhanced-mode"`
		FakeIPRange  string `json:"fake-ip-range"`
		FakeIPRange6 string `json:"fake-ip-range6"`
	} `json:"dns"`
}

// Metadata contains full connection metadata exposed by Mihomo /connections.
type Metadata struct {
	Network           string `json:"network"`
	Type              string `json:"type"`
	SourceIP          string `json:"sourceIP"`
	SourcePort        string `json:"sourcePort"`
	DestinationIP     string `json:"destinationIP"`
	DestinationPort   string `json:"destinationPort"`
	RemoteDestination string `json:"remoteDestination"`
	Host              string `json:"host"`
	SniffHost         string `json:"sniffHost"`
	DNSMode           string `json:"dnsMode"`
	Process           string `json:"process"`
	ProcessPath       string `json:"processPath"`
}

// Connection represents a single active connection entry from Mihomo.
type Connection struct {
	ID          string   `json:"id"`
	Upload      int64    `json:"upload"`
	Download    int64    `json:"download"`
	Start       string   `json:"start"`
	Chains      []string `json:"chains"`
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
	Metadata    Metadata `json:"metadata"`
}

// ConnectionsSnapshot represents the response from Mihomo GET or WS /connections.
type ConnectionsSnapshot struct {
	Connections   []Connection `json:"connections"`
	UploadTotal   int64        `json:"uploadTotal"`
	DownloadTotal int64        `json:"downloadTotal"`
	Memory        int64        `json:"memory"`
}

// ProxyGroup represents a controllable Mihomo select/fallback proxy group.
type ProxyGroup struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Now  string   `json:"now"`
	All  []string `json:"all"`
}

type proxyInfo struct {
	Type string   `json:"type"`
	Now  string   `json:"now"`
	All  []string `json:"all"`
}

type proxiesResponse struct {
	Proxies map[string]proxyInfo `json:"proxies"`
}

// ResolvedConnection is a normalized connection with Fake-IP attribution resolved.
type ResolvedConnection struct {
	ID              string
	SourceIP        string
	Host            string
	DestinationIP   string
	DestinationPort string
	Process         string
	ProcessPath     string
	Outbound        string
	Chains          []string
	RuleGroup       string
	DNSMode         string
	Network         string
	InboundType     string
	SniffHost       string
	Start           string
	Upload          int64
	Download        int64
}

// FakeIPDetector identifies whether an IP belongs to Mihomo's Fake-IP pools.
type FakeIPDetector struct {
	nets []*net.IPNet
}

// NewFakeIPDetector creates a detector seeded with default Mihomo Fake-IP CIDRs
// plus any custom CIDRs obtained from Mihomo /configs.
func NewFakeIPDetector(extraCIDRs ...string) *FakeIPDetector {
	defaults := []string{
		"198.18.0.0/15",
		"fc00::/18",
	}
	nets := make([]*net.IPNet, 0, len(defaults)+len(extraCIDRs))
	seen := make(map[string]struct{})
	for _, raw := range append(defaults, extraCIDRs...) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(raw)
		if err != nil {
			continue
		}
		key := ipNet.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		nets = append(nets, ipNet)
	}
	return &FakeIPDetector{nets: nets}
}

// IsFakeIP reports whether ipStr parses as an IP inside any configured Fake-IP CIDR.
func (d *FakeIPDetector) IsFakeIP(ipStr string) bool {
	if d == nil {
		return false
	}
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	for _, cidr := range d.nets {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// ResolveConnection normalizes a raw Mihomo connection with Fake-IP attribution.
func (d *FakeIPDetector) ResolveConnection(conn Connection) ResolvedConnection {
	meta := conn.Metadata
	rawHost := strings.TrimSpace(meta.Host)
	sniffHost := strings.TrimSpace(meta.SniffHost)
	remoteDest := strings.TrimSpace(meta.RemoteDestination)
	destIP := strings.TrimSpace(meta.DestinationIP)

	rawDNSMode := strings.ToLower(strings.TrimSpace(meta.DNSMode))
	isFakeIP := rawDNSMode == DNSModeFakeIP || d.IsFakeIP(destIP)

	var dnsMode string
	switch {
	case isFakeIP:
		if rawHost == "" && sniffHost == "" {
			dnsMode = DNSModeFakeIPUnresolved
		} else {
			dnsMode = DNSModeFakeIP
		}
	case rawDNSMode == DNSModeRedirHost:
		dnsMode = DNSModeRedirHost
	default:
		dnsMode = DNSModeNormal
	}

	host := firstNonEmpty(rawHost, sniffHost, remoteDest, destIP, "Unknown")

	displayDestIP := destIP
	if remoteDest != "" && (displayDestIP == "" || d.IsFakeIP(displayDestIP)) {
		displayDestIP = remoteDest
	}

	process := strings.TrimSpace(meta.Process)
	if process == "" && strings.TrimSpace(meta.ProcessPath) != "" {
		process = filepath.Base(strings.TrimSpace(meta.ProcessPath))
	}
	if process == "" {
		process = "Unknown"
	}

	network := strings.ToLower(strings.TrimSpace(meta.Network))
	if network == "" {
		network = "unknown"
	}

	return ResolvedConnection{
		ID:              conn.ID,
		SourceIP:        defaultString(meta.SourceIP, "Inner"),
		Host:            host,
		DestinationIP:   displayDestIP,
		DestinationPort: strings.TrimSpace(meta.DestinationPort),
		Process:         process,
		ProcessPath:     strings.TrimSpace(meta.ProcessPath),
		Outbound:        OutboundName(conn.Chains),
		Chains:          SanitizeChains(conn.Chains),
		RuleGroup:       RuleGroupLabel(conn.Rule, conn.RulePayload),
		DNSMode:         dnsMode,
		Network:         network,
		InboundType:     strings.TrimSpace(meta.Type),
		SniffHost:       sniffHost,
		Start:           conn.Start,
		Upload:          conn.Upload,
		Download:        conn.Download,
	}
}

// IsFakeIPMode reports whether a resolved dnsMode represents Fake-IP traffic.
func IsFakeIPMode(dnsMode string) bool {
	return dnsMode == DNSModeFakeIP || dnsMode == DNSModeFakeIPUnresolved
}

// RuleGroupLabel derives a normalized display group from a Mihomo matched rule.
func RuleGroupLabel(rule, rulePayload string) string {
	rule = strings.ToLower(strings.TrimSpace(rule))
	rulePayload = strings.TrimSpace(rulePayload)
	if rule == "" {
		return UnknownRuleGroup
	}
	if rulePayload == "" {
		return rule
	}
	return rule + ":" + rulePayload
}

// OutboundName returns the final outbound hop from Mihomo chains.
func OutboundName(chains []string) string {
	if len(chains) == 0 || strings.TrimSpace(chains[0]) == "" {
		return DirectOutboundName
	}
	return strings.TrimSpace(chains[0])
}

// SanitizeChains trims whitespace and guarantees at least ["DIRECT"].
func SanitizeChains(chains []string) []string {
	if len(chains) == 0 {
		return []string{DirectOutboundName}
	}
	cleaned := make([]string, 0, len(chains))
	for _, c := range chains {
		c = strings.TrimSpace(c)
		if c != "" {
			cleaned = append(cleaned, c)
		}
	}
	if len(cleaned) == 0 {
		return []string{DirectOutboundName}
	}
	return cleaned
}

// IsDirectConnection reports whether any hop in chains is DIRECT.
func IsDirectConnection(chains []string) bool {
	for _, hop := range SanitizeChains(chains) {
		if strings.EqualFold(hop, DirectOutboundName) {
			return true
		}
	}
	return false
}

// NormalizeProxyGroupType returns "select" or "fallback" for controllable groups.
func NormalizeProxyGroupType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "selector", "select":
		return "select"
	case "fallback":
		return "fallback"
	default:
		return ""
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
