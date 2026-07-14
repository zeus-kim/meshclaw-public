# Public Release Checklist

This branch is prepared as a public-safe source snapshot.

Before making a repository public:

1. Use a clean-history public branch or a fresh public repository.
2. Ensure private branches, tags, and historical commits are not exposed.
3. Run the secret scan (credentials, keys, tokens):

   ```sh
   rg -n -iE 'ghp_|gho_|sk-ant-|pypi-AgEI|AKIA[0-9A-Z]{16}|xox[baprs]-|BEGIN .* PRIVATE KEY|VSSH_SECRET|api[_-]?key|password\s*[:=]' .
   ```

4. Run the personal-data scan. None of these should survive into a public
   snapshot — replace real values with public placeholders
   (`you@example.com`, `example.com`, `/Users/example`, Tailscale `100.64.0.x`):

   ```sh
   # Real e-mail addresses (anything not an example.* placeholder)
   rg -n -iE '[a-z0-9._%+-]+@(?!example\.)[a-z0-9.-]+\.[a-z]{2,}' .
   # Hard-coded IPv4 — keep only documentation ranges (100.64.0.x, 203.0.113.x)
   rg -n -E '\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b' .
   # Operator hostnames / real domains — none should remain in the public tree
   #   (keep the concrete blocklist of your real domains in the PRIVATE cutting
   #    script, not here; see scripts/public-scan.sh in the private repo)
   # Private home paths
   rg -n -E '/Users/(?!example|argos)[a-z0-9_-]+' .
   ```

   Deployment/marketing assets tied to real domains (nginx configs, per-domain
   landing pages, install scripts under `web/`) are operator infrastructure —
   drop them from the snapshot rather than trying to sanitize them in place.

5. Run tests:

   ```sh
   go test ./...
   go build -o ./bin/meshclaw ./cmd/meshclaw
   ```

6. Confirm that only public-safe docs are included.
7. Keep private fleet configuration (real hosts, IPs, credentials, domains)
   outside the repository. The authoritative blocklist of real operator values
   lives in the private repo's `scripts/public-scan.sh`; run it against the
   snapshot and require a clean pass before publishing.

The private development repository should not simply be toggled public if its
history contains local handoff logs, private paths, private fleet topology, or
operator-specific launchd/deployment scripts.
