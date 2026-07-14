import EventKit
import Foundation

struct ReminderHelperError: Error, CustomStringConvertible {
    let description: String
}

struct Args {
    var requestAccessOnly = false
    var listOnly = false
    var completeOnly = false
    var deleteOnly = false
    var id = ""
    var title = ""
    var notes = ""
    var due = ""
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
        case "--complete":
            parsed.completeOnly = true
        case "--delete":
            parsed.deleteOnly = true
        case "--id":
            index += 1
            guard index < values.count else { throw ReminderHelperError(description: "--id requires a value") }
            parsed.id = values[index]
        case "--title":
            index += 1
            guard index < values.count else { throw ReminderHelperError(description: "--title requires a value") }
            parsed.title = values[index]
        case "--notes":
            index += 1
            guard index < values.count else { throw ReminderHelperError(description: "--notes requires a value") }
            parsed.notes = values[index]
        case "--due":
            index += 1
            guard index < values.count else { throw ReminderHelperError(description: "--due requires a value") }
            parsed.due = values[index]
        case "--start":
            index += 1
            guard index < values.count else { throw ReminderHelperError(description: "--start requires a value") }
            parsed.start = values[index]
        case "--end":
            index += 1
            guard index < values.count else { throw ReminderHelperError(description: "--end requires a value") }
            parsed.end = values[index]
        case "--query":
            index += 1
            guard index < values.count else { throw ReminderHelperError(description: "--query requires a value") }
            parsed.query = values[index]
        default:
            throw ReminderHelperError(description: "unknown argument: \(arg)")
        }
        index += 1
    }
    parsed.title = parsed.title.trimmingCharacters(in: .whitespacesAndNewlines)
    if parsed.title.isEmpty && !parsed.requestAccessOnly && !parsed.listOnly && !parsed.completeOnly && !parsed.deleteOnly {
        throw ReminderHelperError(description: "title is required")
    }
    if parsed.completeOnly && parsed.deleteOnly {
        throw ReminderHelperError(description: "--complete and --delete cannot be used together")
    }
    return parsed
}

func jsonEscape(_ value: String) -> String {
    let data = try? JSONSerialization.data(withJSONObject: [value], options: [])
    let wrapped = String(data: data ?? Data("[\"\"]".utf8), encoding: .utf8) ?? "[\"\"]"
    return String(wrapped.dropFirst().dropLast())
}

func printJSON(ok: Bool, id: String = "", title: String = "", error: String = "") {
    var parts = ["\"kind\":\"argos_reminder_helper\"", "\"ok\":\(ok ? "true" : "false")"]
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

func printReminderListJSON(reminders: [[String: String]], start: String, end: String, query: String) throws {
    let payload: [String: Any] = [
        "kind": "argos_reminder_helper",
        "ok": true,
        "start": start,
        "end": end,
        "query": query,
        "count": reminders.count,
        "reminders": reminders,
    ]
    let data = try JSONSerialization.data(withJSONObject: payload, options: [])
    print(String(data: data, encoding: .utf8) ?? "{\"kind\":\"argos_reminder_helper\",\"ok\":false,\"error\":\"json encoding failed\"}")
}

func requestReminderAccess(_ store: EKEventStore) async throws -> Bool {
    if #available(macOS 14.0, *) {
        return try await store.requestFullAccessToReminders()
    }
    return try await withCheckedThrowingContinuation { continuation in
        store.requestAccess(to: .reminder) { granted, error in
            if let error {
                continuation.resume(throwing: error)
                return
            }
            continuation.resume(returning: granted)
        }
    }
}

