package storctl

import (
	"strings"
	"testing"
)

func TestEnsureApplyDriverReadyAllowsExplicitTCPFallback(t *testing.T) {
	runner := &fakeRunner{
		exists: map[string]bool{"hinicadm3": true, "rdma": true},
		outputs: map[string]string{
			"rdma link": "",
		},
	}
	var out, stderr strings.Builder
	ready, err := ensureApplyDriverReady(Config{AllowTCPFallback: true}, "1823", NewReporter(&out, &stderr), runner)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("driver should not be marked ready")
	}
	if !strings.Contains(out.String(), "WARN driver 1823 not ready, using explicit tcp fallback") {
		t.Fatalf("missing warning: stdout=%q stderr=%q", out.String(), stderr.String())
	}
}

func TestEnsureApplyDriverReadyFailsWithoutTCPFallback(t *testing.T) {
	runner := &fakeRunner{
		exists: map[string]bool{"hinicadm3": true, "rdma": true},
		outputs: map[string]string{
			"rdma link": "",
		},
	}
	var out, stderr strings.Builder
	ready, err := ensureApplyDriverReady(Config{}, "1823", NewReporter(&out, &stderr), runner)
	if err == nil || !strings.Contains(err.Error(), "rdma link is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Fatal("driver should not be marked ready")
	}
}
