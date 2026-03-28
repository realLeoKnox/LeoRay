import SwiftUI

struct NodesView: View {
    @EnvironmentObject var api: APIClient

    // Subscription management state
    @State private var showAddSub       = false
    @State private var newSubName       = ""
    @State private var newSubURL        = ""
    @State private var isBusySub: String? = nil   // subscription ID being operated on
    @State private var isRefreshingAll  = false
    @State private var subError: String? = nil

    // Legacy paste import
    @State private var showPaste        = false
    @State private var pasteContent     = ""
    @State private var isImporting      = false
    @State private var importError: String? = nil

    // Node testing
    @State private var testingIdx: Int?

    var body: some View {
        VStack(spacing: 0) {

            // ── Subscription list ─────────────────────────────────────────
            subscriptionSection

            Divider()

            // ── Node list ─────────────────────────────────────────────────
            nodeListSection

            Divider()

            // ── Bottom action bar ─────────────────────────────────────────
            bottomBar
        }
        .navigationTitle("节点管理")
        .task {
            try? await api.fetchSubscriptions()
            try? await api.fetchNodes()
        }
        .sheet(isPresented: $showAddSub)  { addSubSheet }
        .sheet(isPresented: $showPaste)   { PasteSheet(content: $pasteContent) { importPaste() } }
    }

    // MARK: - Subscription Section
    private var subscriptionSection: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header
            HStack {
                Label("订阅组", systemImage: "antenna.radiowaves.left.and.right")
                    .font(.headline)
                Spacer()
                if isRefreshingAll {
                    ProgressView().scaleEffect(0.75)
                } else {
                    Button {
                        refreshAll()
                    } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .buttonStyle(.plain)
                    .help("刷新所有订阅")
                }
                Button {
                    showAddSub = true
                } label: {
                    Image(systemName: "plus")
                }
                .buttonStyle(.plain)
                .help("添加订阅")
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)

            if let err = subError {
                Text(err).font(.caption).foregroundStyle(.red)
                    .padding(.horizontal, 14).padding(.bottom, 6)
            }

