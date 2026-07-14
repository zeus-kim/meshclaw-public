import EventKit
import Foundation

struct CalendarHelperError: Error, CustomStringConvertible {
    let description: String
}

struct Args {
    var requestAccessOnly = false
    var listOnly = false
    var deleteOnly = false
    var id = ""
    var title = ""
    var notes = ""
    var start = ""
    var end = ""
    var query = ""
}

func parseArgs() throws -> Args {
    var parsed = Args()
    var index = 1
    let values = CommandLine.arguments
    while index < values.count {
        let arg = values[index]
        switch arg {
        case "--request-access":
            parsed.requestAccessOnly = true
        case "--list":
            parsed.listOnly = true
        case "--delete":
            parsed.deleteOnly = true
        case "--id":
            index += 1
            guard index < values.count else { throw CalendarHelperError(description: "--id requires a value") }
            parsed.id = values[index]
        case "--title":
            index += 1
            guard index < values.count else { throw CalendarHelperError(description: "--title requires a value") }
            parsed.title = values[index]
        case "--notes":
            index += 1
            guard index < values.count else { throw CalendarHelperError(description: "--notes requires a value") }
            parsed.notes = values[index]
        case "--start":
            index += 1
            guard index < values.count else { throw CalendarHelperError(description: "--start requires a value") }
            parsed.start = values[index]
        case "--end":
            index += 1
            guard index < values.count else { throw CalendarHelperError(description: "--end requires a value") }
            parsed.end = values[index]
        case "--query":
            index += 1
            guard index < values.count else { throw CalendarHelperError(description: "--query requires a value") }
            parsed.query = values[index]
        default:
            throw CalendarHelperError(description: "unknown argument: \(arg)")
        }
        index += 1
    }
    parsed.title = parsed.title.trimmingCharacters(in: .whitespacesAndNewlines)
    if parsed.title.isEmpty && !parsed.requestAccessOnly && !parsed.listOnly && !parsed.deleteOnly {
        throw CalendarHelperError(description: "title is required")
    }
    return parsed
}

func jsonEscape(_ value: String) -> String {
    let data = try? JSONSerialization.data(withJSONObject: [value], options: [])
    let wrapped = String(data: data ?? Data("[\"\"]".utf8), encoding: .utf8) ?? "[\"\"]"
    return String(wrapped.dropFirst().dropLast())
}

func printJSON(ok: Bool, id: String = "", title: String = "", error: String = "") {
    var parts = ["\"kind\":\"argos_calendar_helper\"", "\"ok\":\(ok ? "true" : "false")"]
    if !id.isEmpty {
        parts.append("\"id\":\(jsonEscape(id))")
    }
    if !title.isEmpty {
        parts.append("\"title\":\(jsonEscape(title))")
    }
    if !error.isEmpty {
        parts.append("\"error\":\(jsonEscape(error))")
    }
    print("{\(parts.joined(separator: ","))}")
}

func printEventListJSON(events: [[String: String]], start: String, end: String, query: String) throws {
    let payload: [String: Any] = [
        "kind": "argos_calendar_helper",
        "ok": true,
        "start": start,
        "end": end,
        "query": query,
        "count": events.count,
        "events": events,
    ]
    let data = try JSONSerialization.data(withJSONObject: payload, options: [])
    print(String(data: data, encoding: .utf8) ?? "{\"kind\":\"argos_calendar_helper\",\"ok\":false,\"error\":\"json encoding failed\"}")
}

func requestCalendarAccess(_ store: EKEventStore) async throws -> Bool {
    if #available(macOS 14.0, *) {
        return try await store.requestFullAccessToEvents()
    }
    return try await withCheckedThrowingContinuation { continuation in
        store.requestAccess(to: .event) { granted, error in
            if let error {
                continuation.resume(throwing: error)
                return
            }
            continuation.resume(returning: granted)
        }
    }
}

func parseDate(_ value: String) -> Date? {
    let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
    if trimmed.isEmpty {
        return nil
    }
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    if let date = formatter.date(from: trimmed) {
        return date
    }
    formatter.formatOptions = [.withInternetDateTime]
    return formatter.date(from: trimmed)
}

