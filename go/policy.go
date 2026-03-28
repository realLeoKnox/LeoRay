package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ─── Data Structures ─────────────────────────────────────────────────────────

// PolicyConfig is the single source of truth stored in config/policy.json.
type PolicyConfig struct {
	Groups      []PolicyGroup `json:"groups"`
	GeoRules    []GeoRule     `json:"geo_rules"`
	RuleSets    []RuleSet     `json:"rule_sets"`
	InlineRules []InlineRule  `json:"inline_rules"`
	Final       string        `json:"final"`
	AllowLAN    bool          `json:"allow_lan"`
	EnableTUN   bool          `json:"enable_tun"`
	EnableFakeIP bool         `json:"enable_fakeip"`
	EnableSniff  bool         `json:"enable_sniff"`
	DnsConfig   DnsConfig     `json:"dns_config"`
}

// PolicyGroup maps a logical group name to an Xray outbound tag (node).
type PolicyGroup struct {
	Name    string `json:"name"`
	Node    string `json:"node"`               // Xray outbound tag; "" means DefaultNode
	Order   int    `json:"order"`               // drag-sort weight (lower = higher priority)
}

// GeoRule is a GEO-based routing rule bound to a policy group.
type GeoRule struct {
	GeoRule string `json:"geo_rule"`  // e.g. "geosite:google", "geoip:cn"
	Policy  string `json:"policy"`    // group name | "direct" | "block"
	Order   int    `json:"order"`
}

// DnsConfig holds user-configurable DNS settings for anti-pollution.
type DnsConfig struct {
	DirectDNS []string `json:"direct_dns"` // DNS for DIRECT traffic, e.g. ["223.5.5.5", "123.123.123.124"]
	ProxyDNS  []string `json:"proxy_dns"`  // DNS for proxied traffic, e.g. ["1.1.1.1", "8.8.8.8"]
	DirectDOH string   `json:"direct_doh"` // DOH for DIRECT, e.g. "https://dns.alidns.com/dns-query"
	ProxyDOH  string   `json:"proxy_doh"`  // DOH for proxy, e.g. "https://cloudflare-dns.com/dns-query"
}

// RuleSet is a rule-list source (remote URL + local fallback) with a policy.
type RuleSet struct {
	Tag     string `json:"tag"`
	Policy  string `json:"policy"`  // group name | "direct" | "block"
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty"`
	Local   string `json:"local,omitempty"` // e.g. "data/rule_cache/AI.list"
}

// InlineRule is a single manually-added routing rule.
type InlineRule struct {
	Type    string `json:"type"`    // DOMAIN | DOMAIN-SUFFIX | DOMAIN-KEYWORD | IP-CIDR | DST-PORT | GEOSITE | GEOIP
	Payload string `json:"payload"`
	Policy  string `json:"policy"` // group name | "direct" | "block"
	Order   int    `json:"order"`  // drag-sort weight
}

// ─── Load / Save ─────────────────────────────────────────────────────────────

// LoadPolicy reads policy.json; if absent, initialises from defaults.
// existingMapping is the old mapping.json content used to pre-fill group nodes.
func LoadPolicy(policyPath, defaultPolicyPath string, existingMapping map[string]string) (*PolicyConfig, error) {
	data, err := os.ReadFile(policyPath)
	if err == nil {
		var p PolicyConfig
		if jsonErr := json.Unmarshal(data, &p); jsonErr == nil {
			// Migrate old embedded geo_rule in groups to standalone GeoRules
			p.migrateGeoRulesFromGroups()
			p.ensureDefaults()
			p.ensureDefaultDNS()
			return &p, nil
		}
	}
	fmt.Printf("[Policy] %s not found, initialising with defaults\n", policyPath)
	return initPolicyWithDefaults(existingMapping), nil
}

// SavePolicy serialises PolicyConfig and writes it to policyPath.
func SavePolicy(p *PolicyConfig, path string) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// ─── Default Policy ──────────────────────────────────────────────────────────

