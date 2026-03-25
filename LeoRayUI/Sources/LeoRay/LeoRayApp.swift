import SwiftUI

// Reactive label for MenuBarExtra
struct MenuBarLabel: View {
    @ObservedObject var api: APIClient
    var body: some View {
        Image(systemName: api.status?.running == true ? "shield.fill" : "shield.slash")
    }
}

@main
struct LeoRayApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    var body: some Scene {
        // Main window
        WindowGroup {
            ContentView()
                .environmentObject(appDelegate.api)
                .environmentObject(appDelegate.backend)
        }
        .defaultSize(width: 820, height: 560)
        .commands {
            CommandGroup(replacing: .newItem) {}
        }

        // Menu bar extra
        MenuBarExtra {
            MenuBarView()
                .environmentObject(appDelegate.api)
                .environmentObject(appDelegate.backend)
        } label: {
            MenuBarLabel(api: appDelegate.api)
        }
        .menuBarExtraStyle(.window)
    }
}
