package storctl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePlanWithProfileDerivesDataIP(t *testing.T) {
	path := writeTestProfile(t)
	cfg, err := parsePlan([]string{
		"--profile-file", path,
		"--profile", "c4",
		"--nic", "enp23s0f1",
		"--mgmt-ip", "80.5.17.113",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VLANID != 172 {
		t.Fatalf("VLANID = %d", cfg.VLANID)
	}
	if cfg.DataCIDR != "172.27.4.113/18" {
		t.Fatalf("DataCIDR = %q", cfg.DataCIDR)
	}
	if cfg.Gateway != "172.27.0.1" {
		t.Fatalf("Gateway = %q", cfg.Gateway)
	}
	if len(cfg.Mounts) != 2 || cfg.Mounts[0].MountPoint != "/mnt/share" {
		t.Fatalf("Mounts = %+v", cfg.Mounts)
	}
}

func TestCLIOverridesProfile(t *testing.T) {
	path := writeTestProfile(t)
	cfg, err := parsePlan([]string{
		"--profile-file", path,
		"--profile", "c4",
		"--nic", "enp23s0f1",
		"--mgmt-ip", "80.5.17.113",
		"--vlan-id", "3001",
		"--data-ip", "172.27.9.9/18",
		"--gateway", "172.27.0.254",
		"--mount", "172.27.1.2:/Other:/mnt/other",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VLANID != 3001 || cfg.DataCIDR != "172.27.9.9/18" || cfg.Gateway != "172.27.0.254" {
		t.Fatalf("unexpected overrides: %+v", cfg)
	}
	if len(cfg.Mounts) != 1 || cfg.Mounts[0].MountPoint != "/mnt/other" {
		t.Fatalf("mount override failed: %+v", cfg.Mounts)
	}
}

func TestMissingThirdOctetMappingFails(t *testing.T) {
	path := writeTestProfile(t)
	_, err := parsePlan([]string{
		"--profile-file", path,
		"--profile", "c4",
		"--nic", "enp23s0f1",
		"--mgmt-ip", "80.5.99.113",
	})
	if err == nil || !strings.Contains(err.Error(), "no third_octet_map entry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlanOutputHasNoMutation(t *testing.T) {
	path := writeTestProfile(t)
	cfg, err := parsePlan([]string{
		"--profile-file", path,
		"--profile", "c4",
		"--nic", "enp23s0f1",
		"--mgmt-ip", "80.5.17.113",
	})
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Plan(cfg, NewReporter(&out, &stderr)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "OK data-ip 172.27.4.113/18") {
		t.Fatalf("missing data-ip: %s", got)
	}
	if !strings.Contains(got, "OK plan no changes applied") {
		t.Fatalf("missing no changes marker: %s", got)
	}
}

func writeTestProfile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "storctl-profiles.json")
	data := `{
  "profiles": {
    "c4": {
      "vlan_id": 172,
      "gateway": "172.27.0.1",
      "prefix": 18,
      "route_table": 5000,
      "mtu": 5500,
      "artifact_dir": "/root/storage_pkgs",
      "third_octet_map": {"17": 4},
      "mounts": [
        {"server": "172.27.1.1", "export": "/Share", "mount_point": "/mnt/share"},
        {"server": "172.27.1.1", "export": "/Weight", "mount_point": "/mnt/weight"}
      ]
    }
  }
}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