            if api.subscriptions.isEmpty {
                HStack {
                    Image(systemName: "tray").foregroundStyle(.tertiary)
                    Text("暂无订阅，点击 + 添加").foregroundStyle(.secondary).font(.callout)
                }
                .padding(.horizontal, 14).padding(.bottom, 10)
            } else {
                VStack(spacing: 0) {
                    ForEach(api.subscriptions) { sub in
                        SubscriptionRow(
                            sub: sub,
                            isBusy: isBusySub == sub.id,
                            onRefresh: { refreshSub(sub) },
                            onDelete:  { deleteSub(sub) }
                        )
                        Divider()
                    }
                }
            }
        }
        .background(Color(NSColor.controlBackgroundColor))
    }

    // MARK: - Node List Section
    private var nodeListSection: some View {
        Group {
            if api.nodes.isEmpty {
                VStack(spacing: 14) {
                    Image(systemName: "server.rack").font(.system(size: 44)).foregroundStyle(.tertiary)
                    Text("暂无节点").font(.title3).foregroundStyle(.secondary)
                    Text("请在上方添加订阅").font(.callout).foregroundStyle(.tertiary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List {
                    ForEach(Array(api.nodes.enumerated()), id: \.element.id) { idx, node in
                        NodeRow(
                            node:      node,
                            latency:   api.latencies[idx],
                            isTesting: testingIdx == idx
                        ) {
                            testSingle(idx)
                        }
                    }
                }
                .listStyle(.inset(alternatesRowBackgrounds: true))
            }
        }
    }

    // MARK: - Bottom Bar
    private var bottomBar: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                Button("粘贴内容导入") { showPaste = true }
                if isImporting {
                    HStack(spacing: 4) {
                        ProgressView().scaleEffect(0.7)
                        Text("导入中…").font(.caption).foregroundStyle(.secondary)
                    }
                }
                if let err = importError {
                    Text(err).font(.caption).foregroundStyle(.red)
                }
                Spacer()
                Button("全部测速") { testAll() }
                    .disabled(api.nodes.isEmpty)
            }
        }
        .padding(14)
    }

    // MARK: - Add Subscription Sheet
    private var addSubSheet: some View {
        VStack(spacing: 16) {
            Text("添加订阅").font(.headline)

            VStack(alignment: .leading, spacing: 4) {
                Text("名称").font(.caption).foregroundStyle(.secondary)
                TextField("如 机场A", text: $newSubName)
                    .textFieldStyle(.roundedBorder)
            }

            VStack(alignment: .leading, spacing: 4) {
                Text("订阅链接").font(.caption).foregroundStyle(.secondary)
                TextField("https://…", text: $newSubURL)
                    .textFieldStyle(.roundedBorder)
            }

            if let err = subError {
                Text(err).font(.caption).foregroundStyle(.red)
            }

            HStack {
                Button("取消") {
                    showAddSub = false
                    newSubName = ""
                    newSubURL  = ""
                    subError   = nil
                }
                Spacer()
                Button("添加并更新") {
                    addSub()
                }
                .buttonStyle(.borderedProminent)
                .disabled(newSubURL.isEmpty)
            }
        }
        .padding(24)
        .frame(width: 380)
    }

    // MARK: - Actions (Subscription)
    private func addSub() {
        let url  = newSubURL.trimmingCharacters(in: .whitespacesAndNewlines)
        let name = newSubName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !url.isEmpty else { return }

        isBusySub = "new"
        subError  = nil

        Task {
            do {
                _ = try await api.addSubscription(name: name.isEmpty ? "订阅" : name, url: url)
                await MainActor.run {
                    isBusySub  = nil
                    newSubName = ""
                    newSubURL  = ""
                    showAddSub = false
                }
            } catch {
                await MainActor.run {
                    isBusySub = nil
                    subError  = "添加失败: \(error.localizedDescription)"
                }
            }
        }
    }

    private func refreshSub(_ sub: Subscription) {
        isBusySub = sub.id
        subError  = nil
        Task {
            do {
                try await api.refreshSubscription(id: sub.id)
                await MainActor.run { isBusySub = nil }
            } catch {
                await MainActor.run {
                    isBusySub = nil
                    subError  = "刷新失败: \(error.localizedDescription)"
                }
            }
        }
    }

    private func deleteSub(_ sub: Subscription) {
        isBusySub = sub.id
        Task {
            try? await api.deleteSubscription(id: sub.id)
            await MainActor.run { isBusySub = nil }
        }
    }

    private func refreshAll() {
        isRefreshingAll = true
        subError = nil
        Task {
            do {
                try await api.refreshAllSubscriptions()
                await MainActor.run { isRefreshingAll = false }
            } catch {
                await MainActor.run {
                    isRefreshingAll = false
                    subError = "刷新失败: \(error.localizedDescription)"
                }
            }
        }
    }

    // MARK: - Actions (Legacy paste import)
    private func importPaste() {
        isImporting = true
        importError = nil
        showPaste   = false
        Task {
            do {
                try await api.importSubscriptionContent(pasteContent)
                await MainActor.run { pasteContent = ""; isImporting = false }
            } catch {
                await MainActor.run { importError = "失败: \(error.localizedDescription)"; isImporting = false }
            }
        }
    }

    // MARK: - Actions (Node testing)
    private func testSingle(_ idx: Int) {
        testingIdx = idx
        Task {
            _ = try? await api.testNode(index: idx)
            await MainActor.run { testingIdx = nil }
        }
    }

    private func testAll() {
        Task {
            for idx in api.nodes.indices {
                testSingle(idx)
                try? await Task.sleep(nanoseconds: 200_000_000)
            }
        }
    }
}

// MARK: - Subscription Row
struct SubscriptionRow: View {
    let sub:       Subscription
    let isBusy:    Bool
    let onRefresh: () -> Void
    let onDelete:  () -> Void

