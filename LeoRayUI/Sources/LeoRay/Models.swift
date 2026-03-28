import Foundation

// MARK: - Xray Status
struct XrayStatus: Codable {
    let running: Bool
    let pid: Int
    let uptime: String
}

// MARK: - Outbound Node
struct OutboundNode: Codable, Identifiable {
    var id: String { tag }
    let tag: String
    let `protocol`: String

    enum CodingKeys: String, CodingKey {
        case tag
        case `protocol` = "protocol"
    }

    var protocolBadge: String { `protocol`.uppercased() }
}

// MARK: - Policy
struct PolicyConfig: Codable {
    var allowLan:     Bool
    var enableTun:    Bool
    var enableFakeip: Bool
    var enableSniff:  Bool
    var finalPolicy:  String
    var groups:       [PolicyGroup]
    var geoRules:     [GeoRule]
    var ruleSets:     [RuleSet]
    var inlineRules:  [InlineRule]
    var dnsConfig:    DnsConfig

    enum CodingKeys: String, CodingKey {
        case allowLan     = "allow_lan"
        case enableTun    = "enable_tun"
        case enableFakeip = "enable_fakeip"
        case enableSniff  = "enable_sniff"
        case finalPolicy  = "final"
        case groups
        case geoRules     = "geo_rules"
        case ruleSets     = "rule_sets"
        case inlineRules  = "inline_rules"
        case dnsConfig    = "dns_config"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        allowLan     = (try? c.decode(Bool.self,          forKey: .allowLan))     ?? false
        enableTun    = (try? c.decode(Bool.self,          forKey: .enableTun))    ?? false
        enableFakeip = (try? c.decode(Bool.self,          forKey: .enableFakeip)) ?? false
        enableSniff  = (try? c.decode(Bool.self,          forKey: .enableSniff))  ?? false
        finalPolicy  = (try? c.decode(String.self,        forKey: .finalPolicy))  ?? "direct"
        groups       = (try? c.decode([PolicyGroup].self, forKey: .groups))       ?? []
        geoRules     = (try? c.decode([GeoRule].self,     forKey: .geoRules))     ?? []
        ruleSets     = (try? c.decode([RuleSet].self,     forKey: .ruleSets))     ?? []
        inlineRules  = (try? c.decode([InlineRule].self,  forKey: .inlineRules))  ?? []
        dnsConfig    = (try? c.decode(DnsConfig.self,     forKey: .dnsConfig))    ?? DnsConfig()
    }
}

// MARK: - Policy Group
// Go: PolicyGroup { Name, Node, Order }
struct PolicyGroup: Codable, Identifiable {
    var id: String { name }
    var name:    String
    var node:    String   // "" means use defaultNode
    var order:   Int      // drag-sort weight

    enum CodingKeys: String, CodingKey {
        case name
        case node
        case order
    }

    init(name: String, node: String = "", order: Int = 0) {
        self.name    = name
        self.node    = node
        self.order   = order
    }

    init(from decoder: Decoder) throws {
        let c    = try decoder.container(keyedBy: CodingKeys.self)
        name     = (try? c.decode(String.self, forKey: .name))    ?? ""
        node     = (try? c.decode(String.self, forKey: .node))    ?? ""
        order    = (try? c.decode(Int.self,    forKey: .order))   ?? 0
    }
}

// MARK: - GEO Rule
// Go: GeoRule { GeoRule, Policy, Order }
struct GeoRule: Codable, Identifiable {
    var id: String { geoRule }
    var geoRule: String   // e.g. "geosite:google", "geoip:cn"
    var policy:  String   // group name | "direct" | "block"
    var order:   Int

    enum CodingKeys: String, CodingKey {
        case geoRule = "geo_rule"
        case policy
        case order
    }

    init(geoRule: String, policy: String, order: Int = 0) {
        self.geoRule = geoRule
        self.policy  = policy
        self.order   = order
    }

    init(from decoder: Decoder) throws {
        let c    = try decoder.container(keyedBy: CodingKeys.self)
        geoRule  = (try? c.decode(String.self, forKey: .geoRule)) ?? ""
        policy   = (try? c.decode(String.self, forKey: .policy))  ?? "direct"
        order    = (try? c.decode(Int.self,    forKey: .order))   ?? 0
    }
}

// MARK: - DNS Config
// Go: DnsConfig { DirectDNS, ProxyDNS, DirectDOH, ProxyDOH }
struct DnsConfig: Codable {
    var directDns: [String]
    var proxyDns:  [String]
    var directDoh: String
    var proxyDoh:  String