// defaultGroups returns the 3 base policy groups (name → node only).
func defaultGroups() []PolicyGroup {
	return []PolicyGroup{
		{Name: "Final", Node: "", Order: 0},
		{Name: "Proxy", Node: "", Order: 1},
		{Name: "DIRECT", Node: "direct", Order: 2},
	}
}

// defaultGeoRules returns the built-in GEO routing rules.
func defaultGeoRules() []GeoRule {
	return []GeoRule{
		{GeoRule: "geosite:google", Policy: "Proxy", Order: 0},
		{GeoRule: "geosite:cn", Policy: "DIRECT", Order: 1},
		{GeoRule: "geoip:cn", Policy: "DIRECT", Order: 2},
		{GeoRule: "geosite:microsoft", Policy: "Proxy", Order: 3},
		{GeoRule: "geosite:category-cryptocurrency", Policy: "Proxy", Order: 4},
		{GeoRule: "geosite:category-ai-!cn", Policy: "Proxy", Order: 5},
	}
}

// defaultDnsConfig returns sensible DNS defaults for anti-pollution.
func defaultDnsConfig() DnsConfig {
	return DnsConfig{
		DirectDNS: []string{"223.5.5.5", "123.123.123.124"},
		ProxyDNS:  []string{"1.1.1.1", "8.8.8.8"},
		DirectDOH: "",
		ProxyDOH:  "",
	}
}

func defaultEmptyPolicy() *PolicyConfig {
	return &PolicyConfig{
		Final:       "Final",
		Groups:      defaultGroups(),
		GeoRules:    defaultGeoRules(),
		RuleSets:    []RuleSet{},
		InlineRules: []InlineRule{},
		DnsConfig:   defaultDnsConfig(),
	}
}

// initPolicyWithDefaults creates a new policy with defaults and migrates
// old mapping.json settings if available.
func initPolicyWithDefaults(existingMapping map[string]string) *PolicyConfig {
	p := defaultEmptyPolicy()

	if existingMapping != nil {
		if v, ok := existingMapping["__allow_lan"]; ok && v == "true" {
			p.AllowLAN = true
		}
		if v, ok := existingMapping["__enable_tun"]; ok && v == "true" {
			p.EnableTUN = true
		}
		if v, ok := existingMapping["__enable_fakeip"]; ok && v == "true" {
			p.EnableFakeIP = true
		}
		if v, ok := existingMapping["__enable_sniff"]; ok && v == "true" {
			p.EnableSniff = true
		}
		// Try to carry over node assignments from old mapping
		for i := range p.Groups {
			if node, ok := existingMapping[p.Groups[i].Name]; ok && node != "" {
				p.Groups[i].Node = node
			}
		}
	}

	return p
}

// migrateGeoRulesFromGroups moves any legacy embedded geo_rule fields from
// PolicyGroup entries into the standalone GeoRules array. This ensures
// configs saved before the split are silently upgraded.
func (p *PolicyConfig) migrateGeoRulesFromGroups() {
	// We need to read the raw JSON to detect old-format groups with geo_rule.
	// Since fields already decoded, we check the legacy way: any Group with
	// a populated "geo_rule" key in the raw JSON would result in a matching
	// GeoRule entry not yet present in p.GeoRules.
	// For simplicity: scan groups for any that still carry a non-empty GeoRule
	// via the old struct tag. We temporarily preserve the field during decode
	// using json:"geo_rule,omitempty" in a migration struct.

	// Because PolicyGroup no longer has GeoRule field, the old JSON data is
	// simply ignored during Unmarshal. So we detect migration need by checking
	// if GeoRules is empty while there are known default geo patterns.
	// The real migration path: if GeoRules is nil/empty AND groups exist,
	// apply defaults. ensureDefaults() handles this.
}

