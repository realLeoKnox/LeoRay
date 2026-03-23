import SwiftUI
import AppKit

struct MenuBarView: View {
    @EnvironmentObject var api: APIClient
    @EnvironmentObject var backend: BackendManager
    @State private var isBusy = false

    var xrayRunning: Bool { api.status?.running == true }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {

            // ── Status ─────────────────────────────────────────────────────
            HStack(spacing: 8) {
                Circle()
                    .fill(xrayRunning ? Color.green : Color.secondary)
                    .frame(width: 8, height: 8)
                Text(xrayRunning ? "Xray 运行中" : "代理已停止")
                    .font(.headline)
            }
            .padding(.horizontal, 12).padding(.top, 10).padding(.bottom, 2)

            if let s = api.status, s.running {
                Text("PID \(s.pid)  ·  \(s.uptime)")
                    .font(.caption).foregroundStyle(.secondary)
                    .padding(.horizontal, 12)
            }

            Divider()

            // ── Start / Stop Proxy ─────────────────────────────────────────
            Button {
                isBusy = true
                Task {
                    if xrayRunning { try? await api.stopProxy() }
                    else           { try? await api.startProxy() }
                    try? await Task.sleep(nanoseconds: 800_000_000)
                    _ = try? await api.fetchStatus()
                    await MainActor.run { isBusy = false }
                }
            } label: {
                if isBusy {
                    Label(xrayRunning ? "停止中…" : "启动中…", systemImage: "circle.dotted")
                } else if xrayRunning {
                    Label("停止代理", systemImage: "stop.circle")
                } else {
                    Label("启动代理", systemImage: "play.circle")
                }
            }
            .buttonStyle(.plain)
            .disabled(isBusy || !backend.processRunning)
            .padding(.horizontal, 12).padding(.vertical, 5)

            Divider()

            // ── Mode indicators ────────────────────────────────────────────
            if let p = api.policy {
                MenuBarBadge(label: "TUN",    icon: "network",        on: p.enableTun)
                MenuBarBadge(label: "FakeIP", icon: "wand.and.stars", on: p.enableFakeip)
                MenuBarBadge(label: "Sniff",  icon: "eye",            on: p.enableSniff)
                Divider()
            }

            // ── Open main window ───────────────────────────────────────────
            Button {
                NSApp.activate(ignoringOtherApps: true)
                NSApp.windows.first?.makeKeyAndOrderFront(nil)
            } label: {
                Label("打开主界面", systemImage: "macwindow")
            }
            .buttonStyle(.plain)
            .padding(.horizontal, 12).padding(.vertical, 4)

            Divider()

            Button(role: .destructive) { NSApp.terminate(nil) } label: {
                Label("退出 LeoRay", systemImage: "power")
            }
            .buttonStyle(.plain)
            .padding(.horizontal, 12).padding(.vertical, 4).padding(.bottom, 6)
        }
        .frame(minWidth: 230)
    }
}

private struct MenuBarBadge: View {
    let label: String; let icon: String; let on: Bool
    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: icon).frame(width: 18).foregroundStyle(.secondary)
            Text(label)
            Spacer()
            Image(systemName: on ? "checkmark.circle.fill" : "circle")
                .foregroundStyle(on ? .green : .secondary)
        }
        .font(.callout).padding(.horizontal, 12).padding(.vertical, 3)
    }
}