func findEvent(_ store: EKEventStore, id: String, query: String, start: Date, end: Date) throws -> EKEvent {
    let cleanID = id.trimmingCharacters(in: .whitespacesAndNewlines)
    if !cleanID.isEmpty, let event = store.event(withIdentifier: cleanID) {
        return event
    }
    let calendars = store.calendars(for: .event)
    let predicate = store.predicateForEvents(withStart: start, end: end, calendars: calendars)
    let needle = query.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    let matches = store.events(matching: predicate)
        .filter { event in
            if needle.isEmpty {
                return true
            }
            return event.title.lowercased().contains(needle) || (event.notes ?? "").lowercased().contains(needle)
        }
        .sorted { $0.startDate < $1.startDate }
    if matches.isEmpty {
        throw CalendarHelperError(description: "no matching calendar event was found")
    }
    if matches.count > 1 {
        let titles = matches.prefix(5).map { $0.title ?? "" }.joined(separator: ", ")
        throw CalendarHelperError(description: "multiple calendar events matched; specify a more exact title or id: \(titles)")
    }
    return matches[0]
}

@main
struct ArgosCalendarHelper {
    static func main() async {
        do {
            let args = try parseArgs()
            let store = EKEventStore()
            let granted = try await requestCalendarAccess(store)
            if !granted {
                throw CalendarHelperError(description: "Calendar permission was not granted")
            }
            if args.requestAccessOnly {
                printJSON(ok: true, title: "Calendar access granted")
                return
            }
            guard let start = parseDate(args.start) else {
                throw CalendarHelperError(description: "start must be RFC3339")
            }
            let end = parseDate(args.end) ?? start.addingTimeInterval(3600)
            guard end > start else {
                throw CalendarHelperError(description: "end must be after start")
            }
            if args.deleteOnly {
                let event = try findEvent(store, id: args.id, query: args.query.isEmpty ? args.title : args.query, start: start, end: end)
                let title = event.title ?? ""
                let id = event.eventIdentifier ?? ""
                try store.remove(event, span: .thisEvent, commit: true)
                printJSON(ok: true, id: id, title: title)
                return
            }
            if args.listOnly {
                let calendars = store.calendars(for: .event)
                let predicate = store.predicateForEvents(withStart: start, end: end, calendars: calendars)
                let query = args.query.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
                let formatter = ISO8601DateFormatter()
                formatter.formatOptions = [.withInternetDateTime]
                let events = store.events(matching: predicate)
                    .filter { event in
                        if query.isEmpty {
                            return true
                        }
                        return event.title.lowercased().contains(query) || (event.notes ?? "").lowercased().contains(query)
                    }
                    .sorted { $0.startDate < $1.startDate }
                    .prefix(20)
                    .map { event in
                        [
                            "id": event.eventIdentifier ?? "",
                            "title": event.title ?? "",
                            "start": formatter.string(from: event.startDate),
                            "end": formatter.string(from: event.endDate),
                            "calendar": event.calendar.title,
                            "location": event.location ?? "",
                            "notes": event.notes ?? "",
                        ]
                    }
                try printEventListJSON(events: Array(events), start: formatter.string(from: start), end: formatter.string(from: end), query: args.query.trimmingCharacters(in: .whitespacesAndNewlines))
                return
            }
            guard let calendar = store.defaultCalendarForNewEvents ?? store.calendars(for: .event).first else {
                throw CalendarHelperError(description: "no calendar is available")
            }
            let event = EKEvent(eventStore: store)
            event.calendar = calendar
            event.title = args.title
            event.notes = args.notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? nil : args.notes
            event.startDate = start
            event.endDate = end
            event.addAlarm(EKAlarm(relativeOffset: -600))
            try store.save(event, span: .thisEvent, commit: true)
            printJSON(ok: true, id: event.eventIdentifier, title: args.title)
        } catch {
            printJSON(ok: false, error: String(describing: error))
            exit(1)
        }
    }
}
