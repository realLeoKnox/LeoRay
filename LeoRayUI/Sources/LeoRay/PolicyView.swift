import SwiftUI

// ─── Common GEO rule presets for the picker ──────────────────────────────────
private let geoSitePresets: [(label: String, value: String)] = [
    ("geosite:cn", "geosite:cn"),
    ("geosite:google", "geosite:google"),
    ("geosite:microsoft", "geosite:microsoft"),
    ("geosite:apple", "geosite:apple"),
    ("geosite:category-ai-!cn", "geosite:category-ai-!cn"),
    ("geosite:category-cryptocurrency", "geosite:category-cryptocurrency"),
    ("geosite:telegram", "geosite:telegram"),
    ("geosite:youtube", "geosite:youtube"),
    ("geosite:netflix", "geosite:netflix"),
    ("geosite:spotify", "geosite:spotify"),
    ("geosite:twitter", "geosite:twitter"),
    ("geosite:tiktok", "geosite:tiktok"),
    ("geosite:bilibili", "geosite:bilibili"),
    ("geosite:category-games", "geosite:category-games"),
    ("geoip:cn", "geoip:cn"),
    ("geoip:private", "geoip:private"),
    ("geoip:us", "geoip:us"),
]

struct PolicyView: View {
    @EnvironmentObject var api: APIClient
    @EnvironmentObject var backend: BackendManager

    @State private var isRefreshing     = false
    @State private var showRuleSets     = false   // rule set compat mode hidden by default

    // Add Group
    @State private var showingAddGroup  = false
    @State private var newGroupName     = ""

    // Add GEO Rule
    @State private var showingAddGeoRule  = false
    @State private var newGeoRuleValue    = "geosite:google"
    @State private var newGeoRulePolicy   = "Proxy"
    @State private var newGeoRuleCustom   = ""

    // Add Inline Rule
    @State private var showingAddInlineRule = false
    @State private var newInlineType        = "GEOSITE"
    @State private var newInlinePayload     = ""
    @State private var newInlinePolicy      = "direct"

    // Add Rule Set (compat mode)
    @State private var showingAddRuleSet = false
    @State private var newRuleSetTag     = ""
    @State private var newRuleSetPolicy  = "direct"
    @State private var newRuleSetURL     = ""

    // Route Testing
    @State private var testTarget     = ""
    @State private var testMethod     = "xray"
    @State private var testResult: String? = nil
    @State private var isTestingRoute = false

    var body: some View {
        ScrollView {
            VStack(spacing: 16) {

                // ── Guard: backend must be running ─────────────────────────
                if !backend.processRunning {
                    emptyState
                } else if let p = api.policy {
                    policyBody(p)
                } else {
                    ProgressView("正在加载策略…")
                        .frame(maxWidth: .infinity).padding(.top, 60)
                }
            }
            .padding(20)
        }
        .navigationTitle("路由策略")
        .task {
            if backend.processRunning {
                try? await api.fetchPolicy()
                try? await api.fetchNodeNames()
            }
        }
        // Sheets
        .sheet(isPresented: $showingAddGroup)       { addGroupSheet }
        .sheet(isPresented: $showingAddGeoRule)      { addGeoRuleSheet }
        .sheet(isPresented: $showingAddInlineRule)   { addInlineRuleSheet }
        .sheet(isPresented: $showingAddRuleSet)      { addRuleSetSheet }
    }

