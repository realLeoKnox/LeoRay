import Foundation
import Combine

class APIClient: ObservableObject {
    static let baseURL = "http://127.0.0.1:3406"

    @Published var status:        XrayStatus?
    @Published var nodes:         [OutboundNode] = []
    @Published var nodeNames:     [String] = []
    @Published var policy:        PolicyConfig?
    @Published var logs:          [String] = []
    @Published var latencies:     [Int: NodeLatency] = [:]
    @Published var subscriptions: [Subscription] = []
    @Published var isConnected = false

    private var statusTimer: Timer?
    private var logTimer:    Timer?

    // MARK: - Polling
    func startPolling() {
        statusTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { [weak self] _ in
            self?.refreshStatus()
        }
        logTimer = Timer.scheduledTimer(withTimeInterval: 2.0, repeats: true) { [weak self] _ in
            self?.refreshLogs()
        }
        refreshStatus()
        refreshLogs()
    }

    func stopPolling() {
        statusTimer?.invalidate(); statusTimer = nil
        logTimer?.invalidate();    logTimer    = nil
    }

    private func refreshStatus() {
        Task { @MainActor in
            if let s = try? await fetchStatus() { status = s; isConnected = true }
        }
    }

    private func refreshLogs() {
        Task { @MainActor in
            if let l = try? await fetchLogs() { logs = l }
        }
    }

    // MARK: - Status / Logs
    func fetchStatus() async throws -> XrayStatus {
        let v: XrayStatus = try await get("/api/status")
        await MainActor.run { status = v; isConnected = true }
        return v
    }

    func fetchLogs() async throws -> [String] {
        let resp: LogsResponse = try await get("/api/logs")
        return resp.logs
    }

    // MARK: - Nodes
    func fetchNodes() async throws {
        let v: [OutboundNode] = try await get("/api/nodes")
        await MainActor.run { nodes = v }
    }

    func fetchNodeNames() async throws {
        let resp: ConfigResponse = try await get("/api/config")
        await MainActor.run { nodeNames = resp.nodes }
    }

    // MARK: - Policy
    func fetchPolicy() async throws {
        let v: PolicyConfig = try await get("/api/policy")
        await MainActor.run { policy = v }
    }

    func savePolicy(_ p: PolicyConfig) async throws {
        let data = try JSONEncoder().encode(p)
        try await postData("/api/policy", data: data)
        await MainActor.run { policy = p }
    }

    func refreshRules() async throws {
        try await post("/api/policy/refresh", body: [String: String]())
    }

    // MARK: - Proxy Control
    func startProxy() async throws {
        try await post("/api/start", body: [String: String]())
    }

    func stopProxy() async throws {
        try await post("/api/stop", body: [String: String]())
    }

    // MARK: - Node Testing
    func testNode(index: Int) async throws -> NodeLatency {
        let data = try JSONEncoder().encode(["index": index])
        let resp: NodeLatency = try await postDataDecoding("/api/test_node", data: data, timeout: 15)
        await MainActor.run { latencies[index] = resp }
        return resp
    }

    func testRoute(target: String, method: String) async throws -> RouteTestResult {
        let params = ["target": target, "method": method]
        let data   = try JSONEncoder().encode(params)
        return try await postDataDecoding("/api/test_route", data: data, timeout: 5)
    }

    // MARK: - Legacy Subscription Import (raw URL / content paste)
    func importSubscription(url urlStr: String) async throws {
        try await post("/api/sub", body: ["url": urlStr])
        try await fetchNodes()
        try await fetchNodeNames()
    }

    func importSubscriptionContent(_ content: String) async throws {
        try await post("/api/sub", body: ["content": content])
        try await fetchNodes()
        try await fetchNodeNames()
    }

    // MARK: - Subscription Management (new multi-sub system)

    /// Load all saved subscriptions from backend.
    func fetchSubscriptions() async throws {
        let v: [Subscription] = try await get("/api/subscriptions")
        await MainActor.run { subscriptions = v }
    }

    /// Add a new named subscription and refresh node list.
    func addSubscription(name: String, url: String) async throws -> Subscription {
        struct Req: Encodable { let action, name, url: String }
        let data = try JSONEncoder().encode(Req(action: "add", name: name, url: url))
        let sub: Subscription = try await postDataDecoding("/api/subscriptions", data: data, timeout: 30)
        try? await fetchNodes()
        try? await fetchNodeNames()
        try await fetchSubscriptions()
        return sub
    }

    /// Re-fetch a single subscription's nodes.
    func refreshSubscription(id: String) async throws {
        struct Req: Encodable { let action, id: String }
        let data = try JSONEncoder().encode(Req(action: "refresh", id: id))
        try await postData("/api/subscriptions", data: data)
        try await fetchSubscriptions()
        try? await fetchNodes()
        try? await fetchNodeNames()
    }

    /// Re-fetch ALL subscriptions' nodes.
    func refreshAllSubscriptions() async throws {
        struct Req: Encodable { let action: String }
        let data = try JSONEncoder().encode(Req(action: "refresh_all"))
        try await postData("/api/subscriptions", data: data)
        try await fetchSubscriptions()
        try? await fetchNodes()
        try? await fetchNodeNames()
    }

    /// Delete a subscription and its cached nodes.
    func deleteSubscription(id: String) async throws {
        struct Req: Encodable { let action, id: String }
        let data = try JSONEncoder().encode(Req(action: "delete", id: id))
        try await postData("/api/subscriptions", data: data)
        try await fetchSubscriptions()
        try? await fetchNodes()
        try? await fetchNodeNames()
    }

    // MARK: - HTTP Helpers
    private func get<T: Decodable>(_ path: String) async throws -> T {
        let req = URLRequest(url: URL(string: Self.baseURL + path)!, timeoutInterval: 5)
        let (data, _) = try await URLSession.shared.data(for: req)
        return try JSONDecoder().decode(T.self, from: data)
    }

    private func post<B: Encodable>(_ path: String, body: B) async throws {
        var req = URLRequest(url: URL(string: Self.baseURL + path)!)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONEncoder().encode(body)
        _ = try await URLSession.shared.data(for: req)
    }

    private func postData(_ path: String, data: Data) async throws {
        var req = URLRequest(url: URL(string: Self.baseURL + path)!)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = data
        _ = try await URLSession.shared.data(for: req)
    }

    private func postDataDecoding<T: Decodable>(_ path: String, data: Data, timeout: TimeInterval = 10) async throws -> T {
        var req = URLRequest(url: URL(string: Self.baseURL + path)!, timeoutInterval: timeout)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = data
        let (respData, _) = try await URLSession.shared.data(for: req)
        return try JSONDecoder().decode(T.self, from: respData)
    }

}