    var body: some View {
        HStack(spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Text(sub.name).lineLimit(1).font(.body)
                HStack(spacing: 8) {
                    if sub.nodeCount > 0 {
                        Text("\(sub.nodeCount) 个节点")
                            .font(.caption2.bold())
                            .padding(.horizontal, 5).padding(.vertical, 2)
                            .background(Color.accentColor.opacity(0.12))
                            .clipShape(RoundedRectangle(cornerRadius: 4))
                    }
                    if !sub.lastUpdated.isEmpty {
                        Text("更新: \(sub.lastUpdated)").font(.caption2).foregroundStyle(.tertiary)
                    }
                }
            }
            Spacer()
            if isBusy {
                ProgressView().scaleEffect(0.7).frame(width: 28)
            } else {
                Button {
                    onRefresh()
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                .help("更新此订阅")

                Button(role: .destructive) {
                    onDelete()
                } label: {
                    Image(systemName: "trash")
                }
                .buttonStyle(.borderless)
                .help("删除此订阅")
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
    }
}

// MARK: - Node Row
struct NodeRow: View {
    let node:      OutboundNode
    let latency:   NodeLatency?
    let isTesting: Bool
    let onTest:    () -> Void

    /// Primary label: shows connect (HTTPS) latency when available, else tcp_ping
    var primaryLabel: String {
        guard !isTesting else { return "…" }
        guard let l = latency else { return "—" }
        if l.error != nil { return "超时" }
        // Prefer connect (HTTPS latency) when it's a real measurement
        if let c = l.connect, c != "未启动", !c.isEmpty {
            return c
        }
        return l.tcpPing ?? "—"
    }

    /// Secondary label: shows tcp_ping if connect is also available
    var secondaryLabel: String? {
        guard !isTesting, let l = latency, l.error == nil else { return nil }
        if let c = l.connect, c != "未启动", !c.isEmpty {
            // connect is the primary; show tcp_ping as secondary
            if let tcp = l.tcpPing, tcp != "超时" {
                return "TCP: \(tcp)"
            }
        } else if let c = l.connect, c == "未启动" {
            return "Xray 未运行"
        }
        return nil
    }

    var latencyColor: Color {
        guard let l = latency, l.error == nil else { return .secondary }
        // Try connect first, then tcp_ping
        let str = (l.connect != nil && l.connect != "未启动") ? l.connect : l.tcpPing
        guard let s = str,
              let ms = Int(s.replacingOccurrences(of: "ms", with: "").trimmingCharacters(in: .whitespaces))
        else { return .secondary }
        return ms < 150 ? .green : ms < 400 ? .yellow : .red
    }

    var body: some View {
        HStack(spacing: 10) {
            VStack(alignment: .leading, spacing: 3) {
                Text(node.tag).lineLimit(1)
                Text(node.protocolBadge)
                    .font(.caption2.bold())
                    .padding(.horizontal, 5).padding(.vertical, 2)
                    .background(Color.accentColor.opacity(0.12))
                    .clipShape(RoundedRectangle(cornerRadius: 4))
            }
            Spacer()
            if isTesting { ProgressView().scaleEffect(0.65) }
            else {
                VStack(alignment: .trailing, spacing: 2) {
                    Text(primaryLabel)
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(latencyColor)
                    if let sec = secondaryLabel {
                        Text(sec)
                            .font(.caption2)
                            .foregroundStyle(.tertiary)
                    }
                }
                .frame(minWidth: 70, alignment: .trailing)
            }
            Button("测速", action: onTest)
                .buttonStyle(.borderless)
                .disabled(isTesting)
        }
        .padding(.vertical, 3)
    }
}

// MARK: - Paste Sheet
struct PasteSheet: View {
    @Binding var content: String
    let onImport: () -> Void
    @Environment(\.dismiss) var dismiss

    var body: some View {
        VStack(spacing: 16) {
            Text("粘贴订阅内容").font(.headline)
            TextEditor(text: $content)
                .font(.system(.body, design: .monospaced))
                .frame(width: 460, height: 220)
                .border(Color.secondary.opacity(0.3), width: 1)
            HStack {
                Button("取消") { dismiss() }
                Spacer()
                Button("导入") { onImport() }
                    .buttonStyle(.borderedProminent)
                    .disabled(content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }
        .padding(24)
    }
}