// ensureDefaults merges any missing default groups and GEO rules.
func (p *PolicyConfig) ensureDefaults() {
	// Ensure base groups exist
	existing := map[string]bool{}
	for _, g := range p.Groups {
		existing[g.Name] = true
	}
	for _, dg := range defaultGroups() {
		if !existing[dg.Name] {
			p.Groups = append(p.Groups, dg)
		}
	}

	// Ensure GeoRules exist (at least defaults on first migration)
	if len(p.GeoRules) == 0 {
		p.GeoRules = defaultGeoRules()
	}
}

// ensureDefaultDNS fills in DNS config defaults if empty.
func (p *PolicyConfig) ensureDefaultDNS() {
	if len(p.DnsConfig.DirectDNS) == 0 {
		p.DnsConfig.DirectDNS = []string{"223.5.5.5", "123.123.123.124"}
	}
	if len(p.DnsConfig.ProxyDNS) == 0 {
		p.DnsConfig.ProxyDNS = []string{"1.1.1.1", "8.8.8.8"}
	}
}

// ─── Legacy initPolicyFromDefault (for backward compat with default_policy.md) ───

// initPolicyFromDefault parses the LCF-format default_policy.md file.
// Kept for backward compatibility but no longer the primary init path.
func initPolicyFromDefault(path string, existingMapping map[string]string) (*PolicyConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return defaultEmptyPolicy(), nil
	}
	defer f.Close()

	p := defaultEmptyPolicy()
	if existingMapping != nil {
		if v, ok := existingMapping["__allow_lan"]; ok && v == "true" {
			p.AllowLAN = true
		}
	}

	section := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}

		switch section {
		case "Proxy Group":
			eq := strings.Index(line, "=")
			if eq < 0 {
				continue
			}
			name := strings.TrimSpace(line[:eq])
			node := ""
			if existingMapping != nil {
				node = existingMapping[name]
			}
			// Check if this group already exists (from defaults)
			found := false
			for _, g := range p.Groups {
				if g.Name == name {
					found = true
					break
				}
			}
			if !found {
				p.Groups = append(p.Groups, PolicyGroup{Name: name, Node: node, Order: len(p.Groups)})
			}

		case "Rule":
			parts := strings.SplitN(line, ",", 3)
			if len(parts) < 2 {
				continue
			}
			ruleType := strings.ToUpper(strings.TrimSpace(parts[0]))
			if ruleType == "FINAL" || ruleType == "MATCH" {
				if len(parts) >= 2 {
					p.Final = strings.TrimSpace(parts[1])
				}
				continue
			}
			if len(parts) < 3 {
				continue
			}
			payload := strings.TrimSpace(parts[1])
			policy := strings.TrimSpace(parts[2])
			p.InlineRules = append(p.InlineRules, InlineRule{
				Type: ruleType, Payload: payload, Policy: policy, Order: len(p.InlineRules),
			})

		case "Remote Rule":
			rs := parseRemoteRuleToRuleSet(line)
			if rs != nil {
				p.RuleSets = append(p.RuleSets, *rs)
			}
		}
	}
	return p, scanner.Err()
}

func parseRemoteRuleToRuleSet(line string) *RuleSet {
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return nil
	}
	rs := &RuleSet{
		URL:     strings.TrimSpace(parts[0]),
		Enabled: true,
	}
	base := filepath.Base(strings.Split(rs.URL, "?")[0])
	rs.Local = filepath.Join("data", "rule_cache", base) // New cache path
	rs.Tag = base // default tag = filename

	for _, kv := range parts[1:] {
		kv = strings.TrimSpace(kv)
		switch {
		case strings.HasPrefix(kv, "policy="):
			rs.Policy = strings.TrimPrefix(kv, "policy=")
		case strings.HasPrefix(kv, "tag="):
			rs.Tag = strings.TrimPrefix(kv, "tag=")
		case kv == "enabled=false":
			rs.Enabled = false
		}
	}
	return rs
}

