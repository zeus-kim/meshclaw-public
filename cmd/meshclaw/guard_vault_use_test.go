package main

import "testing"

func TestParseGuardVaultUsePreservesChildFlags(t *testing.T) {
	req, err := parseGuardVaultUse([]string{
		"vault://meshclaw/test/token",
		"TOKEN",
		"--approve",
		"--actor", "operator",
		"--reason", "smoke",
		"--json",
		"--",
		"provider-cli", "zones", "list", "--json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !req.approved || !req.jsonOut || req.actor != "operator" || req.reason != "smoke" {
		t.Fatalf("unexpected request metadata: %+v", req)
	}
	if got := req.command[len(req.command)-1]; got != "--json" {
		t.Fatalf("child --json was not preserved: %+v", req.command)
	}
}

func TestParseGuardVaultUseRequiresApproval(t *testing.T) {
	req, err := parseGuardVaultUse([]string{
		"vault://meshclaw/test/token",
		"TOKEN",
		"--json",
		"--",
		"echo", "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.approved {
		t.Fatalf("request should not be approved: %+v", req)
	}
}
