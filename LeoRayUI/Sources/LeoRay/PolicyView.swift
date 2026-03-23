import SwiftUI

struct PolicyView: View {
    @EnvironmentObject var api: APIClient
    @EnvironmentObject var backend: BackendManager
    @State private var isRefreshing = false

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
                    if !p.groups.isEmpty {
                        GroupBox(label: Label("代理组", systemImage: "list.bullet.rectangle")) {
                            VStack(spacing: 0) {
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
                                    }
                                    .padding(.horizontal, 8).padding(.vertical, 8)
                                    if idx < p.groups.count - 1 { Divider() }
                                }
                            }
                        }
                    }

                    // ── Rule Sets ──────────────────────────────────────
                    if !p.ruleSets.isEmpty {
                        GroupBox(label: Label("规则集", systemImage: "square.stack.3d.up")) {
                            VStack(spacing: 0) {
                                ForEach(Array(p.ruleSets.enumerated()), id: \.element.id) { idx, rs in
                                    HStack(spacing: 10) {
                                        VStack(alignment: .leading, spacing: 2) {
                                            Text(rs.tag).lineLimit(1)          // ← "tag" field
                                            Text(rs.policy).font(.caption).foregroundStyle(.secondary) // ← "policy" field
                                        }
                                        Spacer()
                                        Toggle("", isOn: Binding(
                                            get: { rs.enabled },
                                            set: { nv in save(p) { $0.ruleSets[idx].enabled = nv } }
                                        )).labelsHidden()
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
    }

    private func save(_ base: PolicyConfig, mutation: (inout PolicyConfig) -> Void) {
        var p = base; mutation(&p)
        Task { try? await api.savePolicy(p) }
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
