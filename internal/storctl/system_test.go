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

func TestNormalizeOpenEulerVersions(t *testing.T) {
	cases := map[string]string{
		"22.03":                         "22.03",
		"22.03-LTS-SP4":                 "22.03-lts-sp4",
		"22.03 (LTS-SP4)":               "22.03-lts-sp4",
		"openEuler 22.03 (LTS-SP4)":     "22.03-lts-sp4",
		"VERSION_ID=24.03-LTS-SP2":      "24.03-lts-sp2",
		"SDK openEuler22.03SP4 aarch64": "22.03-lts-sp4",
	}
	for input, want := range cases {
		if got := normalizeOSVersion(input); got != want {
			t.Fatalf("normalizeOSVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseOSReleaseKeepsSPDetails(t *testing.T) {
	info := parseOSRelease([]byte(`ID=openEuler
VERSION_ID="22.03"
VERSION="22.03 (LTS-SP4)"
PRETTY_NAME="openEuler 22.03 (LTS-SP4)"
`))
	if info.ID != "openEuler" || info.VersionID != "22.03" || info.NormalizedVersion != "22.03-lts-sp4" {
		t.Fatalf("unexpected OS info: %+v", info)
	}
}
