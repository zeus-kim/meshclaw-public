import Contacts
import Foundation

struct ContactsHelperError: Error, CustomStringConvertible {
    let description: String
}

struct Args {
    var requestAccessOnly = false
    var searchOnly = false
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
        case "--search":
            parsed.searchOnly = true
        case "--query":
            index += 1
            guard index < values.count else { throw ContactsHelperError(description: "--query requires a value") }
            parsed.query = values[index]
        default:
            throw ContactsHelperError(description: "unknown argument: \(arg)")
        }
        index += 1
    }
    parsed.query = parsed.query.trimmingCharacters(in: .whitespacesAndNewlines)
    if parsed.searchOnly && parsed.query.isEmpty {
        throw ContactsHelperError(description: "query is required")
    }
    return parsed
}

func printJSON(_ payload: [String: Any]) throws {
    let data = try JSONSerialization.data(withJSONObject: payload, options: [])
    print(String(data: data, encoding: .utf8) ?? "{\"kind\":\"argos_contacts_helper\",\"ok\":false,\"error\":\"json encoding failed\"}")
}

func requestContactsAccess(_ store: CNContactStore) async throws -> Bool {
    try await withCheckedThrowingContinuation { continuation in
        store.requestAccess(for: .contacts) { granted, error in
            if let error {
                continuation.resume(throwing: error)
                return
            }
            continuation.resume(returning: granted)
        }
    }
}

func searchContacts(_ store: CNContactStore, query: String) throws -> [[String: Any]] {
    let keys: [CNKeyDescriptor] = [
        CNContactIdentifierKey as CNKeyDescriptor,
        CNContactGivenNameKey as CNKeyDescriptor,
        CNContactFamilyNameKey as CNKeyDescriptor,
        CNContactOrganizationNameKey as CNKeyDescriptor,
        CNContactPhoneNumbersKey as CNKeyDescriptor,
        CNContactEmailAddressesKey as CNKeyDescriptor,
    ]
    let request = CNContactFetchRequest(keysToFetch: keys)
    let needle = query.lowercased()
    let isWildcard = ["*", "all"].contains(needle)
    var results: [[String: Any]] = []
    let maxResults = isWildcard ? 100 : 20
    try store.enumerateContacts(with: request) { contact, stop in
        let nameParts = [contact.familyName, contact.givenName]
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        let name = nameParts.isEmpty ? contact.organizationName : nameParts.joined(separator: " ")
        let haystack = ([name, contact.organizationName] +
            contact.phoneNumbers.map { $0.value.stringValue } +
            contact.emailAddresses.map { String($0.value) }).joined(separator: " ").lowercased()
        if isWildcard || haystack.contains(needle) {
            results.append([
                "id": contact.identifier,
                "name": name,
                "organization": contact.organizationName,
                "phones": contact.phoneNumbers.map { $0.value.stringValue },
                "emails": contact.emailAddresses.map { String($0.value) },
            ])
        }
        if results.count >= maxResults {
            stop.pointee = true
        }
    }
    return results
}

@main
struct ArgosContactsHelper {
    static func main() async {
        do {
            let args = try parseArgs()
            let store = CNContactStore()
            let granted = try await requestContactsAccess(store)
            if !granted {
                throw ContactsHelperError(description: "Contacts permission was not granted")
            }
            if args.requestAccessOnly {
                try printJSON(["kind": "argos_contacts_helper", "ok": true, "title": "Contacts access granted"])
                return
            }
            if args.searchOnly {
                let contacts = try searchContacts(store, query: args.query)
                try printJSON([
                    "kind": "argos_contacts_helper",
                    "ok": true,
                    "query": args.query,
                    "count": contacts.count,
                    "contacts": contacts,
                ])
                return
            }
            throw ContactsHelperError(description: "no action specified")
        } catch {
            try? printJSON(["kind": "argos_contacts_helper", "ok": false, "error": String(describing: error)])
            exit(1)
        }
    }
}
