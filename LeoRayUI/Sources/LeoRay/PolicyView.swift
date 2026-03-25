import SwiftUI

struct PolicyView: View {
    @EnvironmentObject var api: APIClient
    @EnvironmentObject var backend: BackendManager
    @State private var isRefreshing = false

    // State for Add Group
    @State private var showingAddGroup = false
    @State private var newGroupName = ""

    // State for Add Rule Set
    @State private var showingAddRuleSet = false
    @State private var newRuleSetTag = ""
    @State private var newRuleSetPolicy = "direct"
    @State private var newRuleSetURL = ""

    // State for Add Inline Rule
    @State private var showingAddInlineRule = false
    @State private var newInlineType = "GEOSITE"
    @State private var newInlinePayload = ""
    @State private var newInlinePolicy = "direct"

    // State for Route Testing
    @State private var testTarget = ""
    @State private var testMethod = "xray" // "xray" or "go"
    @State private var testResult: String? = nil
    @State private var isTestingRoute = false

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                if !backend.processRunning {
                    VStack(spacing: 10) {
                        Image(systemName: "shield.slash").font(.system(size: 40)).foregroundStyle(.tertiary)
                        Text("请先在 Dashboard 启动代理服务").foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .padding(.top, 60)
                } else if let p = api.policy {

                    // ── Basic toggles ──────────────────────────────────
                    GroupBox(label: Label("基本设置", systemImage: "gearshape")) {
                        VStack(spacing: 0) {
                            PolicyToggle(title: "TUN 模式",    sub: "透明代理，拦截所有流量（需管理员）", icon: "network",        val: p.enableTun)    { v in save(p) { $0.enableTun    = v } }
                            Divider()
                            PolicyToggle(title: "FakeIP",      sub: "DNS 劫持，配合 TUN 使用",            icon: "wand.and.stars", val: p.enableFakeip) { v in save(p) { $0.enableFakeip = v } }
                            Divider()
                            PolicyToggle(title: "流量嗅探",    sub: "嗅探 HTTP/TLS/QUIC 域名",            icon: "eye",            val: p.enableSniff)  { v in save(p) { $0.enableSniff  = v } }
                            Divider()
                            PolicyToggle(title: "允许局域网",  sub: "其他设备可使用本机代理",              icon: "wifi",           val: p.allowLan)     { v in save(p) { $0.allowLan     = v } }
                        }
                    }

                    // ── Dev Tools: Route Testing ──────────────────────────────────
                    GroupBox(label: Label("开发者诊断：路由测试", systemImage: "stethoscope")) {
                        VStack(alignment: .leading, spacing: 12) {
                            HStack {
                                TextField("输入域名或IP (如 google.com)", text: $testTarget)
                                    .textFieldStyle(.roundedBorder)
                                
                                Picker("", selection: $testMethod) {
                                    Text("Xray 实测").tag("xray")
                                    Text("Go 模拟").tag("go")
                                }
                                .pickerStyle(.menu)
                                .frame(width: 120)
                                
                                Button {
                                    runRouteTest()
                                } label: {
                                    if isTestingRoute {
                                        ProgressView().scaleEffect(0.7).frame(width: 60)
                                    } else {
                                        Text("测试").frame(width: 60)
                                    }
                                }
                                .buttonStyle(.borderedProminent)
                                .disabled(testTarget.isEmpty || isTestingRoute)
                            }
                            
                            if let res = testResult {
                                Text(res)
                                    .font(.system(.subheadline, design: .monospaced))
                                    .foregroundStyle(res.contains("error") || res.contains("timeout") ? .red : .green)
                                    .padding(.top, 4)
                            }
                        }
                        .padding(.vertical, 4)
                    }

                    // ── Final policy ───────────────────────────────────
                    GroupBox(label: Label("默认策略", systemImage: "arrow.triangle.branch")) {
                        HStack {
                            VStack(alignment: .leading, spacing: 2) {
                                Text("Final 策略")
                                Text("未匹配规则的流量").font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            Picker("", selection: Binding(
                                get: { p.finalPolicy },
                                set: { nv in save(p) { $0.finalPolicy = nv } }
                            )) {
                                ForEach(api.nodeNames, id: \.self) { Text($0).tag($0) }
                            }
                            .pickerStyle(.menu).frame(maxWidth: 240)
                        }
                        .padding(6)
                    }

                    // ── Proxy Groups ───────────────────────────────────
                    GroupBox(label: HStack {
                        Label("代理组", systemImage: "list.bullet.rectangle")
                        Spacer()
                        Button { showingAddGroup = true } label: { Image(systemName: "plus") }.buttonStyle(.plain)
                    }) {
                        VStack(spacing: 0) {
                            if p.groups.isEmpty {
                                Text("暂无代理组").font(.caption).foregroundStyle(.secondary).padding(.vertical, 8)
                            }
                            ForEach(Array(p.groups.enumerated()), id: \.element.id) { idx, group in
                                HStack(spacing: 10) {
                                    Text(group.name).lineLimit(1)
                                    Spacer()
                                    Picker("", selection: Binding(
                                        get: { group.node },
                                        set: { nv in save(p) { $0.groups[idx].node = nv } }
                                    )) {
                                        Text("默认").tag("")
                                        ForEach(api.nodeNames, id: \.self) { Text($0).tag($0) }
                                    }
                                    .pickerStyle(.menu).frame(maxWidth: 220)
                                    
                                    Button { save(p) { $0.groups.remove(at: idx) } } label: {
                                        Image(systemName: "minus.circle").foregroundStyle(.red)
                                    }.buttonStyle(.plain)
                                }
                                .padding(.horizontal, 8).padding(.vertical, 8)
                                if idx < p.groups.count - 1 { Divider() }
                            }
                        }
                    }

                    // ── Rule Sets ──────────────────────────────────────
                    GroupBox(label: HStack {
                        Label("规则集", systemImage: "square.stack.3d.up")
                        Spacer()
                        Button { showingAddRuleSet = true } label: { Image(systemName: "plus") }.buttonStyle(.plain)
                    }) {
                        VStack(spacing: 0) {
                            if p.ruleSets.isEmpty {
                                Text("暂无规则集").font(.caption).foregroundStyle(.secondary).padding(.vertical, 8)
                            }
                            ForEach(Array(p.ruleSets.enumerated()), id: \.element.id) { idx, rs in
                                HStack(spacing: 10) {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(rs.tag).lineLimit(1)
                                        Text(rs.policy).font(.caption).foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                    Toggle("", isOn: Binding(
                                        get: { rs.enabled },
                                        set: { nv in save(p) { $0.ruleSets[idx].enabled = nv } }
                                    )).labelsHidden()
                                    
                                    Button { save(p) { $0.ruleSets.remove(at: idx) } } label: {
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
                                        Label("刷新规则集", systemImage: "arrow.clockwise")
                                    }
                                }
                                .disabled(isRefreshing)
                            }
                            .padding(.horizontal, 8).padding(.vertical, 10)
                        }
                    }

                    // ── Inline Rules (Custom Single Rules) ─────────────
                    GroupBox(label: HStack {
                        Label("自定义单条规则", systemImage: "text.line.first.and.arrowtriangle.forward")
                        Spacer()
                        Button { showingAddInlineRule = true } label: { Image(systemName: "plus") }.buttonStyle(.plain)
                    }) {
                        VStack(spacing: 0) {
                            if p.inlineRules.isEmpty {
                                Text("暂无自定义单条规则").font(.caption).foregroundStyle(.secondary).padding(.vertical, 8)
                            }
                            ForEach(Array(p.inlineRules.enumerated()), id: \.element.id) { idx, ir in
                                HStack(spacing: 10) {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text("\(ir.type), \(ir.payload)").lineLimit(1)
                                        Text(ir.policy).font(.caption).foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                    Button { save(p) { $0.inlineRules.remove(at: idx) } } label: {
                                        Image(systemName: "minus.circle").foregroundStyle(.red)
                                    }.buttonStyle(.plain)
                                }
                                .padding(.horizontal, 8).padding(.vertical, 8)
                                if idx < p.inlineRules.count - 1 { Divider() }
                            }
                        }
                    }

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
        // ── Sheets ────────────────────────────────────────────────────────
        .sheet(isPresented: $showingAddGroup) {
            VStack(spacing: 16) {
                Text("添加代理组").font(.headline)
                TextField("名称 (如 MyGroup)", text: $newGroupName)
                HStack {
                    Button("取消") { showingAddGroup = false }
                    Spacer()
                    Button("添加") {
                        if !newGroupName.isEmpty, let p = api.policy {
                            save(p) { $0.groups.append(PolicyGroup(name: newGroupName, node: "")) }
                            newGroupName = ""
                        }
                        showingAddGroup = false
                    }.buttonStyle(.borderedProminent)
                }
            }.padding().frame(width: 300)
        }
        .sheet(isPresented: $showingAddRuleSet) {
            VStack(spacing: 16) {
                Text("添加规则集").font(.headline)
                TextField("标签 (如 去广告)", text: $newRuleSetTag)
                TextField("规则链接 (URL，支持 .list 格式)", text: $newRuleSetURL)
                Picker("策略", selection: $newRuleSetPolicy) {
                    Text("直连").tag("direct")
                    Text("拦截").tag("block")
                    if let groups = api.policy?.groups {
                        ForEach(groups) { g in Text(g.name).tag(g.name) }
                    }
                }
                HStack {
                    Button("取消") { showingAddRuleSet = false }
                    Spacer()
                    Button("添加") {
                        if !newRuleSetTag.isEmpty, let p = api.policy {
                            let tag = newRuleSetTag
                            save(p) {
                                $0.ruleSets.append(RuleSet(tag: tag, policy: newRuleSetPolicy, enabled: true, url: newRuleSetURL, local: "rule/\(tag).list"))
                            }
                            newRuleSetTag = ""
                            newRuleSetURL = ""
                        }
                        showingAddRuleSet = false
                    }.buttonStyle(.borderedProminent)
                }
            }.padding().frame(width: 350)
        }
        .sheet(isPresented: $showingAddInlineRule) {
            VStack(spacing: 16) {
                Text("添加自定义规则").font(.headline)
                Picker("类型", selection: $newInlineType) {
                    ForEach(["DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "GEOSITE", "GEOIP", "PORT"], id: \.self) {
                        Text($0).tag($0)
                    }
                }
                TextField("载荷 (如 cn 或 google.com)", text: $newInlinePayload)
                Picker("策略", selection: $newInlinePolicy) {
                    Text("直连").tag("direct")
                    Text("拦截").tag("block")
                    if let groups = api.policy?.groups {
                        ForEach(groups) { g in Text(g.name).tag(g.name) }
                    }
                }
                HStack {
                    Button("取消") { showingAddInlineRule = false }
                    Spacer()
                    Button("添加") {
                        if !newInlinePayload.isEmpty, let p = api.policy {
                            save(p) {
                                $0.inlineRules.append(InlineRule(type: newInlineType, payload: newInlinePayload, policy: newInlinePolicy))
                            }
                            newInlinePayload = ""
                        }
                        showingAddInlineRule = false
                    }.buttonStyle(.borderedProminent)
                }
            }.padding().frame(width: 300)
        }
    }

    private func save(_ base: PolicyConfig, mutation: (inout PolicyConfig) -> Void) {
        var p = base; mutation(&p)
        Task { try? await api.savePolicy(p) }
    }

    private func runRouteTest() {
        guard !testTarget.isEmpty else { return }
        isTestingRoute = true
        testResult = nil
        Task {
            do {
                let resp = try await api.testRoute(target: testTarget.trimmingCharacters(in: .whitespaces), method: testMethod)
                await MainActor.run {
                    if let err = resp.error {
                        testResult = "错误: \(err)"
                    } else {
                        let rText = resp.rule != nil ? " (\(resp.rule!))" : ""
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
