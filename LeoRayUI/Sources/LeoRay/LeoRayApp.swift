import SwiftUI

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
            Image(systemName: appDelegate.api.status?.running == true
                  ? "shield.fill" : "shield.slash")
        }
    }
}
