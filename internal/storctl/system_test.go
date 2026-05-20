package storctl

import (
	"strings"
	"testing"
)

func TestSaveStateRecordsDegradedFallback(t *testing.T) {
	cfg := Config{
		NIC:        "enp23s0f1",
		VLANID:     172,
		DataCIDR:   "172.27.4.113/18",
		Gateway:    "172.27.0.1",
		RouteTable: 5000,
		MTU:        5500,
		StateDir:   t.TempDir(),
		Mounts: []MountSpec{{
			Server:     "172.27.1.1",
			Export:     "/Share",
			MountPoint: "/mnt/share",
			Options:    defaultTCPOptions,
		}},
	}
	if err := saveState(cfg, "1823", false, true, true, "rdma link is empty"); err != nil {
		t.Fatal(err)
	}
	state, err := loadState(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Degraded || !strings.Contains(state.DegradedReason, "rdma link is empty") {
		t.Fatalf("degraded state not recorded: %+v", state)
	}
}
