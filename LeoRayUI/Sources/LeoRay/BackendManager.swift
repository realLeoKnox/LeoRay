import Foundation
import AppKit

class BackendManager: ObservableObject {
    @Published var processRunning = false
    private var process: Process?

    func start() {
        guard let res = Bundle.main.resourceURL else { return }

        let xc = res.appendingPathComponent("xray_controller")
        let xray = res.appendingPathComponent("core/xray")

        // Ensure executables are runnable
        [xc, xray].forEach { url in
            try? FileManager.default.setAttributes(
                [.posixPermissions: NSNumber(value: 0o755)],
                ofItemAtPath: url.path
            )
        }

        let p = Process()
        p.executableURL = xc
        p.currentDirectoryURL = res
        p.terminationHandler = { [weak self] _ in
            DispatchQueue.main.async { self?.processRunning = false }
        }

        do {
            try p.run()
            process = p
            DispatchQueue.main.async { self.processRunning = true }
            print("[LeoRay] Backend started PID=\(p.processIdentifier)")
        } catch {
            print("[LeoRay] Backend launch failed: \(error)")
        }
    }

    func stop() {
        process?.terminate()
        process?.waitUntilExit()
        process = nil
        processRunning = false
    }

    deinit { stop() }
}
