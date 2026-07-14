package guardvault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const version = 1

type Entry struct {
	Handle          string     `json:"handle"`
	Scope           string     `json:"scope"`
	Name            string     `json:"name"`
	Kind            string     `json:"kind,omitempty"`
	Description     string     `json:"description,omitempty"`
	Fingerprint     string     `json:"fingerprint"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	Backend         string     `json:"backend"`
	Policy          string     `json:"policy"`
	RawAvailable    bool       `json:"raw_available"`
	KeychainService string     `json:"keychain_service,omitempty"`
	KeychainAccount string     `json:"keychain_account,omitempty"`
}

type BackendStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	ReadWrite   bool   `json:"read_write"`
	Executable  string `json:"executable,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	Recommended bool   `json:"recommended"`
}

type diskEntry struct {
	Version    int       `json:"version"`
	Entry      Entry     `json:"entry"`
	Nonce      string    `json:"nonce"`
	Ciphertext string    `json:"ciphertext"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type PutOptions struct {
	Scope       string
	Name        string
	Kind        string
	Description string
	Backend     string
	Value       []byte
}

type LinkKeychainOptions struct {
	Scope       string
	Name        string
	Kind        string
	Description string
	Service     string
	Account     string
}

type UseResult struct {
	Handle      string    `json:"handle"`
	EnvName     string    `json:"env_name"`
	Command     []string  `json:"command"`
	ExitCode    int       `json:"exit_code"`
	Stdout      string    `json:"stdout"`
	Stderr      string    `json:"stderr"`
	DurationMs  int64     `json:"duration_ms"`
	Fingerprint string    `json:"fingerprint"`
	UsedAt      time.Time `json:"used_at"`
	Policy      string    `json:"policy"`
}

func ResolveEnv(secretEnv map[string]string) (map[string]string, []Entry, error) {
	resolved := map[string]string{}
	entries := []Entry{}
	for envName, handle := range secretEnv {
		envName = strings.TrimSpace(envName)
		if !validEnvName(envName) {
			return nil, nil, errors.New("env name must contain only A-Z, a-z, 0-9, and underscore, and cannot start with a digit")
		}
		scope, name, err := ParseHandle(handle)
		if err != nil {
			return nil, nil, err
		}
		value, entry, err := openSecret(scope, name)
		if err != nil {
			return nil, nil, err
		}
		resolved[envName] = string(value)
		entries = append(entries, entry)
	}
	return resolved, entries, nil
}

func Root() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_GUARD_VAULT_DIR")); path != "" {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".meshclaw", "guard-vault")
	}
	return filepath.Join(".meshclaw", "guard-vault")
}

func Init() (string, error) {
	root := Root()
	if err := os.MkdirAll(entriesDir(root), 0700); err != nil {
		return "", err
	}
	_, err := masterKey(root)
	return root, err
}

func Put(opts PutOptions) (Entry, error) {
	scope, name, err := normalizeScopeName(opts.Scope, opts.Name)
	if err != nil {
		return Entry{}, err
	}
	backend := normalizeBackend(opts.Backend)
	value := []byte(strings.TrimRight(string(opts.Value), "\r\n"))
	if len(value) == 0 {
		return Entry{}, errors.New("secret value is empty")
	}
	root, err := Init()
	if err != nil {
		return Entry{}, err
	}
	if backend != "local-aes-gcm" {
		return putExternal(root, backend, scope, name, opts, value)
	}
	key, err := masterKey(root)
	if err != nil {
		return Entry{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return Entry{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Entry{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Entry{}, err
	}
	now := time.Now().UTC()
	entry := Entry{
		Handle:       Handle(scope, name),
		Scope:        scope,
		Name:         name,
		Kind:         strings.TrimSpace(opts.Kind),
		Description:  strings.TrimSpace(opts.Description),
		Fingerprint:  fingerprint(value),
		CreatedAt:    now,
		UpdatedAt:    now,
		Backend:      "local-aes-gcm",
		Policy:       "use-only; raw value is not returned by MeshClaw MCP",
		RawAvailable: true,
	}
	if existing, err := Metadata(scope, name); err == nil {
		entry.CreatedAt = existing.CreatedAt
	}
	payload := diskEntry{
		Version:    version,
		Entry:      entry,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, value, []byte(entry.Handle))),
		UpdatedAt:  now,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return Entry{}, err
	}
	if err := os.WriteFile(entryPath(root, scope, name), append(data, '\n'), 0600); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func LinkKeychain(opts LinkKeychainOptions) (Entry, error) {
	scope, name, err := normalizeScopeName(opts.Scope, opts.Name)
	if err != nil {
		return Entry{}, err
	}
	service := strings.TrimSpace(opts.Service)
	account := strings.TrimSpace(opts.Account)
	if service == "" || account == "" {
		return Entry{}, errors.New("keychain service and account are required")
	}
	root, err := Init()
	if err != nil {
		return Entry{}, err
	}
	now := time.Now().UTC()
	entry := Entry{
		Handle:          Handle(scope, name),
		Scope:           scope,
		Name:            name,
		Kind:            strings.TrimSpace(opts.Kind),
		Description:     strings.TrimSpace(opts.Description),
		Fingerprint:     "external:apple-keychain",
		CreatedAt:       now,
		UpdatedAt:       now,
		Backend:         "apple-keychain",
		Policy:          "use-only; raw value stays in Apple Keychain and is not returned by MeshClaw MCP",
		RawAvailable:    true,
		KeychainService: service,
		KeychainAccount: account,
	}
	if existing, err := Metadata(scope, name); err == nil {
		entry.CreatedAt = existing.CreatedAt
	}
	payload := diskEntry{Version: version, Entry: entry, UpdatedAt: now}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return Entry{}, err
	}
	if err := os.WriteFile(entryPath(root, scope, name), append(data, '\n'), 0600); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func Backends() []BackendStatus {
	return []BackendStatus{
		backendStatus("local-aes-gcm", "MeshClaw local AES-GCM fallback", "", true),
		backendStatus("apple-keychain", "Apple Keychain", "security", true),
		backendStatus("pass", "Ubuntu pass", "pass", true),
		backendStatus("1password", "1Password CLI", "op", false),
		backendStatus("bitwarden", "Bitwarden CLI", "bw", false),
	}
}

func List() ([]Entry, error) {
	root, err := Init()
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(entriesDir(root), "*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(paths))
	for _, path := range paths {
		entry, err := readEntry(path)
		if err == nil {
			out = append(out, entry.Entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope == out[j].Scope {
			return out[i].Name < out[j].Name
		}
		return out[i].Scope < out[j].Scope
	})
	return out, nil
}

func Metadata(scope, name string) (Entry, error) {
	scope, name, err := normalizeScopeName(scope, name)
	if err != nil {
		return Entry{}, err
	}
	entry, err := readEntry(entryPath(Root(), scope, name))
	if err != nil {
		return Entry{}, err
	}
	return entry.Entry, nil
}

func MetadataByHandle(handle string) (Entry, error) {
	scope, name, err := ParseHandle(handle)
	if err != nil {
		return Entry{}, err
	}
	return Metadata(scope, name)
}

func Delete(scope, name string) (Entry, error) {
	entry, err := Metadata(scope, name)
	if err != nil {
		return Entry{}, err
	}
	return entry, os.Remove(entryPath(Root(), entry.Scope, entry.Name))
}

func UseEnv(handle, envName string, command []string) (UseResult, error) {
	envName = strings.TrimSpace(envName)
	if !validEnvName(envName) {
		return UseResult{}, errors.New("env name must contain only A-Z, a-z, 0-9, and underscore, and cannot start with a digit")
	}
	if len(command) == 0 {
		return UseResult{}, errors.New("command is required")
	}
	scope, name, err := ParseHandle(handle)
	if err != nil {
		return UseResult{}, err
	}
	value, entry, err := openSecret(scope, name)
	if err != nil {
		return UseResult{}, err
	}
	start := time.Now()
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = append(os.Environ(), envName+"="+string(value))
	out, err := cmd.Output()
	stderr := []byte(nil)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
			exitCode = exitErr.ExitCode()
		} else {
			stderr = []byte(err.Error())
			exitCode = -1
		}
	}
	usedAt := time.Now().UTC()
	_ = touchLastUsed(scope, name, usedAt)
	secret := string(value)
	return UseResult{
		Handle:      entry.Handle,
		EnvName:     envName,
		Command:     command,
		ExitCode:    exitCode,
		Stdout:      redactSecret(string(out), secret),
		Stderr:      redactSecret(string(stderr), secret),
		DurationMs:  time.Since(start).Milliseconds(),
		Fingerprint: entry.Fingerprint,
		UsedAt:      usedAt,
		Policy:      "use-only lease; raw value injected into child process environment and redacted from outputs",
	}, nil
}

func Handle(scope, name string) string {
	return "vault://meshclaw/" + scope + "/" + name
}

func ParseHandle(handle string) (string, string, error) {
	const prefix = "vault://meshclaw/"
	if !strings.HasPrefix(handle, prefix) {
		return "", "", errors.New("handle must start with vault://meshclaw/")
	}
	parts := strings.Split(strings.TrimPrefix(handle, prefix), "/")
	if len(parts) != 2 {
		return "", "", errors.New("handle must be vault://meshclaw/<scope>/<name>")
	}
	return normalizeScopeName(parts[0], parts[1])
}

func normalizeScopeName(scope, name string) (string, string, error) {
	scope = slug(scope)
	name = slug(name)
	if scope == "" || name == "" {
		return "", "", errors.New("scope and name are required")
	}
	return scope, name, nil
}

var slugPattern = regexp.MustCompile(`[^a-z0-9._-]+`)

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-._")
}

func entriesDir(root string) string {
	return filepath.Join(root, "entries")
}

func entryPath(root, scope, name string) string {
	return filepath.Join(entriesDir(root), scope+"--"+name+".json")
}

func masterKey(root string) ([]byte, error) {
	if raw := strings.TrimSpace(os.Getenv("MESHCLAW_GUARD_VAULT_KEY")); raw != "" {
		return decodeKey(raw)
	}
	path := filepath.Join(root, "master.key")
	if data, err := os.ReadFile(path); err == nil {
		return decodeKey(strings.TrimSpace(string(data)))
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func decodeKey(value string) ([]byte, error) {
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	return nil, fmt.Errorf("guard vault key must decode to 32 bytes")
}

func readEntry(path string) (diskEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return diskEntry{}, err
	}
	var entry diskEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return diskEntry{}, err
	}
	entry.Entry.RawAvailable = false
	if entry.Ciphertext != "" {
		entry.Entry.RawAvailable = true
	}
	if entry.Entry.Backend == "apple-keychain" || entry.Entry.Backend == "pass" {
		entry.Entry.RawAvailable = true
	}
	return entry, nil
}

func openSecret(scope, name string) ([]byte, Entry, error) {
	root := Root()
	entry, err := readEntry(entryPath(root, scope, name))
	if err != nil {
		return nil, Entry{}, err
	}
	switch entry.Entry.Backend {
	case "", "local-aes-gcm":
	case "apple-keychain":
		value, err := keychainRead(entry.Entry, scope, name)
		return value, entry.Entry, err
	case "pass":
		value, err := passRead(scope, name)
		return value, entry.Entry, err
	default:
		return nil, Entry{}, fmt.Errorf("guard vault backend %s is metadata-only or unsupported for local use", entry.Entry.Backend)
	}
	key, err := masterKey(root)
	if err != nil {
		return nil, Entry{}, err
	}
	nonce, err := base64.StdEncoding.DecodeString(entry.Nonce)
	if err != nil {
		return nil, Entry{}, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(entry.Ciphertext)
	if err != nil {
		return nil, Entry{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, Entry{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, Entry{}, err
	}
	value, err := gcm.Open(nil, nonce, ciphertext, []byte(entry.Entry.Handle))
	if err != nil {
		return nil, Entry{}, err
	}
	return value, entry.Entry, nil
}

func putExternal(root, backend, scope, name string, opts PutOptions, value []byte) (Entry, error) {
	switch backend {
	case "apple-keychain":
		if err := keychainWrite(scope, name, value); err != nil {
			return Entry{}, err
		}
	case "pass":
		if err := passWrite(scope, name, value); err != nil {
			return Entry{}, err
		}
	default:
		return Entry{}, fmt.Errorf("guard vault backend %s does not support direct put yet", backend)
	}
	now := time.Now().UTC()
	entry := Entry{
		Handle:       Handle(scope, name),
		Scope:        scope,
		Name:         name,
		Kind:         strings.TrimSpace(opts.Kind),
		Description:  strings.TrimSpace(opts.Description),
		Fingerprint:  fingerprint(value),
		CreatedAt:    now,
		UpdatedAt:    now,
		Backend:      backend,
		Policy:       "use-only; raw value is stored in external password manager and is not returned by MeshClaw MCP",
		RawAvailable: true,
	}
	if existing, err := Metadata(scope, name); err == nil {
		entry.CreatedAt = existing.CreatedAt
	}
	payload := diskEntry{Version: version, Entry: entry, UpdatedAt: now}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return Entry{}, err
	}
	if err := os.WriteFile(entryPath(root, scope, name), append(data, '\n'), 0600); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func normalizeBackend(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "local", "aes", "local-aes-gcm", "meshclaw":
		return "local-aes-gcm"
	case "keychain", "apple", "apple-keychain", "macos-keychain":
		return "apple-keychain"
	case "pass", "ubuntu-pass":
		return "pass"
	case "op", "1password", "onepassword":
		return "1password"
	case "bw", "bitwarden":
		return "bitwarden"
	default:
		return value
	}
}

func backendStatus(id, name, executable string, readWrite bool) BackendStatus {
	status := BackendStatus{ID: id, Name: name, ReadWrite: readWrite, Recommended: id == "apple-keychain" || id == "pass"}
	if executable == "" {
		status.Available = true
		status.Status = "available"
		return status
	}
	path, err := exec.LookPath(executable)
	if err != nil {
		status.Status = "missing"
		status.Reason = executable + " not found in PATH"
		return status
	}
	status.Available = true
	status.Executable = path
	status.Status = "available"
	return status
}

func keychainService() string {
	if v := strings.TrimSpace(os.Getenv("MESHCLAW_KEYCHAIN_SERVICE")); v != "" {
		return v
	}
	return "MeshClaw Guard"
}

func keychainAccount(scope, name string) string {
	return scope + "/" + name
}

func keychainWrite(scope, name string, value []byte) error {
	security, err := exec.LookPath("security")
	if err != nil {
		return errors.New("Apple Keychain backend requires security command")
	}
	cmd := exec.Command(security, "add-generic-password", "-a", keychainAccount(scope, name), "-s", keychainService(), "-w", string(value), "-U")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("security add-generic-password failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func keychainRead(entry Entry, scope, name string) ([]byte, error) {
	security, err := exec.LookPath("security")
	if err != nil {
		return nil, errors.New("Apple Keychain backend requires security command")
	}
	service := firstNonEmpty(entry.KeychainService, keychainService())
	account := firstNonEmpty(entry.KeychainAccount, keychainAccount(scope, name))
	cmd := exec.Command(security, "find-generic-password", "-a", account, "-s", service, "-w")
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New("secret not found in Apple Keychain or access denied")
	}
	return []byte(strings.TrimRight(string(out), "\r\n")), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func passPath(scope, name string) string {
	prefix := strings.Trim(strings.TrimSpace(os.Getenv("MESHCLAW_PASS_PREFIX")), "/")
	if prefix == "" {
		prefix = "meshclaw"
	}
	return prefix + "/" + scope + "/" + name
}

func passWrite(scope, name string, value []byte) error {
	pass, err := exec.LookPath("pass")
	if err != nil {
		return errors.New("pass backend requires pass command")
	}
	cmd := exec.Command(pass, "insert", "-m", "-f", passPath(scope, name))
	cmd.Stdin = bytes.NewReader(value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pass insert failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func passRead(scope, name string) ([]byte, error) {
	pass, err := exec.LookPath("pass")
	if err != nil {
		return nil, errors.New("pass backend requires pass command")
	}
	out, err := exec.Command(pass, "show", passPath(scope, name)).Output()
	if err != nil {
		return nil, errors.New("secret not found in pass store or access denied")
	}
	return []byte(strings.TrimRight(string(out), "\r\n")), nil
}

func touchLastUsed(scope, name string, usedAt time.Time) error {
	path := entryPath(Root(), scope, name)
	entry, err := readEntry(path)
	if err != nil {
		return err
	}
	entry.Entry.LastUsedAt = &usedAt
	entry.UpdatedAt = usedAt
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func validEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 && r >= '0' && r <= '9' {
			return false
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func redactSecret(value, secret string) string {
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[REDACTED_SECRET]")
}

func fingerprint(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}
