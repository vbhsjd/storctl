package storctl

import "testing"

func TestParseMountSpecDefaultOptions(t *testing.T) {
	got, err := ParseMountSpec("172.27.0.50:/export/a:/mnt/a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Server != "172.27.0.50" || got.Export != "/export/a" || got.MountPoint != "/mnt/a" {
		t.Fatalf("unexpected mount spec: %+v", got)
	}
	if got.Options != defaultNFSOptions {
		t.Fatalf("unexpected options: %s", got.Options)
	}
}

func TestParseMountSpecExtraOptionsOverride(t *testing.T) {
	got, err := ParseMountSpec("172.27.0.50:/export/a:/mnt/a:rsize=524288,soft")
	if err != nil {
		t.Fatal(err)
	}
	if want := "vers=4.1,proto=rdma,port=20049,rsize=524288,wsize=1048576,hard,noatime,soft"; got.Options != want {
		t.Fatalf("options = %q, want %q", got.Options, want)
	}
}

func TestParseMountSpecRejectsBadShape(t *testing.T) {
	cases := []string{
		"172.27.0.50:/export/a",
		"172.27.0.50:export/a:/mnt/a",
		"172.27.0.50:/export/a:mnt/a",
	}
	for _, tc := range cases {
		if _, err := ParseMountSpec(tc); err == nil {
			t.Fatalf("ParseMountSpec(%q) succeeded, want error", tc)
		}
	}
}

func TestIsOpenEulerCaseInsensitive(t *testing.T) {
	if !isOpenEuler("openEuler") {
		t.Fatal("openEuler should be detected case-insensitively")
	}
}

func TestConfigRejectsAutoNIC(t *testing.T) {
	cfg := Config{
		NIC:        "auto",
		NICType:    "auto",
		VLANID:     172,
		DataCIDR:   "172.27.4.113/18",
		Gateway:    "172.27.0.1",
		RouteTable: 5000,
		MTU:        5500,
		Mounts: []MountSpec{{
			Server:     "172.27.1.1",
			Export:     "/Share",
			MountPoint: "/mnt/share",
			Options:    defaultNFSOptions,
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected --nic auto to be rejected")
	}
}
