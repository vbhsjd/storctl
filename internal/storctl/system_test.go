package storctl

import (
	"os"
	"path/filepath"
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
	if state.SchemaVersion != stateSchemaVersion {
		t.Fatalf("SchemaVersion = %d", state.SchemaVersion)
	}
}

func TestLoadLegacyStateWithoutSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	data := `{"nic":"enp23s0f1","vlan":"data0.172"}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	state, err := loadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 0 || state.NIC != "enp23s0f1" {
		t.Fatalf("legacy state not loaded: %+v", state)
	}
}
