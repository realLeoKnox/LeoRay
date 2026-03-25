package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── Data Structures ─────────────────────────────────────────────────────────

// PolicyConfig is the single source of truth stored in config/policy.json.
type PolicyConfig struct {
	Groups      []PolicyGroup `json:"groups"`
	RuleSets    []RuleSet     `json:"rule_sets"`
	InlineRules []InlineRule  `json:"inline_rules"`
	Final       string        `json:"final"`
	AllowLAN    bool          `json:"allow_lan"`
	EnableTUN   bool          `json:"enable_tun"`
	EnableFakeIP bool         `json:"enable_fakeip"`
	EnableSniff  bool         `json:"enable_sniff"`
}

// PolicyGroup maps a logical group name to an Xray outbound tag (node).
type PolicyGroup struct {
	Name string `json:"name"`
	Node string `json:"node"` // Xray outbound tag; "" means DefaultNode
}

// RuleSet is a rule-list source (remote URL + local fallback) with a policy.
type RuleSet struct {
	Tag     string `json:"tag"`
	Policy  string `json:"policy"`  // group name | "direct" | "block"
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty"`
	Local   string `json:"local,omitempty"` // e.g. "rule/AI.list"
}

// InlineRule is a single manually-added routing rule.
type InlineRule struct {
	Type    string `json:"type"`    // DOMAIN | DOMAIN-SUFFIX | DOMAIN-KEYWORD | IP-CIDR | DST-PORT
	Payload string `json:"payload"`
	Policy  string `json:"policy"` // group name | "direct" | "block"
}

// ─── Load / Save ─────────────────────────────────────────────────────────────

// LoadPolicy reads policy.json; if absent, initialises from defaultPolicyPath.
// existingMapping is the old mapping.json content used to pre-fill group nodes.
func LoadPolicy(policyPath, defaultPolicyPath string, existingMapping map[string]string) (*PolicyConfig, error) {
	data, err := os.ReadFile(policyPath)
	if err == nil {
		var p PolicyConfig
		if jsonErr := json.Unmarshal(data, &p); jsonErr == nil {
			return &p, nil
		}
	}
	fmt.Printf("[Policy] %s not found, initialising from %s\n", policyPath, defaultPolicyPath)
	return initPolicyFromDefault(defaultPolicyPath, existingMapping)
}

// SavePolicy serialises PolicyConfig and writes it to policyPath.
func SavePolicy(p *PolicyConfig, path string) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// ─── Initialise from default_policy.md ───────────────────────────────────────

// initPolicyFromDefault parses the LCF-format default_policy.md file.
func initPolicyFromDefault(path string, existingMapping map[string]string) (*PolicyConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return defaultEmptyPolicy(), nil
	}
	defer f.Close()

	// Migrate settings from old mapping.json if available
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
			p.Groups = append(p.Groups, PolicyGroup{Name: name, Node: node})

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
				Type: ruleType, Payload: payload, Policy: policy,
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
	rs.Local = filepath.Join("rule", base)
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

func defaultEmptyPolicy() *PolicyConfig {
	return &PolicyConfig{
		Final:       "direct",
		Groups:      []PolicyGroup{},
		RuleSets:    []RuleSet{},
		InlineRules: []InlineRule{},
	}
}

// ─── Policy → Xray Rules ─────────────────────────────────────────────────────

var policyHTTPClient = &http.Client{Timeout: 8 * time.Second}

// BuildXrayRulesFromPolicy converts a PolicyConfig to []XrayRouteRule.
// forceRefresh=false: use local cache (fast, safe during TUN restart).
// forceRefresh=true:  fetch remote URLs and update local cache.
func BuildXrayRulesFromPolicy(p *PolicyConfig, defaultTag string, forceRefresh bool) []XrayRouteRule {
	var rules []XrayRouteRule

	// 1. Inline rules (highest priority – user-defined, evaluated first)
	for _, ir := range p.InlineRules {
		tag := resolvePolicy(ir.Policy, p.Groups, defaultTag)
		r := ruleLineToXray(ir.Type, ir.Payload, tag)
		if r != nil {
			rules = append(rules, *r)
		}
	}

	// 2. Rule sets (remote URL with local fallback)
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