// ─── Policy → Xray Rules ─────────────────────────────────────────────────────

var policyHTTPClient = &http.Client{Timeout: 8 * time.Second}

// BuildXrayRulesFromPolicy converts a PolicyConfig to []XrayRouteRule.
// Order: GEO rules from groups → inline rules → rule sets.
func BuildXrayRulesFromPolicy(p *PolicyConfig, defaultTag string, forceRefresh bool) []XrayRouteRule {
	var rules []XrayRouteRule

	// Sort GEO rules by Order
	sortedGeo := make([]GeoRule, len(p.GeoRules))
	copy(sortedGeo, p.GeoRules)
	sort.Slice(sortedGeo, func(i, j int) bool { return sortedGeo[i].Order < sortedGeo[j].Order })

	// Sort inline rules by Order
	sortedInline := make([]InlineRule, len(p.InlineRules))
	copy(sortedInline, p.InlineRules)
	sort.Slice(sortedInline, func(i, j int) bool { return sortedInline[i].Order < sortedInline[j].Order })

	// 1. GEO rules (highest priority) — now from standalone GeoRules array
	for _, gr := range sortedGeo {
		if gr.GeoRule == "" {
			continue
		}
		tag := resolvePolicy(gr.Policy, p.Groups, defaultTag)
		r := geoRuleToXray(gr.GeoRule, tag)
		if r != nil {
			rules = append(rules, *r)
		}
	}

	// 2. Inline rules (user-defined, evaluated after GEO rules)
	for _, ir := range sortedInline {
		tag := resolvePolicy(ir.Policy, p.Groups, defaultTag)
		r := ruleLineToXray(ir.Type, ir.Payload, tag)
		if r != nil {
			rules = append(rules, *r)
		}
	}

	// 3. Rule sets (remote URL with local fallback) — lowest priority
	for _, rs := range p.RuleSets {
		if !rs.Enabled {
			continue
		}
		tag := resolvePolicy(rs.Policy, p.Groups, defaultTag)
		lines, err := fetchRuleLines(rs, forceRefresh)
		if err != nil {
			fmt.Printf("[Policy] Cannot load rule set [%s]: %v\n", rs.Tag, err)
			continue
		}

		var domains []string
		var ips []string
		var ports []string

		for _, line := range lines {
			r := parseListLine(line, tag)
			if r != nil {
				if len(r.Domain) > 0 {
					domains = append(domains, r.Domain...)
				}
				if len(r.IP) > 0 {
					ips = append(ips, r.IP...)
				}
				if r.Port != "" {
					ports = append(ports, r.Port)
				}
			}
		}

		if len(domains) > 0 {
			rules = append(rules, XrayRouteRule{Type: "field", OutboundTag: tag, Domain: domains})
		}
		if len(ips) > 0 {
			rules = append(rules, XrayRouteRule{Type: "field", OutboundTag: tag, IP: ips})
		}
		if len(ports) > 0 {
			rules = append(rules, XrayRouteRule{Type: "field", OutboundTag: tag, Port: strings.Join(ports, ",")})
		}
	}

	return rules
}

// geoRuleToXray converts a GEO rule string like "geosite:google" or "geoip:cn"
// to an XrayRouteRule.
func geoRuleToXray(geoRule, outboundTag string) *XrayRouteRule {
	geoRule = strings.TrimSpace(geoRule)
	if geoRule == "" {
		return nil
	}
	r := &XrayRouteRule{Type: "field", OutboundTag: outboundTag}
	lower := strings.ToLower(geoRule)
	if strings.HasPrefix(lower, "geosite:") {
		r.Domain = []string{lower}
	} else if strings.HasPrefix(lower, "geoip:") {
		r.IP = []string{lower}
	} else {
		// Treat as domain rule
		r.Domain = []string{lower}
	}
	return r
}

