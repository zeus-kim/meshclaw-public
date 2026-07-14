import AVFoundation
import CoreMedia
import Foundation
import ScreenCaptureKit

struct RecorderError: Error, CustomStringConvertible {
    let description: String
}

final class RecordingDelegate: NSObject, SCRecordingOutputDelegate {
    private let started = DispatchSemaphore(value: 0)
    private let finished = DispatchSemaphore(value: 0)
    private var failure: Error?

    func recordingOutputDidStartRecording(_ recordingOutput: SCRecordingOutput) {
        started.signal()
    }

    func recordingOutput(_ recordingOutput: SCRecordingOutput, didFailWithError error: Error) {
        failure = error
        started.signal()
        finished.signal()
    }

    func recordingOutputDidFinishRecording(_ recordingOutput: SCRecordingOutput) {
        finished.signal()
    }

    func waitUntilStarted(timeout: TimeInterval) throws {
        if started.wait(timeout: .now() + timeout) == .timedOut {
            throw RecorderError(description: "screen recording did not start before timeout")
        }
        if let failure {
            throw failure
        }
    }

    func waitUntilFinished(timeout: TimeInterval) throws {
        if finished.wait(timeout: .now() + timeout) == .timedOut {
            throw RecorderError(description: "screen recording did not finish before timeout")
        }
        if let failure {
            throw failure
        }
    }
}

struct Args {
    var seconds: Int = 0
    var output: String = ""
}

func parseArgs() throws -> Args {
    var parsed = Args()
    var index = 1
    let values = CommandLine.arguments
    while index < values.count {
        let arg = values[index]
        switch arg {
        case "--seconds":
            index += 1
            guard index < values.count, let seconds = Int(values[index]) else {
                throw RecorderError(description: "--seconds requires an integer")
            }
            parsed.seconds = seconds
        case "--output":
            index += 1
            guard index < values.count else {
                throw RecorderError(description: "--output requires a path")
            }
            parsed.output = values[index]
        default:
            throw RecorderError(description: "unknown argument: \(arg)")
        }
        index += 1
    }
    if parsed.seconds <= 0 {
        throw RecorderError(description: "seconds must be positive")
    }
    if parsed.output.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
        throw RecorderError(description: "output path is required")
    }
    return parsed
}

func jsonEscape(_ value: String) -> String {
    let data = try? JSONSerialization.data(withJSONObject: [value], options: [])
    let wrapped = String(data: data ?? Data("[\"\"]".utf8), encoding: .utf8) ?? "[\"\"]"
    return String(wrapped.dropFirst().dropLast())
}

func printJSON(ok: Bool, path: String = "", error: String = "") {
    var parts = ["\"kind\":\"argos_screen_recorder\"", "\"ok\":\(ok ? "true" : "false")"]
    if !path.isEmpty {
        parts.append("\"path\":\(jsonEscape(path))")
    }
    if !error.isEmpty {
        parts.append("\"error\":\(jsonEscape(error))")
    }
    print("{\(parts.joined(separator: ","))}")
}

@main
struct ArgosScreenRecorder {
    static func main() async {
        do {
            if #available(macOS 15.0, *) {
                let args = try parseArgs()
                try await record(seconds: args.seconds, output: args.output)
                printJSON(ok: true, path: args.output)
            } else {
                throw RecorderError(description: "ScreenCaptureKit recording requires macOS 15 or later")
            }
        } catch {
            printJSON(ok: false, error: String(describing: error))
            exit(1)
        }
    }

    @available(macOS 15.0, *)
    static func record(seconds: Int, output: String) async throws {
        let outputURL = URL(fileURLWithPath: output)
        try FileManager.default.createDirectory(
            at: outputURL.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )
        if FileManager.default.fileExists(atPath: outputURL.path) {
            try FileManager.default.removeItem(at: outputURL)
        }

        let content = try await SCShareableContent.current
        guard let display = content.displays.first else {
            throw RecorderError(description: "no capturable display found")
        }

        let filter = SCContentFilter(display: display, excludingWindows: [])
        filter.includeMenuBar = true

        let configuration = SCStreamConfiguration()
        configuration.width = display.width
        configuration.height = display.height
        configuration.minimumFrameInterval = CMTime(value: 1, timescale: 30)
        configuration.queueDepth = 6
        configuration.showsCursor = true
        configuration.showMouseClicks = true
        configuration.capturesAudio = false
        configuration.captureMicrophone = false
        configuration.captureDynamicRange = .SDR

        let recordingConfiguration = SCRecordingOutputConfiguration()
        recordingConfiguration.outputURL = outputURL
        recordingConfiguration.videoCodecType = AVVideoCodecType.h264
        recordingConfiguration.outputFileType = AVFileType.mov

        let delegate = RecordingDelegate()
        let recordingOutput = SCRecordingOutput(configuration: recordingConfiguration, delegate: delegate)
        let stream = SCStream(filter: filter, configuration: configuration, delegate: nil)

        try stream.addRecordingOutput(recordingOutput)
        try await stream.startCapture()
        try delegate.waitUntilStarted(timeout: 5)
        try await Task.sleep(nanoseconds: UInt64(seconds) * 1_000_000_000)
        try stream.removeRecordingOutput(recordingOutput)
        try delegate.waitUntilFinished(timeout: 10)
        try await stream.stopCapture()

        let attrs = try FileManager.default.attributesOfItem(atPath: outputURL.path)
        let size = (attrs[.size] as? NSNumber)?.int64Value ?? 0
        if size <= 0 {
            throw RecorderError(description: "recording output is empty")
        }
    }
}