func parseDue(_ value: String) -> Date? {
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

func incompleteReminders(_ store: EKEventStore, start: Date, end: Date) async -> [EKReminder] {
    await withCheckedContinuation { continuation in
        let predicate = store.predicateForIncompleteReminders(withDueDateStarting: start, ending: end, calendars: store.calendars(for: .reminder))
        store.fetchReminders(matching: predicate) { reminders in
            continuation.resume(returning: reminders ?? [])
        }
    }
}

func incompleteRemindersMatching(_ store: EKEventStore, query: String) async -> [EKReminder] {
    await withCheckedContinuation { continuation in
        let predicate = store.predicateForIncompleteReminders(withDueDateStarting: nil, ending: nil, calendars: store.calendars(for: .reminder))
        store.fetchReminders(matching: predicate) { reminders in
            let needle = query.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            let filtered = (reminders ?? []).filter { reminder in
                if needle.isEmpty {
                    return true
                }
                return reminder.title.lowercased().contains(needle) || (reminder.notes ?? "").lowercased().contains(needle)
            }
            continuation.resume(returning: filtered)
        }
    }
}

func findReminder(_ store: EKEventStore, id: String, query: String) async throws -> EKReminder {
    let cleanID = id.trimmingCharacters(in: .whitespacesAndNewlines)
    if !cleanID.isEmpty, let reminder = store.calendarItem(withIdentifier: cleanID) as? EKReminder {
        return reminder
    }
    let matches = await incompleteRemindersMatching(store, query: query)
    if matches.isEmpty {
        throw ReminderHelperError(description: "no matching incomplete reminder was found")
    }
    if matches.count > 1 {
        let titles = matches.prefix(5).map { $0.title ?? "" }.joined(separator: ", ")
        throw ReminderHelperError(description: "multiple reminders matched; specify a more exact title or id: \(titles)")
    }
    return matches[0]
}

@main
struct ArgosReminderHelper {
    static func main() async {
        do {
            let args = try parseArgs()
            let store = EKEventStore()
            let granted = try await requestReminderAccess(store)
            if !granted {
                throw ReminderHelperError(description: "Reminders permission was not granted")
            }
            if args.requestAccessOnly {
                printJSON(ok: true, title: "Reminders access granted")
                return
            }
            if args.listOnly {
                guard let start = parseDue(args.start), let end = parseDue(args.end), end > start else {
                    throw ReminderHelperError(description: "start and end must be RFC3339")
                }
                let query = args.query.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
                let formatter = ISO8601DateFormatter()
                formatter.formatOptions = [.withInternetDateTime]
                let reminders = await incompleteReminders(store, start: start, end: end)
                    .filter { reminder in
                        if query.isEmpty {
                            return true
                        }
                        return reminder.title.lowercased().contains(query) || (reminder.notes ?? "").lowercased().contains(query)
                    }
                    .sorted { left, right in
                        let leftDate = left.dueDateComponents?.date ?? Date.distantFuture
                        let rightDate = right.dueDateComponents?.date ?? Date.distantFuture
                        return leftDate < rightDate
                    }
                    .prefix(20)
                    .map { reminder in
                        [
                            "id": reminder.calendarItemIdentifier,
                            "title": reminder.title ?? "",
                            "due": reminder.dueDateComponents?.date.map { formatter.string(from: $0) } ?? "",
                            "calendar": reminder.calendar.title,
                            "notes": reminder.notes ?? "",
                        ]
                    }
                try printReminderListJSON(reminders: Array(reminders), start: formatter.string(from: start), end: formatter.string(from: end), query: args.query.trimmingCharacters(in: .whitespacesAndNewlines))
                return
            }
            if args.completeOnly || args.deleteOnly {
                let reminder = try await findReminder(store, id: args.id, query: args.query.isEmpty ? args.title : args.query)
                let title = reminder.title ?? ""
                if args.completeOnly {
                    reminder.isCompleted = true
                    reminder.completionDate = Date()
                    try store.save(reminder, commit: true)
                    printJSON(ok: true, id: reminder.calendarItemIdentifier, title: title)
                    return
                }
                try store.remove(reminder, commit: true)
                printJSON(ok: true, id: reminder.calendarItemIdentifier, title: title)
                return
            }
            guard let calendar = store.defaultCalendarForNewReminders() ?? store.calendars(for: .reminder).first else {
                throw ReminderHelperError(description: "no reminder calendar is available")
            }
            let reminder = EKReminder(eventStore: store)
            reminder.calendar = calendar
            reminder.title = args.title
            if !args.notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                reminder.notes = args.notes
            }
            if let due = parseDue(args.due) {
                var calendar = Calendar.current
                calendar.timeZone = TimeZone.current
                reminder.dueDateComponents = calendar.dateComponents([.year, .month, .day, .hour, .minute], from: due)
                reminder.addAlarm(EKAlarm(absoluteDate: due))
            }
            try store.save(reminder, commit: true)
            printJSON(ok: true, id: reminder.calendarItemIdentifier, title: args.title)
        } catch {
            printJSON(ok: false, error: String(describing: error))
            exit(1)
        }
    }
}
