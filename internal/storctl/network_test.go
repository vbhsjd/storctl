package storctl

import (
	"strings"
	"testing"
)

func TestConfigureNetworkRepairsExistingVLANParent(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"nmcli -t -f NAME con show storctl-enp23s0f1": "",
			"nmcli -t -f NAME con show data0.172":         "",
			"nmcli con mod storctl-enp23s0f1 connection.interface-name enp23s0f1 connection.autoconnect yes ipv4.method disabled ipv6.method disabled 802-3-ethernet.mac-address  802-3-ethernet.mtu 5500":                                                                                                                                                                                                                   "",
			"nmcli con mod data0.172 connection.interface-name data0.172 connection.autoconnect yes vlan.parent enp23s0f1 vlan.id 172 ipv4.method manual ipv4.addresses 172.27.4.113/18 ipv4.gateway 172.27.0.1 ipv4.never-default yes ipv4.routes 0.0.0.0/0 172.27.0.1 table=5000 ipv4.routing-rules priority 5 from 172.27.4.113 table 5000 ipv6.method disabled vlan.egress-priority-map 0:3,1:3,2:3,3:3,4:3,5:3,6:3,7:3": "",
			"nmcli con up storctl-enp23s0f1":     "",
			"ip link set dev enp23s0f1 mtu 5500": "",
			"nmcli con up data0.172":             "",
			"ip link set dev data0.172 mtu 5500": "",
		},
	}
	var out, stderr strings.Builder
	cfg := Config{
		NIC:        "enp23s0f1",
		VLANID:     172,
		DataCIDR:   "172.27.4.113/18",
		Gateway:    "172.27.0.1",
		RouteTable: 5000,
		MTU:        5500,
	}
	if err := configureNetwork(cfg, NewReporter(&out, &stderr), runner); err != nil {
		t.Fatal(err)
	}
	var vlanMod string
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "nmcli con mod data0.172 ") {
			vlanMod = call
		}
	}
	if !containsAll(vlanMod, "vlan.parent enp23s0f1", "vlan.id 172", "connection.interface-name data0.172") {
		t.Fatalf("vlan connection was not repaired: %s", vlanMod)
	}
}

func TestConfigureNetworkDoesNotBindPhysicalConnectionToMAC(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"nmcli -t -f NAME con show storctl-enp23s0f1": "",
			"nmcli -t -f NAME con show data0.172":         "",
			"nmcli con mod storctl-enp23s0f1 connection.interface-name enp23s0f1 connection.autoconnect yes ipv4.method disabled ipv6.method disabled 802-3-ethernet.mac-address  802-3-ethernet.mtu 5500":                                                                                                                                                                                                                   "",
			"nmcli con mod data0.172 connection.interface-name data0.172 connection.autoconnect yes vlan.parent enp23s0f1 vlan.id 172 ipv4.method manual ipv4.addresses 172.27.4.113/18 ipv4.gateway 172.27.0.1 ipv4.never-default yes ipv4.routes 0.0.0.0/0 172.27.0.1 table=5000 ipv4.routing-rules priority 5 from 172.27.4.113 table 5000 ipv6.method disabled vlan.egress-priority-map 0:3,1:3,2:3,3:3,4:3,5:3,6:3,7:3": "",
			"nmcli con up storctl-enp23s0f1":     "",
			"ip link set dev enp23s0f1 mtu 5500": "",
			"nmcli con up data0.172":             "",
			"ip link set dev data0.172 mtu 5500": "",
		},
	}
	var out, stderr strings.Builder
	cfg := Config{
		NIC:        "enp23s0f1",
		VLANID:     172,
		DataCIDR:   "172.27.4.113/18",
		Gateway:    "172.27.0.1",
		RouteTable: 5000,
		MTU:        5500,
	}
	if err := configureNetwork(cfg, NewReporter(&out, &stderr), runner); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.calls {
		if strings.Contains(call, "aa:") || strings.Contains(call, "mac-address aa") {
			t.Fatalf("unexpected MAC binding in call: %s", call)
		}
	}
}

