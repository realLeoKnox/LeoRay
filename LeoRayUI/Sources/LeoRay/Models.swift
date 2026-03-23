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
    var ruleSets:     [RuleSet]
    var inlineRules:  [InlineRule]

    enum CodingKeys: String, CodingKey {
        case allowLan     = "allow_lan"
        case enableTun    = "enable_tun"
        case enableFakeip = "enable_fakeip"
        case enableSniff  = "enable_sniff"
        case finalPolicy  = "final"
        case groups
        case ruleSets     = "rule_sets"
        case inlineRules  = "inline_rules"
    }

    // Provide defaults for fields that may be absent in JSON
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        allowLan     = (try? c.decode(Bool.self,          forKey: .allowLan))     ?? false
        enableTun    = (try? c.decode(Bool.self,          forKey: .enableTun))    ?? false
        enableFakeip = (try? c.decode(Bool.self,          forKey: .enableFakeip)) ?? false
        enableSniff  = (try? c.decode(Bool.self,          forKey: .enableSniff))  ?? false
        finalPolicy  = (try? c.decode(String.self,        forKey: .finalPolicy))  ?? "direct"
        groups       = (try? c.decode([PolicyGroup].self, forKey: .groups))       ?? []
        ruleSets     = (try? c.decode([RuleSet].self,     forKey: .ruleSets))     ?? []
        inlineRules  = (try? c.decode([InlineRule].self,  forKey: .inlineRules))  ?? []
    }
}

// Go: PolicyGroup { Name, Node }
struct PolicyGroup: Codable, Identifiable {
    var id: String { name }
    var name: String
    var node: String  // "" means use defaultNode
}

// Go: RuleSet { Tag, Policy, Enabled, URL, Local }
struct RuleSet: Codable, Identifiable {
    var id: String { tag }
    var tag:     String
    var policy:  String   // group name | "direct" | "block"
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
}

// Go: InlineRule { Type, Payload, Policy }
struct InlineRule: Codable, Identifiable {
    var id = UUID()
    var type:    String
    var payload: String
    var policy:  String
    enum CodingKeys: String, CodingKey { case type, payload, policy }
}

// MARK: - Node Latency
struct NodeLatency: Codable {
    let tcpPing: String?   // Go returns strings like "123ms", "超时", "-1ms"
    let connect: String?
    let error:   String?
    enum CodingKeys: String, CodingKey {
        case tcpPing = "tcp_ping"
        case connect
        case error
    }
}

// MARK: - API Responses
struct ConfigResponse: Codable { let nodes: [String] }
struct LogsResponse:   Codable { let logs:  [String] }
