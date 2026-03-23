import SwiftUI

// MARK: - Sidebar items
enum NavItem: String, CaseIterable, Identifiable {
    case dashboard = "Dashboard"
    case nodes     = "Nodes"
    case policy    = "Policy"
    case logs      = "Logs"
    var id: String { rawValue }
    var icon: String {
        switch self {
        case .dashboard: return "gauge.high"
        case .nodes:     return "server.rack"
        case .policy:    return "list.bullet.rectangle"
        case .logs:      return "doc.text.magnifyingglass"
        }
    }
}

struct ContentView: View {
    @EnvironmentObject var api: APIClient
    @EnvironmentObject var backend: BackendManager
    @State private var selection: NavItem? = .dashboard

    var body: some View {
        NavigationSplitView {
            List(NavItem.allCases, selection: $selection) { item in
                Label(item.rawValue, systemImage: item.icon)
                    .tag(item)
            }
            .navigationSplitViewColumnWidth(min: 155, ideal: 170, max: 190)

            Spacer()

            // Status dot at bottom of sidebar
            HStack(spacing: 6) {
                Circle()
                    .fill(api.status?.running == true ? Color.green : Color.red)
                    .frame(width: 8, height: 8)
                Text(api.status?.running == true ? "Running" : "Stopped")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .padding(.horizontal, 14)
            .padding(.bottom, 12)

        } detail: {
            Group {
                switch selection ?? .dashboard {
                case .dashboard: DashboardView()
                case .nodes:     NodesView()
                case .policy:    PolicyView()
                case .logs:      LogsView()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .navigationTitle("LeoRay")
    }
}