func TestConfigureNetworkDeletesStaleVLANParentBeforeUp(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"nmcli -t -f NAME con show storctl-eth3": "",
			"nmcli -t -f NAME con show data0.3001":   "",
			"nmcli con mod storctl-eth3 connection.interface-name eth3 connection.autoconnect yes ipv4.method disabled ipv6.method disabled 802-3-ethernet.mac-address  802-3-ethernet.mtu 5500":                                                                                                                                                                                                                           "",
			"nmcli con mod data0.3001 connection.interface-name data0.3001 connection.autoconnect yes vlan.parent eth3 vlan.id 3001 ipv4.method manual ipv4.addresses 172.27.6.185/18 ipv4.gateway 172.27.0.1 ipv4.never-default yes ipv4.routes 0.0.0.0/0 172.27.0.1 table=5000 ipv4.routing-rules priority 5 from 172.27.6.185 table 5000 ipv6.method disabled vlan.egress-priority-map 0:3,1:3,2:3,3:3,4:3,5:3,6:3,7:3": "",
			"ip -o link show data0.3001":          "9: data0.3001@eth4: <BROADCAST,MULTICAST> mtu 1500 state LOWERLAYERDOWN mode DEFAULT\n",
			"nmcli con up storctl-eth3":           "",
			"ip link set dev eth3 mtu 5500":       "",
			"nmcli con up data0.3001":             "",
			"ip link set dev data0.3001 mtu 5500": "",
		},
	}
	var out, stderr strings.Builder
	cfg := Config{
		NIC:        "eth3",
		VLANID:     3001,
		DataCIDR:   "172.27.6.185/18",
		Gateway:    "172.27.0.1",
		RouteTable: 5000,
		MTU:        5500,
	}
	if err := configureNetwork(cfg, NewReporter(&out, &stderr), runner); err != nil {
		t.Fatal(err)
	}
	if !containsCallBefore(runner.calls, "ip link delete data0.3001", "nmcli con up data0.3001") {
		t.Fatalf("stale vlan link was not deleted before up: %+v", runner.calls)
	}
	if !strings.Contains(out.String(), "WARN vlan data0.3001 parent eth4 -> eth3") {
		t.Fatalf("missing parent repair warning: %s", out.String())
	}
}

func TestConfigureNetworkRebuildsVLANWhenMTUSetFails(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"nmcli -t -f NAME con show storctl-eth3": "",
			"nmcli -t -f NAME con show data0.3001":   "",
			"nmcli con mod storctl-eth3 connection.interface-name eth3 connection.autoconnect yes ipv4.method disabled ipv6.method disabled 802-3-ethernet.mac-address  802-3-ethernet.mtu 5500":                                                                                                                                                                                                                           "",
			"nmcli con mod data0.3001 connection.interface-name data0.3001 connection.autoconnect yes vlan.parent eth3 vlan.id 3001 ipv4.method manual ipv4.addresses 172.27.6.185/18 ipv4.gateway 172.27.0.1 ipv4.never-default yes ipv4.routes 0.0.0.0/0 172.27.0.1 table=5000 ipv4.routing-rules priority 5 from 172.27.6.185 table 5000 ipv6.method disabled vlan.egress-priority-map 0:3,1:3,2:3,3:3,4:3,5:3,6:3,7:3": "",
			"nmcli con up storctl-eth3":           "",
			"ip link set dev eth3 mtu 5500":       "",
			"nmcli con up data0.3001":             "",
			"ip link set dev data0.3001 mtu 5500": "",
		},
		errors: map[string]error{
			"ip link set dev data0.3001 mtu 5500": failf("RTNETLINK answers: Numerical result out of range"),
		},
		failOnce: map[string]bool{
			"ip link set dev data0.3001 mtu 5500": true,
		},
	}
	var out, stderr strings.Builder
	cfg := Config{
		NIC:        "eth3",
		VLANID:     3001,
		DataCIDR:   "172.27.6.185/18",
		Gateway:    "172.27.0.1",
		RouteTable: 5000,
		MTU:        5500,
	}
	if err := configureNetwork(cfg, NewReporter(&out, &stderr), runner); err != nil {
		t.Fatal(err)
	}
	if !containsCallBefore(runner.calls, "ip link delete data0.3001", "ip link set dev data0.3001 mtu 5500") {
		t.Fatalf("vlan was not rebuilt after mtu failure: %+v", runner.calls)
	}
}

func TestParseVLANParentFromIPLink(t *testing.T) {
	got, ok := parseVLANParentFromIPLink("9: data0.3001@eth4: <BROADCAST,MULTICAST> mtu 1500 state LOWERLAYERDOWN\n")
	if !ok || got != "eth4" {
		t.Fatalf("parent = %q ok=%v", got, ok)
	}
}

func containsCallBefore(calls []string, first, second string) bool {
	firstAt := -1
	for i, call := range calls {
		if call == first && firstAt == -1 {
			firstAt = i
		}
		if call == second && firstAt >= 0 && firstAt < i {
			return true
		}
	}
	return false
}
