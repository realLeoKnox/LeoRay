import SwiftUI

struct NodesView: View {
    @EnvironmentObject var api: APIClient

    @State private var subURL       = ""
    @State private var isImporting  = false
    @State private var importError: String?
    @State private var testingIdx: Int?
    @State private var showPaste    = false
    @State private var pasteContent = ""

    var body: some View {
        VStack(spacing: 0) {
            // ── Node list ──────────────────────────────────────────────────
            if api.nodes.isEmpty {
                VStack(spacing: 14) {
                    Image(systemName: "server.rack").font(.system(size: 44)).foregroundStyle(.tertiary)
                    Text("暂无节点").font(.title3).foregroundStyle(.secondary)
                    Text("请在下方导入订阅").font(.callout).foregroundStyle(.tertiary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List {
                    ForEach(Array(api.nodes.enumerated()), id: \.element.id) { idx, node in
                        NodeRow(node: node,
                                latency: api.latencies[idx],
                                isTesting: testingIdx == idx) {
                            testSingle(idx)
                        }
                    }
                }
                .listStyle(.inset(alternatesRowBackgrounds: true))
            }

            Divider()

            // ── Import bar ─────────────────────────────────────────────────
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    TextField("订阅链接 (URL)", text: $subURL)
                        .textFieldStyle(.roundedBorder)

                    Button("URL 导入") { importURL() }
                        .disabled(subURL.isEmpty || isImporting)

                    Button("粘贴内容") { showPaste = true }

                    Spacer()

                    Button("全部测速") { testAll() }
                        .disabled(api.nodes.isEmpty)
                }

                if isImporting {
                    HStack(spacing: 6) {
                        ProgressView().scaleEffect(0.7)
                        Text("导入中…").font(.caption).foregroundStyle(.secondary)
                    }
                }
                if let err = importError {
                    Text(err).font(.caption).foregroundStyle(.red)
                }
            }
            .padding(14)
        }
        .navigationTitle("节点管理")
        .task { try? await api.fetchNodes() }
        .sheet(isPresented: $showPaste) {
            PasteSheet(content: $pasteContent) { importPaste() }
        }
    }

    // MARK: Actions
    private func importURL() {
        isImporting = true; importError = nil
        Task {
            do {
                try await api.importSubscription(url: subURL)
                await MainActor.run { subURL = ""; isImporting = false }
            } catch {
                await MainActor.run { importError = "失败: \(error.localizedDescription)"; isImporting = false }
            }
        }
    }

    private func importPaste() {
        isImporting = true; importError = nil; showPaste = false
        Task {
            do {
                try await api.importSubscriptionContent(pasteContent)
                await MainActor.run { pasteContent = ""; isImporting = false }
            } catch {
                await MainActor.run { importError = "失败: \(error.localizedDescription)"; isImporting = false }
            }
        }
    }

    private func testSingle(_ idx: Int) {
        testingIdx = idx
        Task { _ = try? await api.testNode(index: idx); await MainActor.run { testingIdx = nil } }
    }

    private func testAll() {
        Task {
            for idx in api.nodes.indices { testSingle(idx); try? await Task.sleep(nanoseconds: 200_000_000) }
        }
    }
}

// MARK: - Node Row
struct NodeRow: View {
    let node: OutboundNode
    let latency: NodeLatency?
    let isTesting: Bool
    let onTest: () -> Void

    var latencyLabel: String {
        guard !isTesting else { return "…" }
        guard let l = latency else { return "—" }
        if l.error != nil { return "超时" }
        return l.tcpPing ?? "—"
    }

    var latencyColor: Color {
        guard let l = latency, l.error == nil,
              let s = l.tcpPing,
              let ms = Int(s.replacingOccurrences(of: "ms", with: "").trimmingCharacters(in: .whitespaces))
        else { return .secondary }
        return ms < 100 ? .green : ms < 300 ? .yellow : .red
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
                Text(latencyLabel)
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(latencyColor)
                    .frame(width: 55, alignment: .trailing)
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