    // MARK: - Empty state
    private var emptyState: some View {
        VStack(spacing: 10) {
            Image(systemName: "shield.slash").font(.system(size: 40)).foregroundStyle(.tertiary)
            Text("请先在 Dashboard 启动代理服务").foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(.top, 60)
    }

    // MARK: - Main policy body
    @ViewBuilder
    private func policyBody(_ p: PolicyConfig) -> some View {

        // ── Basic toggles ──────────────────────────────────────────────────
        GroupBox(label: Label("基本设置", systemImage: "gearshape")) {
            VStack(spacing: 0) {
                PolicyToggle(title: "TUN 模式",   sub: "透明代理，拦截所有流量（需管理员）",  icon: "network",        val: p.enableTun)    { v in save(p) { $0.enableTun    = v } }
                Divider()
                PolicyToggle(title: "FakeIP",     sub: "DNS 劫持，配合 TUN 使用",             icon: "wand.and.stars", val: p.enableFakeip) { v in save(p) { $0.enableFakeip = v } }
                Divider()
                PolicyToggle(title: "流量嗅探",   sub: "嗅探 HTTP/TLS/QUIC 域名",             icon: "eye",            val: p.enableSniff)  { v in save(p) { $0.enableSniff  = v } }
                Divider()
                PolicyToggle(title: "允许局域网", sub: "其他设备可使用本机代理",               icon: "wifi",           val: p.allowLan)     { v in save(p) { $0.allowLan     = v } }
            }
        }

        // ── DNS Configuration ────────────────────────────────────────────
        dnsConfigSection(p)

        // ── Final (catch-all) ─────────────────────────────────────────────
        GroupBox(label: Label("Final 兜底策略", systemImage: "arrow.triangle.branch")) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text("未匹配流量指向")
                    Text("所有未命中规则的流量").font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Picker("", selection: Binding(
                    get: { p.finalPolicy },
                    set: { nv in save(p) { $0.finalPolicy = nv } }
                )) {
                    ForEach(allPolicies(p), id: \.self) { Text($0).tag($0) }
                }
                .pickerStyle(.menu).frame(maxWidth: 240)
            }
            .padding(6)
        }

        // ── Proxy Groups (name → node only) ──────────────────────────────
        GroupBox(label: HStack {
            Label("策略组", systemImage: "list.bullet.rectangle")
            Spacer()
            Button { showingAddGroup = true } label: { Image(systemName: "plus") }.buttonStyle(.plain)
        }) {
            VStack(spacing: 0) {
                if p.groups.isEmpty {
                    Text("暂无策略组").font(.caption).foregroundStyle(.secondary).padding(.vertical, 8)
                }

                ForEach(Array(sortedGroups(p).enumerated()), id: \.element.id) { idx, group in
                    groupRow(p: p, group: group, idx: idx)
                    if idx < p.groups.count - 1 { Divider() }
                }
            }
        }

        // ── GEO Rules (geo_rule → policy group) ─────────────────────────
        GroupBox(label: HStack {
            Label("GEO 路由规则", systemImage: "globe")
            Spacer()
            Button { showingAddGeoRule = true } label: { Image(systemName: "plus") }.buttonStyle(.plain)
        }) {
            VStack(spacing: 0) {
                if p.geoRules.isEmpty {
                    Text("暂无 GEO 规则").font(.caption).foregroundStyle(.secondary).padding(.vertical, 8)
                }

                ForEach(Array(sortedGeoRules(p).enumerated()), id: \.element.id) { idx, rule in
                    geoRuleRow(p: p, rule: rule, idx: idx)
                    if idx < p.geoRules.count - 1 { Divider() }
                }
            }
        }

        // ── Inline Rules (user-added, drag-to-sort) ───────────────────────
        GroupBox(label: HStack {
            Label("自定义规则", systemImage: "text.line.first.and.arrowtriangle.forward")
            Spacer()
            Button { showingAddInlineRule = true } label: { Image(systemName: "plus") }.buttonStyle(.plain)
        }) {
            VStack(spacing: 0) {
                if p.inlineRules.isEmpty {
                    Text("暂无自定义规则 — 推荐使用 GEOSITE / GEOIP 类型")
                        .font(.caption).foregroundStyle(.secondary).padding(.vertical, 8)
                }
                ForEach(Array(sortedInlineRules(p).enumerated()), id: \.element.id) { idx, rule in
                    inlineRuleRow(p: p, rule: rule, idx: idx)
                    if idx < p.inlineRules.count - 1 { Divider() }
                }
            }
        }

        // ── Rule Set compat mode (collapsed by default) ───────────────────
        DisclosureGroup(
            isExpanded: $showRuleSets,
            content: { ruleSetSection(p) },
            label: {
                HStack(spacing: 6) {
                    Image(systemName: "square.stack.3d.up").foregroundStyle(.secondary)
                    Text("高级：规则集兼容模式").foregroundStyle(.secondary)
                    Text("(远程 .list)").font(.caption2).foregroundStyle(.tertiary)
                }
            }
        )
        .padding(.vertical, 4)

