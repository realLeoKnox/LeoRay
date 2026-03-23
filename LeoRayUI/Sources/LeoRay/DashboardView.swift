import SwiftUI

struct DashboardView: View {
    @EnvironmentObject var api: APIClient
    @EnvironmentObject var backend: BackendManager
    @State private var isBusy = false

    var xrayRunning: Bool { api.status?.running == true }

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {

                // ── Status card ────────────────────────────────────────────
                GroupBox(label: Label("状态", systemImage: "gauge.high")) {
                    HStack(spacing: 20) {
                        ZStack {
                            Circle()
                                .fill((xrayRunning ? Color.green : Color.secondary).opacity(0.15))
                                .frame(width: 72, height: 72)
                            Image(systemName: xrayRunning ? "shield.fill" : "shield.slash")
                                .font(.system(size: 32))
                                .foregroundStyle(xrayRunning ? .green : .secondary)
                        }

                        VStack(alignment: .leading, spacing: 5) {
                            if !backend.processRunning {
                                Text("控制器未运行").font(.title3.bold()).foregroundStyle(.secondary)
                                Text("请重启应用").foregroundStyle(.tertiary)
                            } else if xrayRunning {
                                Text("Xray 正在运行").font(.title3.bold())
                                if let s = api.status {
                                    Text("PID: \(s.pid)").foregroundStyle(.secondary)
                                    Text("运行时长: \(s.uptime)").foregroundStyle(.secondary)
                                }
                            } else {
                                Text("代理已停止").font(.title3.bold()).foregroundStyle(.secondary)
                                Text("点击 Start 启动代理").foregroundStyle(.tertiary)
                            }
                        }

                        Spacer()

                        // ── Start / Stop Proxy button ──────────────────
                        Button {
                            toggleProxy()
                        } label: {
                            if isBusy {
                                HStack(spacing: 6) { ProgressView().scaleEffect(0.75); Text(xrayRunning ? "停止中…" : "启动中…") }
                            } else {
                                Text(xrayRunning ? "Stop" : "Start").frame(width: 64)
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(xrayRunning ? .red : .green)
                        .disabled(isBusy || !backend.processRunning)
                    }
                    .padding(6)
                }

                // ── Quick toggles (available even when Xray stopped) ───────
                GroupBox(label: Label("快速设置", systemImage: "slider.horizontal.3")) {
                    if let p = api.policy {
                        VStack(spacing: 0) {
                            ToggleRow(title: "TUN 模式",    sub: "透明代理（首次启用需输入一次密码）", icon: "network",        val: p.enableTun)    { v in save(p) { $0.enableTun    = v } }
                            Divider()
                            ToggleRow(title: "FakeIP",     sub: "DNS 劫持，配合 TUN 使用",            icon: "wand.and.stars",   val: p.enableFakeip) { v in save(p) { $0.enableFakeip = v } }
                            Divider()
                            ToggleRow(title: "流量嗅探",   sub: "Sniffing",                           icon: "eye",              val: p.enableSniff)  { v in save(p) { $0.enableSniff  = v } }
                            Divider()
                            ToggleRow(title: "允许局域网", sub: "Allow LAN",                          icon: "wifi",             val: p.allowLan)     { v in save(p) { $0.allowLan     = v } }
                        }
                    } else {
                        HStack { ProgressView().scaleEffect(0.8); Text("加载中…").foregroundStyle(.secondary) }
                            .padding(8)
                    }
                }

                // ── Default node ───────────────────────────────────────────
                if let p = api.policy, !api.nodeNames.isEmpty {
                    GroupBox(label: Label("默认节点", systemImage: "arrow.triangle.branch")) {
                        HStack {
                            Text("Final 策略")
                            Spacer()
                            Picker("", selection: Binding(
                                get: { p.finalPolicy },
                                set: { nv in save(p) { $0.finalPolicy = nv } }
                            )) {
                                ForEach(api.nodeNames, id: \.self) { Text($0).tag($0) }
                            }
                            .pickerStyle(.menu).frame(maxWidth: 260)
                        }
                        .padding(6)
                    }
                }
            }
            .padding(20)
        }
        .navigationTitle("Dashboard")
        .task {
            // Policy and nodes are always available since xray_controller always runs
            if api.policy == nil { try? await api.fetchPolicy() }
            if api.nodeNames.isEmpty { try? await api.fetchNodeNames() }
        }
    }

    private func toggleProxy() {
        isBusy = true
        Task {
            if xrayRunning {
                try? await api.stopProxy()
            } else {
                try? await api.startProxy()
            }
            // Give Xray a moment to change state, then refresh status
            try? await Task.sleep(nanoseconds: 800_000_000)
            _ = try? await api.fetchStatus()
            await MainActor.run { isBusy = false }
        }
    }

    private func save(_ base: PolicyConfig, mutation: (inout PolicyConfig) -> Void) {
        var p = base; mutation(&p)
        Task { try? await api.savePolicy(p) }
    }
}

// MARK: - Toggle Row
struct ToggleRow: View {
    let title, sub, icon: String
    let val: Bool
    let onChange: (Bool) -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: icon).frame(width: 24).foregroundStyle(.blue)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                Text(sub).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            Toggle("", isOn: Binding(get: { val }, set: onChange)).labelsHidden()
        }
        .padding(.horizontal, 8).padding(.vertical, 10)
    }
}
