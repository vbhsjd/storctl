package storctl

import (
	"fmt"
	"os"
	"strconv"
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
	vlanID := fmt.Sprintf("%d", cfg.VLANID)
	if err := ensureConnection(runner, phys, "ethernet", cfg.NIC, ""); err != nil {
		return err
	}
	if _, err := runner.Run("nmcli", "con", "mod", phys,
		"connection.interface-name", cfg.NIC,
		"connection.autoconnect", "yes",
		"ipv4.method", "disabled",
		"ipv6.method", "disabled",
		"802-3-ethernet.mac-address", "",
		"802-3-ethernet.mtu", fmt.Sprintf("%d", cfg.MTU)); err != nil {
		return err
	}
	if err := ensureConnection(runner, vlan, "vlan", vlan, cfg.NIC); err != nil {
		return err
	}
	route := fmt.Sprintf("0.0.0.0/0 %s table=%d", cfg.Gateway, cfg.RouteTable)
	rule := fmt.Sprintf("priority 5 from %s table %d", cfg.dataIPOnly(), cfg.RouteTable)
	if _, err := runner.Run("nmcli", "con", "mod", vlan,
		"connection.interface-name", vlan,
		"connection.autoconnect", "yes",
		"vlan.parent", cfg.NIC,
		"vlan.id", vlanID,
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
	repairStaleVLANParent(cfg, vlan, r, runner)
	if _, err := runner.Run("nmcli", "con", "up", phys); err != nil {
		return err
	}
	if err := setLinkMTU(runner, cfg.NIC, cfg.MTU); err != nil {
		return fmt.Errorf("set parent mtu %s=%d: %w", cfg.NIC, cfg.MTU, err)
	}
	if _, err := runner.Run("nmcli", "con", "up", vlan); err != nil {
		return err
	}
	if err := setLinkMTU(runner, vlan, cfg.MTU); err != nil {
		if repairErr := rebuildVLANLink(cfg, vlan, phys, runner); repairErr != nil {
			return fmt.Errorf("set vlan mtu %s=%d: %w; rebuild failed: %v", vlan, cfg.MTU, err, repairErr)
		}
	}
	r.OK("vlan %s %s", vlan, cfg.DataCIDR)
	return nil
}

func setLinkMTU(runner Runner, link string, mtu int) error {
	_, err := runner.Run("ip", "link", "set", "dev", link, "mtu", strconv.Itoa(mtu))
	return err
}

func repairStaleVLANParent(cfg Config, vlan string, r *Reporter, runner Runner) {
	parent, ok := currentVLANParent(runner, vlan)
	if !ok || parent == "" || parent == cfg.NIC {
		return
	}
	r.Warn("vlan %s parent %s -> %s, recreating stale link", vlan, parent, cfg.NIC)
	_, _ = runner.Run("nmcli", "con", "down", vlan)
	_, _ = runner.Run("ip", "link", "delete", vlan)
}

func rebuildVLANLink(cfg Config, vlan, phys string, runner Runner) error {
	_, _ = runner.Run("nmcli", "con", "down", vlan)
	_, _ = runner.Run("ip", "link", "delete", vlan)
	if _, err := runner.Run("nmcli", "con", "up", phys); err != nil {
		return err
	}
	if err := setLinkMTU(runner, cfg.NIC, cfg.MTU); err != nil {
		return err
	}
	if _, err := runner.Run("nmcli", "con", "up", vlan); err != nil {
		return err
	}
	return setLinkMTU(runner, vlan, cfg.MTU)
}

func currentVLANParent(runner Runner, vlan string) (string, bool) {
	out, err := runner.Run("ip", "-o", "link", "show", vlan)
	if err != nil {
		return "", false
	}
	return parseVLANParentFromIPLink(out)
}

func parseVLANParentFromIPLink(out string) (string, bool) {
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return "", false
	}
	name := strings.TrimSuffix(fields[1], ":")
	_, parent, ok := strings.Cut(name, "@")
	return parent, ok
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