    enum CodingKeys: String, CodingKey {
        case directDns = "direct_dns"
        case proxyDns  = "proxy_dns"
        case directDoh = "direct_doh"
        case proxyDoh  = "proxy_doh"
    }

    init(directDns: [String] = ["223.5.5.5", "123.123.123.124"],
         proxyDns: [String] = ["1.1.1.1", "8.8.8.8"],
         directDoh: String = "",
         proxyDoh: String = "") {
        self.directDns = directDns
        self.proxyDns  = proxyDns
        self.directDoh = directDoh
        self.proxyDoh  = proxyDoh
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        directDns = (try? c.decode([String].self, forKey: .directDns)) ?? ["223.5.5.5", "123.123.123.124"]
        proxyDns  = (try? c.decode([String].self, forKey: .proxyDns))  ?? ["1.1.1.1", "8.8.8.8"]
        directDoh = (try? c.decode(String.self,   forKey: .directDoh)) ?? ""
        proxyDoh  = (try? c.decode(String.self,   forKey: .proxyDoh))  ?? ""
    }
}

// MARK: - Rule Set
// Go: RuleSet { Tag, Policy, Enabled, URL, Local }
struct RuleSet: Codable, Identifiable {
    var id: String { tag }
    var tag:     String
    var policy:  String
    var enabled: Bool
    var url:     String
    var local:   String

    enum CodingKeys: String, CodingKey {
        case tag, policy, enabled, url, local
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        tag     = (try? c.decode(String.self, forKey: .tag))     ?? ""
        policy  = (try? c.decode(String.self, forKey: .policy))  ?? ""
        enabled = (try? c.decode(Bool.self,   forKey: .enabled)) ?? true
        url     = (try? c.decode(String.self, forKey: .url))     ?? ""
        local   = (try? c.decode(String.self, forKey: .local))   ?? ""
    }

    init(tag: String, policy: String, enabled: Bool, url: String, local: String) {
        self.tag     = tag
        self.policy  = policy
        self.enabled = enabled
        self.url      = url
        self.local    = local
    }
}

// MARK: - Inline Rule
// Go: InlineRule { Type, Payload, Policy, Order }
struct InlineRule: Codable, Identifiable {
    var id = UUID()
    var type:    String
    var payload: String
    var policy:  String
    var order:   Int

    enum CodingKeys: String, CodingKey { case type, payload, policy, order }

    init(type: String, payload: String, policy: String, order: Int = 0) {
        self.type    = type
        self.payload = payload
        self.policy  = policy
        self.order   = order
    }

    init(from decoder: Decoder) throws {
        let c    = try decoder.container(keyedBy: CodingKeys.self)
        type     = (try? c.decode(String.self, forKey: .type))    ?? "DOMAIN"
        payload  = (try? c.decode(String.self, forKey: .payload)) ?? ""
        policy   = (try? c.decode(String.self, forKey: .policy))  ?? "direct"
        order    = (try? c.decode(Int.self,    forKey: .order))   ?? 0
    }
}

// MARK: - Subscription
// Go: Subscription { ID, Name, URL, LastUpdated, NodeCount }
struct Subscription: Codable, Identifiable {
    var id:          String
    var name:        String
    var url:         String
    var lastUpdated: String
    var nodeCount:   Int

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case url
        case lastUpdated = "last_updated"
        case nodeCount   = "node_count"
    }

    init(from decoder: Decoder) throws {
        let c       = try decoder.container(keyedBy: CodingKeys.self)
        id          = (try? c.decode(String.self, forKey: .id))          ?? ""
        name        = (try? c.decode(String.self, forKey: .name))        ?? ""
        url         = (try? c.decode(String.self, forKey: .url))         ?? ""
        lastUpdated = (try? c.decode(String.self, forKey: .lastUpdated)) ?? ""
        nodeCount   = (try? c.decode(Int.self,    forKey: .nodeCount))   ?? 0
    }
}

// MARK: - Node Latency
struct NodeLatency: Codable {
    let tcpPing: String?
    let connect: String?
    let error:   String?
    enum CodingKeys: String, CodingKey {
        case tcpPing = "tcp_ping"
        case connect
        case error
    }
}

struct RouteTestResult: Codable {
    var outbound: String
    var rule: String?
    var error: String?
}

// MARK: - API Responses
struct ConfigResponse: Codable { let nodes: [String] }
struct LogsResponse:   Codable { let logs:  [String] }