        // ── Route Testing ──────────────────────────────────────────────────
        GroupBox(label: Label("路由诊断", systemImage: "stethoscope")) {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    TextField("输入域名或 IP (如 google.com)", text: $testTarget)
                        .textFieldStyle(.roundedBorder)
                    Picker("", selection: $testMethod) {
                        Text("Xray 实测").tag("xray")
                        Text("Go 模拟").tag("go")
                    }
                    .pickerStyle(.menu).frame(width: 120)
                    Button { runRouteTest() } label: {
                        if isTestingRoute { ProgressView().scaleEffect(0.7).frame(width: 60) }
                        else              { Text("测试").frame(width: 60) }
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(testTarget.isEmpty || isTestingRoute)
                }
                if let res = testResult {
                    Text(res)
                        .font(.system(.subheadline, design: .monospaced))
                        .foregroundStyle(res.contains("error") || res.contains("timeout") ? .red : .green)
                }
            }
            .padding(.vertical, 4)
        }
    }

    // MARK: - Group Row (name → node picker only)
    @ViewBuilder
    private func groupRow(p: PolicyConfig, group: PolicyGroup, idx: Int) -> some View {
        HStack(spacing: 10) {
            // Drag handle hint
            Image(systemName: "line.3.horizontal").foregroundStyle(.tertiary).frame(width: 16)

            Text(group.name).lineLimit(1)
            Spacer()

            // Node binding
            Picker("节点", selection: Binding(
                get: { group.node },
                set: { nv in save(p) { p in
                    if let i = p.groups.firstIndex(where: { $0.name == group.name }) {
                        p.groups[i].node = nv
                    }
                }}
            )) {
                Text("默认").tag("")
                ForEach(api.nodeNames, id: \.self) { Text($0).tag($0) }
            }
            .pickerStyle(.menu).frame(maxWidth: 200)

            Button {
                save(p) { p in p.groups.removeAll { $0.name == group.name } }
            } label: {
                Image(systemName: "minus.circle").foregroundStyle(.red)
            }.buttonStyle(.plain)
        }
        .padding(.horizontal, 8).padding(.vertical, 8)
    }

    // MARK: - GEO Rule Row
    @ViewBuilder
    private func geoRuleRow(p: PolicyConfig, rule: GeoRule, idx: Int) -> some View {
        HStack(spacing: 10) {
            Image(systemName: "line.3.horizontal").foregroundStyle(.tertiary).frame(width: 16)

            VStack(alignment: .leading, spacing: 2) {
                Text(rule.geoRule).font(.caption.monospaced()).foregroundStyle(.blue)
            }
            Spacer()

            // Policy group picker
            Picker("策略", selection: Binding(
                get: { rule.policy },
                set: { nv in save(p) { p in
                    if let i = p.geoRules.firstIndex(where: { $0.geoRule == rule.geoRule }) {
                        p.geoRules[i].policy = nv
                    }
                }}
            )) {
                ForEach(allPolicies(p), id: \.self) { Text($0).tag($0) }
            }
            .pickerStyle(.menu).frame(maxWidth: 160)

            Button {
                save(p) { p in p.geoRules.removeAll { $0.geoRule == rule.geoRule } }
            } label: {
                Image(systemName: "minus.circle").foregroundStyle(.red)
            }.buttonStyle(.plain)
        }
        .padding(.horizontal, 8).padding(.vertical, 8)
    }

    // MARK: - Inline Rule Row
    @ViewBuilder
    private func inlineRuleRow(p: PolicyConfig, rule: InlineRule, idx: Int) -> some View {
        HStack(spacing: 10) {
            Image(systemName: "line.3.horizontal").foregroundStyle(.tertiary).frame(width: 16)

            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Text(rule.type)
                        .font(.caption2.bold())
                        .padding(.horizontal, 5).padding(.vertical, 2)
                        .background(Color.accentColor.opacity(0.12))
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                    Text(rule.payload).font(.caption.monospaced()).lineLimit(1)
                }
                Text(rule.policy).font(.caption2).foregroundStyle(.secondary)
            }
            Spacer()

            Picker("策略", selection: Binding(
                get: { rule.policy },
                set: { nv in save(p) { p in
                    if let i = p.inlineRules.firstIndex(where: { $0.id == rule.id }) {
                        p.inlineRules[i].policy = nv
                    }
                }}
            )) {
                ForEach(allPolicies(p), id: \.self) { Text($0).tag($0) }
            }
            .pickerStyle(.menu).frame(maxWidth: 150)

            Button {
                save(p) { p in p.inlineRules.removeAll { $0.id == rule.id } }
            } label: {
                Image(systemName: "minus.circle").foregroundStyle(.red)
            }.buttonStyle(.plain)
        }
        .padding(.horizontal, 8).padding(.vertical, 8)
    }

    // MARK: - DNS Config Section
    @ViewBuilder
    private func dnsConfigSection(_ p: PolicyConfig) -> some View {
        GroupBox(label: Label("DNS 防污染", systemImage: "network.badge.shield.half.filled")) {
            VStack(spacing: 10) {
                dnsRow(title: "直连 DNS", sub: "用于命中 DIRECT 的流量",
                       text: Binding(
                        get: { p.dnsConfig.directDns.joined(separator: ", ") },
                        set: { nv in save(p) { $0.dnsConfig.directDns = nv.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty } } }
                       ))
                Divider()
                dnsRow(title: "代理 DNS", sub: "用于命中代理策略的流量",
                       text: Binding(
                        get: { p.dnsConfig.proxyDns.joined(separator: ", ") },
                        set: { nv in save(p) { $0.dnsConfig.proxyDns = nv.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty } } }
                       ))
                Divider()
                dnsRow(title: "直连 DOH", sub: "可选，如 https://dns.alidns.com/dns-query",
                       text: Binding(
                        get: { p.dnsConfig.directDoh },
                        set: { nv in save(p) { $0.dnsConfig.directDoh = nv.trimmingCharacters(in: .whitespaces) } }
                       ))
                Divider()
                dnsRow(title: "代理 DOH", sub: "可选，如 https://cloudflare-dns.com/dns-query",
                       text: Binding(
                        get: { p.dnsConfig.proxyDoh },
                        set: { nv in save(p) { $0.dnsConfig.proxyDoh = nv.trimmingCharacters(in: .whitespaces) } }
                       ))
            }
            .padding(.vertical, 4)
        }
    }

    @ViewBuilder
    private func dnsRow(title: String, sub: String, text: Binding<String>) -> some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                Text(sub).font(.caption).foregroundStyle(.secondary)
            }
            .frame(width: 140, alignment: .leading)
            TextField(title, text: text)
                .textFieldStyle(.roundedBorder)
        }
        .padding(.horizontal, 8)
    }

    // MARK: - Rule Set Section (compat mode)
    @ViewBuilder
    private func ruleSetSection(_ p: PolicyConfig) -> some View {
        VStack(spacing: 0) {
            HStack {
                Spacer()
                Button { showingAddRuleSet = true } label: {
                    Label("添加规则集", systemImage: "plus")
                }.buttonStyle(.plain)
            }
            .padding(.bottom, 6)

            if p.ruleSets.isEmpty {
                Text("暂无远程规则集").font(.caption).foregroundStyle(.secondary).padding(.vertical, 8)
            }
            ForEach(Array(p.ruleSets.enumerated()), id: \.element.id) { idx, rs in
                HStack(spacing: 10) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(rs.tag).lineLimit(1)
                        if !rs.url.isEmpty {
                            Text(rs.url).font(.caption2).foregroundStyle(.tertiary).lineLimit(1)
                        }
                    }
                    Spacer()
                    Toggle("", isOn: Binding(
                        get: { rs.enabled },
                        set: { nv in save(p) { $0.ruleSets[idx].enabled = nv } }
                    )).labelsHidden()

                    Picker("", selection: Binding(
                        get: { rs.policy },
                        set: { nv in save(p) { $0.ruleSets[idx].policy = nv } }
                    )) {
                        ForEach(allPolicies(p), id: \.self) { Text($0).tag($0) }
                    }
                    .pickerStyle(.menu).frame(maxWidth: 160)

                    Button {
                        save(p) { $0.ruleSets.remove(at: idx) }
                    } label: {
                        Image(systemName: "minus.circle").foregroundStyle(.red)
                    }.buttonStyle(.plain)
                }
                .padding(.horizontal, 8).padding(.vertical, 8)
                if idx < p.ruleSets.count - 1 { Divider() }
            }

            Divider()
            HStack {
                Spacer()
                Button {
                    isRefreshing = true
                    Task {
                        try? await api.refreshRules()
                        await MainActor.run { isRefreshing = false }
                    }
                } label: {
                    if isRefreshing {
                        HStack(spacing: 6) { ProgressView().scaleEffect(0.7); Text("刷新中…") }
                    } else {
                        Label("刷新远程规则集", systemImage: "arrow.clockwise")
                    }
                }
                .disabled(isRefreshing)
            }
            .padding(.horizontal, 8).padding(.vertical, 10)
        }
        .padding(.leading, 8)
    }

    // MARK: - Add Sheets
    private var addGroupSheet: some View {
        VStack(spacing: 16) {
            Text("添加策略组").font(.headline)
            TextField("名称 (如 Streaming、Download)", text: $newGroupName)
                .textFieldStyle(.roundedBorder)
            Text("策略组用于分组管理节点，GEO 规则可在独立区块中绑定到策略组")
                .font(.caption).foregroundStyle(.secondary)
            HStack {
                Button("取消") { showingAddGroup = false }
                Spacer()
                Button("添加") {
                    if !newGroupName.isEmpty, let p = api.policy {
                        let order = p.groups.count
                        save(p) {
                            $0.groups.append(PolicyGroup(name: newGroupName, node: "", order: order))
                        }
                        newGroupName = ""
                    }
                    showingAddGroup = false
                }.buttonStyle(.borderedProminent)
            }
        }.padding().frame(width: 360)
    }

    private var addGeoRuleSheet: some View {
        VStack(spacing: 16) {
            Text("添加 GEO 路由规则").font(.headline)

            Picker("GEO 规则", selection: $newGeoRuleValue) {
                ForEach(geoSitePresets, id: \.value) { preset in
                    Text(preset.label).tag(preset.value)
                }
                Divider()
                Text("自定义…").tag("__custom__")
            }

            if newGeoRuleValue == "__custom__" {
                TextField("如 geosite:steam 或 geoip:jp", text: $newGeoRuleCustom)
                    .textFieldStyle(.roundedBorder)
            }

            if let p = api.policy {
                Picker("目标策略组", selection: $newGeoRulePolicy) {
                    ForEach(allPolicies(p), id: \.self) { Text($0).tag($0) }
                }
            }

            // Hint: where to find rules
            HStack(spacing: 4) {
                Image(systemName: "info.circle").foregroundStyle(.blue).font(.caption)
                Text("在 ")
                    .font(.caption).foregroundStyle(.secondary)
                Link("v2fly/domain-list-community",
                     destination: URL(string: "https://github.com/v2fly/domain-list-community/tree/master/data")!)
                    .font(.caption)
                Text(" 查找可用的 geosite 规则")
                    .font(.caption).foregroundStyle(.secondary)
            }

            HStack {
                Button("取消") { showingAddGeoRule = false }
                Spacer()
                Button("添加") {
                    if let p = api.policy {
                        let value = newGeoRuleValue == "__custom__" ? newGeoRuleCustom : newGeoRuleValue
                        if !value.isEmpty {
                            let order = p.geoRules.count
                            save(p) {
                                $0.geoRules.append(GeoRule(geoRule: value, policy: newGeoRulePolicy, order: order))
                            }
                        }
                        newGeoRuleValue = "geosite:google"
                        newGeoRuleCustom = ""
                    }
                    showingAddGeoRule = false
                }.buttonStyle(.borderedProminent)
            }
        }.padding().frame(width: 420)
    }

    private var addInlineRuleSheet: some View {
        VStack(spacing: 16) {
            Text("添加自定义规则").font(.headline)
            Picker("类型", selection: $newInlineType) {
                // GEO types first (recommended)
                Text("GEOSITE ⭐").tag("GEOSITE")
                Text("GEOIP ⭐").tag("GEOIP")
                Divider()
                ForEach(["DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "IP-CIDR6", "DST-PORT"], id: \.self) {
                    Text($0).tag($0)
                }
            }
            TextField(newInlineType.hasPrefix("GEO") ? "如 cn 或 google" : "如 google.com 或 192.168.0.0/16",
                      text: $newInlinePayload)
                .textFieldStyle(.roundedBorder)

            if let p = api.policy {
                Picker("策略", selection: $newInlinePolicy) {
                    ForEach(allPolicies(p), id: \.self) { Text($0).tag($0) }
                }
            }

            HStack {
                Button("取消") { showingAddInlineRule = false }
                Spacer()
                Button("添加") {
                    if !newInlinePayload.isEmpty, let p = api.policy {
                        let order = p.inlineRules.count
                        save(p) {
                            $0.inlineRules.append(InlineRule(type: newInlineType, payload: newInlinePayload, policy: newInlinePolicy, order: order))
                        }
                        newInlinePayload = ""
                    }
                    showingAddInlineRule = false
                }.buttonStyle(.borderedProminent)
            }
        }.padding().frame(width: 340)
    }

    private var addRuleSetSheet: some View {
        VStack(spacing: 16) {
            Text("添加远程规则集").font(.headline)
            TextField("标签 (如 去广告)", text: $newRuleSetTag)
                .textFieldStyle(.roundedBorder)
            TextField("规则链接 (URL, .list 格式)", text: $newRuleSetURL)
                .textFieldStyle(.roundedBorder)
            if let p = api.policy {
                Picker("策略", selection: $newRuleSetPolicy) {
                    ForEach(allPolicies(p), id: \.self) { Text($0).tag($0) }
                }
            }
            HStack {
                Button("取消") { showingAddRuleSet = false }
                Spacer()
                Button("添加") {
                    if !newRuleSetTag.isEmpty, let p = api.policy {
                        let tag   = newRuleSetTag
                        let url   = newRuleSetURL
                        let local = "data/rule_cache/\(tag).list"
                        save(p) {
                            $0.ruleSets.append(RuleSet(tag: tag, policy: newRuleSetPolicy, enabled: true, url: url, local: local))
                        }
                        newRuleSetTag = ""
                        newRuleSetURL = ""
                    }
                    showingAddRuleSet = false
                }.buttonStyle(.borderedProminent)
            }
        }.padding().frame(width: 380)
    }

    // MARK: - Helpers
    private func allPolicies(_ p: PolicyConfig) -> [String] {
        p.groups.map(\.name) + ["direct", "block"]
    }

    private func sortedGroups(_ p: PolicyConfig) -> [PolicyGroup] {
        p.groups.sorted { $0.order < $1.order }
    }

    private func sortedGeoRules(_ p: PolicyConfig) -> [GeoRule] {
        p.geoRules.sorted { $0.order < $1.order }
    }

    private func sortedInlineRules(_ p: PolicyConfig) -> [InlineRule] {
        p.inlineRules.sorted { $0.order < $1.order }
    }

    private func save(_ base: PolicyConfig, mutation: (inout PolicyConfig) -> Void) {
        var p = base; mutation(&p)
        // Re-assign order based on current array positions
        for i in p.groups.indices       { p.groups[i].order      = i }
        for i in p.geoRules.indices     { p.geoRules[i].order   = i }
        for i in p.inlineRules.indices  { p.inlineRules[i].order = i }
        Task { try? await api.savePolicy(p) }
    }

    private func runRouteTest() {
        guard !testTarget.isEmpty else { return }
        isTestingRoute = true
        testResult = nil
        Task {
            do {
                let resp = try await api.testRoute(
                    target: testTarget.trimmingCharacters(in: .whitespaces),
                    method: testMethod
                )
                await MainActor.run {
                    if let err = resp.error {
                        testResult = "错误: \(err)"
                    } else {
                        let rText = resp.rule.map { " (\($0))" } ?? ""
                        testResult = "命中节点: [ \(resp.outbound) ]\(rText)"
                    }
                    isTestingRoute = false
                }
            } catch {
                await MainActor.run {
                    testResult = "请求失败: \(error.localizedDescription)"
                    isTestingRoute = false
                }
            }
        }
    }
}

// MARK: - Toggle Row Component
struct PolicyToggle: View {
    let title, sub, icon: String
    let val: Bool
    let onChange: (Bool) -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: icon).frame(width: 26).foregroundStyle(.blue)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                Text(sub).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            Toggle("", isOn: Binding(get: { val }, set: onChange)).labelsHidden()
        }
        .padding(.horizontal, 8).padding(.vertical, 12)
    }
}
