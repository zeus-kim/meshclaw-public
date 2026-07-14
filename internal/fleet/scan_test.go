package fleet

import "testing"

func TestSelectedHostsDedupesAndValidates(t *testing.T) {
	hosts, err := selectedHosts([]string{"d1", "d1", "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0] != "d1" || hosts[1] != "v1" {
		t.Fatalf("hosts = %#v, want d1,v1", hosts)
	}
}

func TestSelectedHostsRejectsUnknown(t *testing.T) {
	if _, err := selectedHosts([]string{"missing"}); err == nil {
		t.Fatal("expected unknown node error")
	}
}

func TestNormalizeOptionsDefaultsToFullScan(t *testing.T) {
	opts := normalizeOptions(Options{})
	if !opts.Security || !opts.Hygiene || !opts.Logs {
		t.Fatalf("opts = %#v, want all scan modes enabled", opts)
	}
	if opts.MaxParallel <= 0 {
		t.Fatalf("MaxParallel = %d, want positive", opts.MaxParallel)
	}
}
