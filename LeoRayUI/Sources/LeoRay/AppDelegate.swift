import Foundation
import AppKit

class AppDelegate: NSObject, NSApplicationDelegate {
    let backend = BackendManager()
    let api     = APIClient()

    func applicationDidFinishLaunching(_ notification: Notification) {
        // xray_controller starts immediately — provides management API even without Xray running.
        // Xray proxy is started only when the user clicks "Start Proxy" in the UI.
        backend.start()
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
            self.api.startPolling()
            Task {
                try? await self.api.fetchNodes()
                try? await self.api.fetchNodeNames()
                try? await self.api.fetchPolicy()
            }
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        api.stopPolling()
        // Stop Xray proxy gracefully before killing the controller
        Task { try? await self.api.stopProxy() }
        Thread.sleep(forTimeInterval: 0.3)
        backend.stop()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }
}