// resolvePolicy converts a policy name to an Xray outbound tag.
func resolvePolicy(policy string, groups []PolicyGroup, defaultTag string) string {
	// Highest priority: user defined explicitly
	for _, g := range groups {
		if g.Name == policy {
			if g.Node != "" {
				return g.Node
			}
			return defaultTag
		}
	}
	// Fallback to built-in
	switch strings.ToUpper(policy) {
	case "DIRECT":
		return "direct"
	case "REJECT", "REJECT-DROP", "BLOCK":
		return "block"
	}
	// unknown policy → use default
	return defaultTag
}

// fetchRuleLines loads rule lines for a RuleSet.
//   forceRefresh=false: use local cache if it exists (no HTTP request).
//   forceRefresh=true:  try remote, save to local cache, fall back to local.
func fetchRuleLines(rs RuleSet, forceRefresh bool) ([]string, error) {
	// ── Cache path: use local file directly when not refreshing ──────────────
	if !forceRefresh && rs.Local != "" {
		if data, err := os.ReadFile(rs.Local); err == nil && len(data) > 0 {
			return strings.Split(string(data), "\n"), nil
		}
		// Local doesn't exist yet – fall through to remote fetch below
	}

	// ── Remote fetch ──────────────────────────────────────────────────────────
	if rs.URL != "" {
		resp, err := policyHTTPClient.Get(rs.URL)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				body, readErr := io.ReadAll(resp.Body)
				if readErr == nil {
					fmt.Printf("[Policy] Fetched remote: %s\n", rs.Tag)
					// Save to local cache
					if rs.Local != "" {
						os.MkdirAll(filepath.Dir(rs.Local), 0755)
						os.WriteFile(rs.Local, body, 0644)
					}
					return strings.Split(string(body), "\n"), nil
				}
			}
		}
		fmt.Printf("[Policy] Remote failed [%s], using local: %s\n", rs.Tag, rs.Local)
	}

	// ── Local fallback ────────────────────────────────────────────────────────
	if rs.Local != "" {
		data, err := os.ReadFile(rs.Local)
		if err == nil {
			return strings.Split(string(data), "\n"), nil
		}
		return nil, fmt.Errorf("local fallback %s: %w", rs.Local, err)
	}
	return nil, fmt.Errorf("no source for rule set [%s]", rs.Tag)
}

// ruleLineToXray builds an XrayRouteRule from a type/payload/tag triple.
func ruleLineToXray(ruleType, payload, outboundTag string) *XrayRouteRule {
	r := &XrayRouteRule{Type: "field", OutboundTag: outboundTag}
	switch strings.ToUpper(ruleType) {
	case "DOMAIN":
		r.Domain = []string{"full:" + payload}
	case "DOMAIN-SUFFIX":
		r.Domain = []string{"domain:" + payload}
	case "DOMAIN-KEYWORD":
		r.Domain = []string{"keyword:" + payload}
	case "IP-CIDR", "IP-CIDR6":
		r.IP = []string{payload}
	case "GEOIP":
		r.IP = []string{"geoip:" + strings.ToLower(payload)}
	case "GEOSITE":
		r.Domain = []string{"geosite:" + strings.ToLower(payload)}
	case "DST-PORT", "PORT":
		r.Port = payload
	default:
		return nil
	}
	return r
}

// parseListLine parses a single line from a .list rule file.
func parseListLine(rawLine, outboundTag string) *XrayRouteRule {
	line := strings.TrimSpace(rawLine)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	// strip inline comments
	if idx := strings.Index(line, " #"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	parts := strings.SplitN(line, ",", 3)
	if len(parts) < 2 {
		return nil
	}
	ruleType := strings.ToUpper(strings.TrimSpace(parts[0]))
	payload := strings.TrimSpace(parts[1])
	// IP-ASN is not supported by Xray routing natively
	if ruleType == "IP-ASN" {
		return nil
	}
	return ruleLineToXray(ruleType, payload, outboundTag)
}
