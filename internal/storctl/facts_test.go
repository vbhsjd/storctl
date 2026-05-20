package storctl

import "testing"

func TestCollectFactsIncludesCommandAndRDMA(t *testing.T) {
	runner := &fakeRunner{
		exists: map[string]bool{"rdma": true, "nmcli": true},
		outputs: map[string]string{
			"rdma link": "link mlx5_0/1 state ACTIVE netdev enp1s0\n",
		},
	}
	report := collectFacts(runner)
	if !report.Commands["rdma"] || !report.Commands["nmcli"] {
		t.Fatalf("commands missing: %+v", report.Commands)
	}
	if !report.RDMA.Available || len(report.RDMA.Links) != 1 {
		t.Fatalf("rdma facts missing: %+v", report.RDMA)
	}
	if report.OS.Arch == "" {
		t.Fatalf("arch missing: %+v", report.OS)
	}
}
