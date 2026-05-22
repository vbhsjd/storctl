package storctl

import (
	"fmt"
	"os"
	"strings"
)

func ensureNICExists(nic string) error {
	if _, err := os.Stat(hostPath("/sys/class/net/" + nic)); err != nil {
		return err
	}
	return nil
}

func configureNetwork(cfg Config, r *Reporter, runner Runner) error {
	phys := cfg.physicalConName()
	vlan := cfg.vlanName()
	if err := ensureConnection(runner, phys, "ethernet", cfg.NIC, ""); err != nil {
		return err
	}
	if _, err := runner.Run("nmcli", "con", "mod", phys,
		"connection.autoconnect", "yes",
		"ipv4.method", "disabled",
		"ipv6.method", "disabled",
		"802-3-ethernet.mtu", fmt.Sprintf("%d", cfg.MTU)); err != nil {
		return err
	}
	if err := ensureConnection(runner, vlan, "vlan", vlan, cfg.NIC); err != nil {
		return err
	}
	route := fmt.Sprintf("0.0.0.0/0 %s table=%d", cfg.Gateway, cfg.RouteTable)
	rule := fmt.Sprintf("priority 5 from %s table %d", cfg.dataIPOnly(), cfg.RouteTable)
	if _, err := runner.Run("nmcli", "con", "mod", vlan,
		"connection.autoconnect", "yes",
		"ipv4.method", "manual",
		"ipv4.addresses", cfg.DataCIDR,
		"ipv4.gateway", cfg.Gateway,
		"ipv4.never-default", "yes",
		"ipv4.routes", route,
		"ipv4.routing-rules", rule,
		"ipv6.method", "disabled",
		"vlan.egress-priority-map", "0:3,1:3,2:3,3:3,4:3,5:3,6:3,7:3"); err != nil {
		return err
	}
	if _, err := runner.Run("nmcli", "con", "up", phys); err != nil {
		return err
	}
	if _, err := runner.Run("nmcli", "con", "up", vlan); err != nil {
		return err
	}
	if _, err := runner.Run("ip", "link", "set", "dev", vlan, "mtu", fmt.Sprintf("%d", cfg.MTU)); err != nil {
		return err
	}
	r.OK("vlan %s %s", vlan, cfg.DataCIDR)
	return nil
}

func ensureConnection(runner Runner, name, kind, ifname, parent string) error {
	if connectionExists(runner, name) {
		return nil
	}
	if kind == "ethernet" {
		_, err := runner.Run("nmcli", "con", "add", "type", "ethernet", "ifname", ifname, "con-name", name)
		return err
	}
	_, err := runner.Run("nmcli", "con", "add", "type", "vlan", "con-name", name, "ifname", ifname, "dev", parent, "id", strings.TrimPrefix(ifname[strings.LastIndex(ifname, ".")+1:], "."))
	return err
}

func connectionExists(runner Runner, name string) bool {
	_, err := runner.Run("nmcli", "-t", "-f", "NAME", "con", "show", name)
	return err == nil
}
