package storctl

import (
	"strings"
	"testing"
)

func TestGuardManagementNICFailsWhenNICOwnsMgmtIP(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"ip -o -4 addr show dev enp48s3u1u1": "2: enp48s3u1u1    inet 80.5.21.122/22 brd 80.5.23.255 scope global enp48s3u1u1\n",
		},
	}
	err := guardManagementNIC(Config{NIC: "enp48s3u1u1", MgmtIP: "80.5.21.122"}, runner)
	if err == nil || !strings.Contains(err.Error(), "SSH management interface") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGuardManagementNICAllowsDifferentNIC(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"ip -o -4 addr show dev enp23s0f1": "3: enp23s0f1    inet 172.27.4.113/18 brd 172.27.63.255 scope global data0.172\n",
		},
	}
	if err := guardManagementNIC(Config{NIC: "enp23s0f1", MgmtIP: "80.5.21.122"}, runner); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
