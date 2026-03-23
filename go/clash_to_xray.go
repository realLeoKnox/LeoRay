package main

import (
	"strings"
)

// XrayRouteRule 定义了 Xray 路由规则的基础结构
type XrayRouteRule struct {
	Type        string   `json:"type"`
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Port        string   `json:"port,omitempty"`
	InboundTag  []string `json:"inboundTag,omitempty"`
	OutboundTag string   `json:"outboundTag"`
	Network     string   `json:"network,omitempty"`
}

// ParseClashRuleToXray 解析单个 Clash 规则字符串为 Xray 的路由规则对象
func ParseClashRuleToXray(clashRule string, policyToTagMap map[string]string, defaultFallbackTag string) *XrayRouteRule {
	clashRule = strings.TrimSpace(clashRule)
	if clashRule == "" || strings.HasPrefix(clashRule, "#") || strings.HasPrefix(clashRule, "//") {
		return nil // 忽略空行和注释行
	}

	parts := strings.Split(clashRule, ",")
	if len(parts) < 2 {
		return nil
	}

	ruleType := strings.ToUpper(strings.TrimSpace(parts[0]))
	
	if (ruleType == "MATCH" || ruleType == "FINAL") && len(parts) >= 2 {
		policy := strings.TrimSpace(parts[1])
		outboundTag := policyToTagMap[policy]
		if outboundTag == "" {
			outboundTag = defaultFallbackTag
		}
		return &XrayRouteRule{
			Type:        "field",
			OutboundTag: outboundTag,
			Network:     "tcp,udp",
		}
	}

	if len(parts) < 3 {
		return nil
	}

	payload := strings.TrimSpace(parts[1])
	policy := strings.TrimSpace(parts[2])

	outboundTag := policyToTagMap[policy]
	if outboundTag == "" {
		outboundTag = defaultFallbackTag // 对于未知策略（如 "手动选择"、"Apple" 等），统一 fallback 到默认节点
	}

	rule := &XrayRouteRule{
		Type:        "field",
		OutboundTag: outboundTag,
	}

	switch ruleType {
	case "DOMAIN":
		rule.Domain = []string{"full:" + payload}
	case "DOMAIN-SUFFIX":
		rule.Domain = []string{"domain:" + payload}
	case "DOMAIN-KEYWORD":
		rule.Domain = []string{"keyword:" + payload}
	case "IP-CIDR", "IP-CIDR6":
		rule.IP = []string{payload}
	case "GEOIP":
		rule.IP = []string{"geoip:" + strings.ToLower(payload)}
	case "GEOSITE":
		rule.Domain = []string{"geosite:" + strings.ToLower(payload)}
	case "DST-PORT", "PORT":
		rule.Port = payload
	default:
		return nil
	}

	return rule
}
