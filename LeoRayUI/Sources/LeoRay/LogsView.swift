import SwiftUI

struct LogsView: View {
    @EnvironmentObject var api: APIClient
    @State private var filter     = ""
    @State private var autoScroll = true

    var filtered: [String] {
        filter.isEmpty ? api.logs : api.logs.filter { $0.localizedCaseInsensitiveContains(filter) }
    }

    var body: some View {
        VStack(spacing: 0) {
            // Toolbar
            HStack(spacing: 12) {
                TextField("过滤关键词…", text: $filter)
                    .textFieldStyle(.roundedBorder)
                    .frame(maxWidth: 220)
                Toggle("自动滚动", isOn: $autoScroll)
                    .toggleStyle(.checkbox)
                Spacer()
                Button(role: .destructive) { api.logs = [] } label: {
                    Label("清空", systemImage: "trash")
                }
                .buttonStyle(.borderless)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
            .background(Color(nsColor: .controlBackgroundColor))

            Divider()

            // Log output
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 1) {
                        ForEach(Array(filtered.enumerated()), id: \.offset) { idx, line in
                            Text(line)
                                .font(.system(.caption, design: .monospaced))
                                .foregroundStyle(logColor(line))
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(.horizontal, 10)
                                .id(idx)
                        }
                    }
                    .padding(.vertical, 6)
                }
                .onChange(of: filtered.count) { _ in
                    if autoScroll, let last = filtered.indices.last {
                        withAnimation { proxy.scrollTo(last, anchor: .bottom) }
                    }
                }
            }
        }
        .navigationTitle("日志")
    }

    private func logColor(_ line: String) -> Color {
        if line.contains("[ERROR]") || line.contains("error") || line.contains("failed") { return .red }
        if line.contains("[WARN]")  || line.contains("warning")                          { return .orange }
        if line.contains("[SYSTEM]")                                                      { return .blue }
        if line.contains("[TUN]")                                                         { return .purple }
        return .primary
    }
}
